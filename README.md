# Go Git CLI

This provides a Go Git CLI binary using `go-git`.

## Purpose

The CLI exposes key features from `go-git` so it can be assessed and tested
against upstream Git on a like-for-like basis. Each subcommand is a thin
wrapper over a `go-git` API, so the behaviour exercised by `gogit <command>`
is the behaviour that `go-git` consumers will see.

## Bridging gaps in `go-git`

When upstream Git tests require a feature that `go-git` does not yet
expose, the gap is filled temporarily in `internal/<package>/`. This
directory is a staging area whose layout mirrors the eventual go-git
home (e.g. `internal/plumbing/format/idxfile` is shaped for upstream's
`plumbing/format/idxfile`). Not everything that lives here belongs in
`go-git` — some bits are test-only shims — but the layering keeps
upstream tests that exercise the bulk of `go-git` unblocked while the
upstreaming conversation is in progress.

## Installation

```bash
go install github.com/go-git/cli/cmd/gogit
```

## Usage

```bash
gogit <command> [flags]
```

## Conformance testing

`make conformance` runs a curated set of upstream Git tests against
`gogit`, using upstream's own `t/` harness. The runner
(`conformance/run.sh`) builds `gogit`, symlinks it as `git` in a
staging directory, and points `GIT_TEST_INSTALLED` there so the
upstream framework picks our binary up unchanged.

The upstream test source comes from either:

- A local checkout pointed at by `$GIT_SRC` (e.g.
  `GIT_SRC=$HOME/git/go-git/git make conformance`), or
- A shallow clone of `github.com/git/git`'s default branch, re-fetched
  on every run so upstream behaviour drift surfaces immediately.

The set of curated tests is `conformance/tests.txt` — one filename per
line, `#` comments are ignored:

```
t2008-checkout-subdir.sh
t5308-pack-detect-duplicates.sh
t5325-reverse-index.sh
```

Individual tests can be run directly:

```bash
./conformance/run.sh t2008-checkout-subdir.sh        # one test, all cases
./conformance/run.sh t2008-checkout-subdir.sh 1,3    # one test, selected cases
```

`$GO_GIT_REF` (commit SHA, tag, or branch) overrides the `go.mod` pin
for a single run so the gate can be evaluated against an in-flight
`go-git` change.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
