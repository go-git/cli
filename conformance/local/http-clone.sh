#!/usr/bin/env bash
#
# Local conformance test: HTTP clone over gogit-http-server, in both sha1 and
# sha256 modes. Upstream's HTTP tests (t5551, t5561) require Apache and the
# `git-http-backend` CGI, so they can't drive our standalone server. This
# script is the in-house equivalent — start gogit-http-server on a free port,
# build a source repo in the chosen hash, clone it via http://, verify the
# clone's format and HEAD match the source.
#
# Inputs:
#   GOGIT      — path to the gogit binary (built by the wrapper)
#   SERVER     — path to the gogit-http-server binary (built by the wrapper)
#   WORK_DIR   — scratch directory; the test owns it for the run
#   PORT       — TCP port to bind the HTTP server to
#
# Exit 0 on full success; non-zero with a printed reason on any failure.

set -euo pipefail

: "${GOGIT:?GOGIT not set}"
: "${SERVER:?SERVER not set}"
: "${WORK_DIR:?WORK_DIR not set}"
: "${PORT:?PORT not set}"

# A fresh HOME so the user's git config (commit.gpgsign, hooks, includes…)
# can't reach into the test repo.
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

# Wait until the server is accepting connections (up to ~2s). Without this
# the first clone races the server's bind() and produces a flaky failure.
for _ in $(seq 1 20); do
    if (echo >/dev/tcp/127.0.0.1/"$PORT") 2>/dev/null; then
        break
    fi
    sleep 0.1
done

run_case() {
    local hash="$1"
    local repo="$SERVE_DIR/repo-$hash"

    "$GOGIT" init --quiet --object-format="$hash" "$repo"
    echo "content-$hash" >"$repo/file"
    ( cd "$repo" && "$GOGIT" add file )
    ( cd "$repo" && "$GOGIT" commit -m initial >/dev/null )

    local source_head
    source_head=$( cd "$repo" && "$GOGIT" rev-parse HEAD )
    local source_format
    source_format=$( cd "$repo" && "$GOGIT" rev-parse --show-object-format )

    if [ "$source_format" != "$hash" ]; then
        echo "FAIL ($hash): source repo format is $source_format, want $hash" >&2
        return 1
    fi

    local clone="$WORK_DIR/clone-$hash"
    rm -rf "$clone"

    "$GOGIT" clone "http://127.0.0.1:$PORT/repo-$hash" "$clone" >"$WORK_DIR/clone-$hash.log" 2>&1

    local clone_head
    clone_head=$( cd "$clone" && "$GOGIT" rev-parse HEAD )
    local clone_format
    clone_format=$( cd "$clone" && "$GOGIT" rev-parse --show-object-format )

    if [ "$clone_format" != "$hash" ]; then
        echo "FAIL ($hash): clone repo format is $clone_format, want $hash" >&2
        return 1
    fi

    if [ "$clone_head" != "$source_head" ]; then
        echo "FAIL ($hash): clone HEAD ($clone_head) != source HEAD ($source_head)" >&2
        return 1
    fi

    if ! [ -f "$clone/file" ] || [ "$(cat "$clone/file")" != "content-$hash" ]; then
        echo "FAIL ($hash): clone worktree file missing or wrong content" >&2
        return 1
    fi

    echo "ok - http-clone $hash (HEAD=${clone_head:0:12}…)"
}

run_case sha1
run_case sha256
