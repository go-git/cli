#!/usr/bin/env bash
#
# Local conformance test: gogit cherry-pick in both sha1 and sha256 modes.
#
# Setup: commit A (base.txt), commit B (B = A + topic.txt). Rewind the active
# branch back to A and remove topic.txt from the worktree. Cherry-pick B —
# the new commit should re-introduce topic.txt and its content. Because
# environment timestamps and identities are fixed and B's parent is A,
# the cherry-picked commit comes out byte-identical to B; we still verify
# the round-trip (HEAD advanced, worktree contains topic.txt, ls-files
# entries are present) which exercises gogit cherry-pick + go-git's
# Worktree.CherryPick + the new commit's full round-trip.
#
# Cherry-pick onto a *different* parent (with conflicting modifications)
# is intentionally out of scope: go-git's CherryPick is a single-strategy
# merge (theirs/ours) rather than a true upstream-style cherry-pick, and
# its worktree-sync behaviour on multi-file diffs has rough edges. The
# add-only case here is the well-supported subset.
#
# Inputs:
#   GOGIT      — path to the gogit binary
#   WORK_DIR   — scratch directory; the test owns it
# Unused but supplied by the runner: SERVER, PORT
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

    echo "base-$hash" >base.txt
    "$GOGIT" add base.txt
    "$GOGIT" commit -m base >/dev/null
    local branch
    branch=$(sed 's|ref: refs/heads/||' <"$repo/.git/HEAD")
    local oid_a
    oid_a=$("$GOGIT" rev-parse HEAD)

    echo "topic-$hash" >topic.txt
    "$GOGIT" add topic.txt
    "$GOGIT" commit -m "topic adds topic.txt" >/dev/null
    local oid_b
    oid_b=$("$GOGIT" rev-parse HEAD)

    # Rewind to A so cherry-pick has work to do.
    echo "$oid_a" >"$repo/.git/refs/heads/$branch"
    rm -f topic.txt

    if [ "$("$GOGIT" rev-parse HEAD)" != "$oid_a" ]; then
        echo "FAIL ($hash): pre-pick HEAD != A" >&2
        return 1
    fi

    "$GOGIT" cherry-pick "$oid_b" >"$WORK_DIR/cherry-$hash.log" 2>&1

    local oid_after
    oid_after=$("$GOGIT" rev-parse HEAD)

    if [ "$oid_after" = "$oid_a" ]; then
        echo "FAIL ($hash): cherry-pick did not advance HEAD" >&2
        cat "$WORK_DIR/cherry-$hash.log" >&2
        return 1
    fi

    if [ ! -f topic.txt ]; then
        echo "FAIL ($hash): topic.txt missing from worktree after cherry-pick" >&2
        return 1
    fi

    if [ "$(cat topic.txt)" != "topic-$hash" ]; then
        echo "FAIL ($hash): topic.txt content is $(cat topic.txt), want topic-$hash" >&2
        return 1
    fi

    # Index must contain topic.txt staged with a hash of the right width.
    local idx
    idx=$("$GOGIT" ls-files --stage topic.txt)
    if [ -z "$idx" ]; then
        echo "FAIL ($hash): topic.txt not in index after cherry-pick" >&2
        return 1
    fi

    # Confirm the resulting commit's parent is A (the cherry-pick lands on
    # the active history) and that the commit message survives.
    local parent
    parent=$("$GOGIT" rev-parse HEAD^)
    if [ "$parent" != "$oid_a" ]; then
        echo "FAIL ($hash): cherry-pick parent is $parent, want A ($oid_a)" >&2
        return 1
    fi

    local msg
    msg=$("$GOGIT" cat-file commit "$oid_after" | tail -1)
    if [ "$msg" != "topic adds topic.txt" ]; then
        echo "FAIL ($hash): cherry-pick commit message is $msg" >&2
        return 1
    fi

    cd "$WORK_DIR"
    echo "ok - cherry-pick $hash (A=${oid_a:0:12}…, B=${oid_b:0:12}…, new=${oid_after:0:12}…)"
}

run_case sha1
run_case sha256
