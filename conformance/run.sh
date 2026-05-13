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

# Resolve upstream tests source. The clone/fetch/reset commands below use the
# HOST'S `git` binary, not gogit, by design:
#   - gogit is not yet built at this point in the script.
#   - We are cloning over HTTPS from github.com; this is test infrastructure,
#     not part of what is being tested.
#   - We want canonical Git semantics for the test sources themselves so any
#     gogit divergence shows up in test results rather than in test setup.
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

# Build the Go-based test-tool helper at the location test-lib.sh expects.
# It supplies the subcommands curated tests invoke (genrandom, delta, sha1/2,
# date, path-utils, env-helper). Rebuilt on every run so source changes take
# effect without manual cache invalidation, matching the policy used for
# gogit itself.
STUB_TEST_TOOL="$FAKE_BUILD_DIR/t/helper/test-tool"
# `go build` refuses to overwrite a non-Go-built file at the output path, so
# clear any pre-existing stub (typically a leftover from an earlier shell-stub
# version of this script) before invoking the build.
rm -f "$STUB_TEST_TOOL"
( cd "$REPO_ROOT" && go build -o "$STUB_TEST_TOOL" ./cmd/gogit-test-tool )

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

# Verbose mode keeps upstream's full test output (the -v `expecting success...`
# trace plus every `ok N - title` and per-test command output). Summary mode
# (the default) keeps the same upstream invocation but filters stdout down to
# harness headers, `not ok` lines, the per-script summary line, and the plan.
# CI sets CONFORMANCE_VERBOSE=1 so its captured logs stay inspectable.
VERBOSE=0
case "${CONFORMANCE_VERBOSE:-}" in
    1|true|TRUE|yes|YES) VERBOSE=1 ;;
esac

# summary_filter is the identity function in verbose mode and a `grep` keeping
# only summary-worthy lines otherwise.
summary_filter() {
    if [ "$VERBOSE" = 1 ]; then
        cat
    else
        grep -E '^(=== Running|not ok|# (passed|failed|fixed|still have|skip)|1\.\.[0-9]+)' || true
    fi
}

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
    # 2>&1 inside the subshell so the upstream `-v` trace, which goes to the
    # test script's stderr, also reaches summary_filter; otherwise it would
    # bypass the grep and leak straight to our stderr in summary mode.
    if [ "$INTERACTIVE" = 1 ]; then
        if ( cd "$UPSTREAM_T" && sh "./$test_script" -v -i ${selector_args[@]+"${selector_args[@]}"} 2>&1 ) | summary_filter; then
            :
        else
            EXIT_CODE=1
        fi
    else
        tap_file="$RESULTS_DIR/$test_script.tap"
        if ( cd "$UPSTREAM_T" && sh "./$test_script" -v -i ${selector_args[@]+"${selector_args[@]}"} 2>&1 ) | tee "$tap_file" | summary_filter; then
            :
        else
            EXIT_CODE=1
        fi
    fi
done

exit $EXIT_CODE
