#!/usr/bin/env bash
#
# Local conformance test: fast-forward merge in both sha1 and sha256 modes.
# Exercises the new `gogit merge --ff-only` command.
#
# go-git only supports the FastForward merge strategy and gogit doesn't have
# a branch / switch command, so the two-branch setup is built directly:
# commit A, commit B on the same branch, then rewind the branch to A and
# create `feature` pointing at B by writing the ref file. Merging `feature`
# back into the branch should advance HEAD from A to B and sync the worktree.
#
# Inputs:
#   GOGIT      — path to the gogit binary
#   WORK_DIR   — scratch directory; the test owns it
#
# Unused by this test, but the runner provides them for symmetry:
#   SERVER, PORT
#
# Exit 0 on full success; non-zero with a printed reason on any failure.

set -euo pipefail

: "${GOGIT:?GOGIT not set}"
: "${WORK_DIR:?WORK_DIR not set}"

HOME="$WORK_DIR/home"
mkdir -p "$HOME"
export HOME GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=t@example.com
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=t@example.com
export GIT_AUTHOR_DATE='1700000000 +0000'
export GIT_COMMITTER_DATE='1700000000 +0000'

run_case() {
    local hash="$1"
    local repo="$WORK_DIR/repo-$hash"

    "$GOGIT" init --quiet --object-format="$hash" "$repo"
    cd "$repo"

    # Commit A on the initial branch.
    echo "$hash-A" >file
    "$GOGIT" add file
    "$GOGIT" commit -m A >/dev/null
    local branch
    branch=$(sed 's|ref: refs/heads/||' <"$repo/.git/HEAD")
    local oid_a
    oid_a=$("$GOGIT" rev-parse HEAD)

    # Commit B on the same branch (so B is a descendant of A).
    echo "$hash-B" >file
    "$GOGIT" add file
    "$GOGIT" commit -m B >/dev/null
    local oid_b
    oid_b=$("$GOGIT" rev-parse HEAD)

    # Move `feature` to B, rewind the active branch back to A. HEAD still
    # points at the active branch by name, so it now resolves to A.
    echo "$oid_b" >"$repo/.git/refs/heads/feature"
    echo "$oid_a" >"$repo/.git/refs/heads/$branch"

    local pre
    pre=$("$GOGIT" rev-parse HEAD)
    if [ "$pre" != "$oid_a" ]; then
        echo "FAIL ($hash): pre-merge HEAD is $pre, want $oid_a" >&2
        return 1
    fi

    "$GOGIT" merge --ff-only feature >"$WORK_DIR/merge-$hash.log" 2>&1

    local post
    post=$("$GOGIT" rev-parse HEAD)
    if [ "$post" != "$oid_b" ]; then
        echo "FAIL ($hash): post-merge HEAD is $post, want $oid_b" >&2
        cat "$WORK_DIR/merge-$hash.log" >&2
        return 1
    fi

    if [ "$(cat "$repo/file")" != "$hash-B" ]; then
        echo "FAIL ($hash): worktree file not updated to B content" >&2
        return 1
    fi

    cd "$WORK_DIR"
    echo "ok - ff-merge $hash (${oid_a:0:12}… -> ${oid_b:0:12}…)"
}

run_case sha1
run_case sha256
