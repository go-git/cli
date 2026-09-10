#!/usr/bin/env bash
#
# Local conformance test: HTTP push via gogit-http-server, in both sha1 and
# sha256 modes. Same shape as http-clone.sh — start the server, build a bare
# destination repo in the chosen hash format, push to it, verify the server
# now holds the pushed commit by re-cloning and comparing HEADs.
#
# Inputs:
#   GOGIT      — path to the gogit binary
#   SERVER     — path to the gogit-http-server binary
#   WORK_DIR   — scratch directory; the test owns it
#   PORT       — TCP port for the HTTP server
#
# Exit 0 on full success; non-zero with a printed reason on any failure.

set -euo pipefail

: "${GOGIT:?GOGIT not set}"
: "${SERVER:?SERVER not set}"
: "${WORK_DIR:?WORK_DIR not set}"
: "${PORT:?PORT not set}"

HOME="$WORK_DIR/home"
mkdir -p "$HOME"
export HOME GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=t@example.com
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=t@example.com
export GIT_AUTHOR_DATE='1700000000 +0000'
export GIT_COMMITTER_DATE='1700000000 +0000'

SERVE_DIR="$WORK_DIR/serve"
mkdir -p "$SERVE_DIR"

SERVER_PID=""
cleanup() {
    if [ -n "$SERVER_PID" ]; then
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

"$SERVER" -p "$PORT" "$SERVE_DIR" >"$WORK_DIR/server.log" 2>&1 &
SERVER_PID=$!

for _ in $(seq 1 20); do
    if (echo >/dev/tcp/127.0.0.1/"$PORT") 2>/dev/null; then
        break
    fi
    sleep 0.1
done

run_case() {
    local hash="$1"
    local seed="$WORK_DIR/seed-$hash"
    local bare="$SERVE_DIR/dest-$hash.git"

    # Seed: a non-bare repo with an initial commit, used only to populate
    # the bare destination with starting history.
    "$GOGIT" init --quiet --object-format="$hash" "$seed"
    echo "seed-$hash" >"$seed/file"
    ( cd "$seed" && "$GOGIT" add file )
    ( cd "$seed" && "$GOGIT" commit -m initial >/dev/null )

    "$GOGIT" clone --bare "$seed" "$bare" >/dev/null 2>&1

    # Clone with credentials baked into the URL. go-git stores the URL
    # verbatim in the remote config, which means subsequent pushes inherit
    # the Authorization header. The server doesn't validate the credentials
    # — it just rejects receive-pack requests without an Authorization
    # header as a basic sanity check.
    local client="$WORK_DIR/client-$hash"
    "$GOGIT" clone "http://test:test@127.0.0.1:$PORT/dest-$hash.git" "$client" \
        >"$WORK_DIR/client-clone-$hash.log" 2>&1

    echo "pushed-$hash" >"$client/file"
    ( cd "$client" && "$GOGIT" add file )
    ( cd "$client" && "$GOGIT" commit -m "push-update" >/dev/null )

    local pushed_head
    pushed_head=$( cd "$client" && "$GOGIT" rev-parse HEAD )

    ( cd "$client" && "$GOGIT" push origin master ) \
        >"$WORK_DIR/push-$hash.log" 2>&1

    # Verify: re-clone the destination and confirm its HEAD matches.
    local verify="$WORK_DIR/verify-$hash"
    "$GOGIT" clone "http://127.0.0.1:$PORT/dest-$hash.git" "$verify" \
        >"$WORK_DIR/verify-clone-$hash.log" 2>&1

    local verify_head
    verify_head=$( cd "$verify" && "$GOGIT" rev-parse HEAD )
    local verify_format
    verify_format=$( cd "$verify" && "$GOGIT" rev-parse --show-object-format )

    if [ "$verify_format" != "$hash" ]; then
        echo "FAIL ($hash): verify clone format is $verify_format, want $hash" >&2
        return 1
    fi

    if [ "$verify_head" != "$pushed_head" ]; then
        echo "FAIL ($hash): server HEAD after push ($verify_head) != client HEAD ($pushed_head)" >&2
        return 1
    fi

    if [ "$(cat "$verify/file")" != "pushed-$hash" ]; then
        echo "FAIL ($hash): pushed content not present in re-clone" >&2
        return 1
    fi

    echo "ok - http-push $hash (HEAD=${verify_head:0:12}…)"
}

run_case sha1
run_case sha256
