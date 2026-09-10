#!/usr/bin/env bash
#
# Runner for local (in-house) conformance tests under conformance/local/.
#
# Upstream's HTTP test framework (lib-httpd.sh, t5551, t5561) is locked to
# Apache + git-http-backend, so it can't drive `gogit-http-server`. This
# runner is the in-house equivalent: it builds gogit + gogit-http-server,
# allocates a scratch dir and TCP port, and runs each *.sh in this
# directory as a self-contained sub-test.
#
# Each sub-test receives:
#   GOGIT     — path to the freshly-built gogit binary
#   SERVER    — path to the freshly-built gogit-http-server binary
#   WORK_DIR  — its own scratch dir
#   PORT      — a free TCP port
#
# Non-zero exit from any sub-test fails the runner.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
LOCAL_DIR="$REPO_ROOT/conformance/local"
CACHE_DIR="$REPO_ROOT/conformance/.cache"
BIN_DIR="$CACHE_DIR/bin"

mkdir -p "$BIN_DIR"

echo "Building gogit..."
( cd "$REPO_ROOT" && go build -o "$BIN_DIR/gogit" ./cmd/gogit )

echo "Building gogit-http-server..."
( cd "$REPO_ROOT" && go build -o "$BIN_DIR/gogit-http-server" ./cmd/gogit-http-server )

GOGIT="$BIN_DIR/gogit"
SERVER="$BIN_DIR/gogit-http-server"

# Pick a free local port. mktemp+ss-based discovery would also work but
# this is portable and good enough — collisions just re-roll.
pick_port() {
    local p
    for _ in 1 2 3 4 5; do
        p=$(( (RANDOM % 20000) + 30000 ))
        if ! (echo >/dev/tcp/127.0.0.1/"$p") 2>/dev/null; then
            echo "$p"
            return 0
        fi
    done
    echo "could not allocate a free local port after 5 attempts" >&2
    return 1
}

EXIT_CODE=0
for test_script in "$LOCAL_DIR"/*.sh; do
    [ -e "$test_script" ] || continue

    case "$(basename "$test_script")" in
        run.sh) continue ;;
    esac

    name=$(basename "$test_script" .sh)
    work_dir="$CACHE_DIR/local/$name"
    rm -rf "$work_dir"
    mkdir -p "$work_dir"

    port=$(pick_port)

    echo "=== Running local/$name (port $port) ==="

    if GOGIT="$GOGIT" SERVER="$SERVER" WORK_DIR="$work_dir" PORT="$port" \
            bash "$test_script"; then
        :
    else
        EXIT_CODE=1
        echo "--- $name failed; server log: ---" >&2
        if [ -f "$work_dir/server.log" ]; then
            sed 's/^/  /' "$work_dir/server.log" >&2
        fi
    fi
done

exit "$EXIT_CODE"
