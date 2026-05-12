#!/usr/bin/env bash
set -euo pipefail

# Resolve repo root (parent of conformance/).
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CACHE_DIR="$REPO_ROOT/conformance/.cache"
BIN_DIR="$CACHE_DIR/bin"
RESULTS_DIR="$CACHE_DIR/results"
BUILD_SNAPSHOT="$CACHE_DIR/build"

mkdir -p "$BIN_DIR" "$RESULTS_DIR"

# Optional go-git ref override: snapshot go.mod/go.sum, bump, restore on exit.
if [ -n "${GO_GIT_REF:-}" ]; then
    mkdir -p "$BUILD_SNAPSHOT"
    cp "$REPO_ROOT/go.mod" "$BUILD_SNAPSHOT/go.mod"
    cp "$REPO_ROOT/go.sum" "$BUILD_SNAPSHOT/go.sum"
    trap 'cp "$BUILD_SNAPSHOT/go.mod" "$REPO_ROOT/go.mod"; cp "$BUILD_SNAPSHOT/go.sum" "$REPO_ROOT/go.sum"' EXIT
    ( cd "$REPO_ROOT" && go get "github.com/go-git/go-git/v6@$GO_GIT_REF" )
fi

echo "Building gogit..."
( cd "$REPO_ROOT" && go build -o "$BIN_DIR/gogit" ./cmd/gogit )
ln -sf gogit "$BIN_DIR/git"

# Resolve upstream tests source.
if [ -n "${GIT_SRC:-}" ] && [ -f "$GIT_SRC/t/test-lib.sh" ]; then
    UPSTREAM_T="$GIT_SRC/t"
    echo "Using GIT_SRC=$GIT_SRC"
else
    UPSTREAM_REPO="$CACHE_DIR/git"
    if [ ! -d "$UPSTREAM_REPO/.git" ]; then
        echo "Cloning git/git into $UPSTREAM_REPO..."
        git clone --depth=1 https://github.com/git/git "$UPSTREAM_REPO"
    else
        echo "Refreshing $UPSTREAM_REPO from origin..."
        git -C "$UPSTREAM_REPO" fetch --depth=1 origin
        git -C "$UPSTREAM_REPO" reset --hard FETCH_HEAD
    fi
    UPSTREAM_T="$UPSTREAM_REPO/t"
fi

# Prepare a minimal GIT_BUILD_DIR so test-lib.sh can initialise.
# test-lib.sh requires: GIT-BUILD-OPTIONS, t/helper/test-tool, and
# a valid GIT_TEST_TEMPLATE_DIR. None of these need to be a real git
# build; a stub test-tool and the system templates suffice for t2008.
FAKE_BUILD_DIR="$CACHE_DIR/fake-build"
mkdir -p "$FAKE_BUILD_DIR/t/helper"

# Locate system git templates (needed by test-lib.sh's BAIL_OUT guard).
SYSTEM_TEMPLATES=""
for d in /usr/share/git-core/templates /usr/local/share/git-core/templates; do
    if [ -d "$d" ]; then
        SYSTEM_TEMPLATES="$d"
        break
    fi
done
if [ -z "$SYSTEM_TEMPLATES" ]; then
    # Fall back to upstream source templates (unbuilt, but the directory exists).
    UPSTREAM_ROOT="${UPSTREAM_T%/t}"
    SYSTEM_TEMPLATES="$UPSTREAM_ROOT/templates"
fi

# Write GIT-BUILD-OPTIONS (only the variables test-lib.sh actually reads).
# Values with spaces must be single-quoted so the shell evaluates them
# correctly when test-lib.sh sources this file.
PERL_BIN="$(command -v perl 2>/dev/null || echo /usr/bin/perl)"
cat > "$FAKE_BUILD_DIR/GIT-BUILD-OPTIONS" <<EOF
SHELL_PATH=/bin/sh
PERL_PATH=$PERL_BIN
GIT_TEST_TEMPLATE_DIR=$SYSTEM_TEMPLATES
GIT_TEST_GITPERLLIB=
GIT_TEST_CMP='diff -u'
NO_PERL=
NO_PYTHON=YesPlease
PAGER_ENV='LESS=FRX LV=-c'
EOF

# Stub test-tool: must be executable; test-lib.sh checks it exists.
# Curated tests may invoke test-tool for prereq checks (e.g. TIME_IS_64BIT)
# and for pack-trailer construction (`test-tool sha1 -b` from t/lib-pack.sh).
STUB_TEST_TOOL="$FAKE_BUILD_DIR/t/helper/test-tool"
# Always rewrite the stub so changes to this script take effect immediately.
cat > "$STUB_TEST_TOOL" <<'STUB'
#!/bin/sh
# Minimal test-tool stub for conformance harness.
case "$1" in
    date)
        case "$2" in
            is64bit)       date +%s | awk '{exit ($1 > 2147483647) ? 0 : 1}' ;;
            time_t-is64bit) date +%s | awk '{exit ($1 > 2147483647) ? 0 : 1}' ;;
            *) echo "test-tool stub: unimplemented date subcommand: $2" >&2; exit 1 ;;
        esac
        ;;
    path-utils)
        case "$2" in
            file-size) wc -c < "$3" ;;
            *) echo "test-tool stub: unimplemented path-utils subcommand: $2" >&2; exit 1 ;;
        esac
        ;;
    env-helper) printenv "$2" ;;
    sha1)
        # `test-tool sha1 [-b]` computes the SHA1 of stdin. With -b the digest
        # is emitted as 20 raw bytes (used by t/lib-pack.sh to build a pack
        # trailer); without -b it's a 40-char hex string + newline.
        if [ "$2" = "-b" ]; then
            openssl dgst -sha1 -binary
        else
            openssl dgst -sha1 -hex | awk '{print $NF}'
        fi
        ;;
    sha256)
        if [ "$2" = "-b" ]; then
            openssl dgst -sha256 -binary
        else
            openssl dgst -sha256 -hex | awk '{print $NF}'
        fi
        ;;
    *)
        echo "test-tool stub: unimplemented subcommand: $1" >&2
        exit 1
        ;;
esac
STUB
chmod +x "$STUB_TEST_TOOL"

export GIT_TEST_INSTALLED GIT_BUILD_DIR
GIT_TEST_INSTALLED="$(cd "$BIN_DIR" && pwd)"
GIT_BUILD_DIR="$(cd "$FAKE_BUILD_DIR" && pwd)"

# Decide what to run.
if [ "$#" -ge 1 ]; then
    TEST_NAME="$1"
    SELECTOR="${2:-}"
    TESTS_TO_RUN=("$TEST_NAME")
elif [ -n "${TESTS:-}" ]; then
    read -r -a TESTS_TO_RUN <<< "$TESTS"
    SELECTOR=""
else
    # Read curated list, ignoring blank lines and comments.
    TESTS_TO_RUN=()
    while IFS= read -r line; do
        case "$line" in
            ''|\#*) continue ;;
        esac
        TESTS_TO_RUN+=("$line")
    done < "$REPO_ROOT/conformance/tests.txt"
    SELECTOR=""
fi

if [ "${#TESTS_TO_RUN[@]}" -eq 0 ]; then
    echo "No tests to run."
    exit 0
fi

EXIT_CODE=0
# Upstream test-lib only colours output when stdout is a TTY (test-lib.sh
# `test -t 1`). Piping to tee defeats that, so when run.sh itself is on a
# terminal we run the test scripts directly and skip the per-test TAP capture
# (TAP capture is only relied on by CI for artifact upload).
INTERACTIVE=0
if [ -t 1 ]; then
    INTERACTIVE=1
fi
for test_script in "${TESTS_TO_RUN[@]}"; do
    if [ ! -f "$UPSTREAM_T/$test_script" ]; then
        echo "Skipping missing test: $test_script" >&2
        EXIT_CODE=1
        continue
    fi
    echo "=== Running $test_script ==="
    selector_args=()
    if [ -n "$SELECTOR" ]; then
        selector_args=(--run="$SELECTOR")
    fi
    if [ "$INTERACTIVE" = 1 ]; then
        if ( cd "$UPSTREAM_T" && sh "./$test_script" -v -i "${selector_args[@]}" ); then
            :
        else
            EXIT_CODE=1
        fi
    else
        tap_file="$RESULTS_DIR/$test_script.tap"
        if ( cd "$UPSTREAM_T" && sh "./$test_script" -v -i "${selector_args[@]}" ) | tee "$tap_file"; then
            :
        else
            EXIT_CODE=1
        fi
    fi
done

exit $EXIT_CODE
