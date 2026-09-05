<div align="center">

# thought

**write markdown locally and publish it deliberately to bluesky and x**

keep drafts, attachments, thread structure, and publication history in a readable local archive.

</div>

## Install

```sh
go install github.com/samm81/thought/cmd/thought@main
```

requires Go 1.27.1. publishing also requires the maintained `xpost` bridge
on `PATH`, or its path in `THOUGHT_XPOST`.

## Quickstart

```sh
archive_root="$(mktemp -d)"
export THOUGHT_HOME="$archive_root"

thought new daily-note
thought status daily-note
```

`new` creates `post.md` and `meta.toml`, then opens the configured editor.
`true` keeps this example editor-free; use `VISUAL` or `EDITOR` to choose an
editor for normal use.

## What you can do

- **draft locally:** write one post, or a whole thread separated by `---`, in `post.md`.
- **build threads:** put posts in publication order in one `post.md` file.
- **attach images:** put trailing Markdown image declarations in the thought directory.
- **publish explicitly:** send a thought to both destinations or select one target.
- **recover safely:** inspect and repair per-post, per-target state in `meta.toml`.

## Commands

| command | description |
| --- | --- |
| `thought new [name]` | create a thought and edit its initial post |
| `thought edit <name-or-directory>` | edit the thought's `post.md` source |
| `thought publish [name-or-directory]` | publish an explicit thought, or confirm the most recent thought when omitted |
| `thought publish <name-or-directory> --target bluesky` | publish only to bluesky |
| `thought publish <name-or-directory> --target x` | publish only to x |
| `thought status <name-or-directory>` | show local states, remote references, and recovery actions |

## Configuration

| setting | purpose |
| --- | --- |
| `~/.config/xpost/config.toml` | xpost provider credentials and settings; override with `xpost --config PATH` |
| `THOUGHT_HOME` | archive root; defaults to `~/thoughts` |
| `VISUAL` or `EDITOR` | editor command; defaults to `vi` |
| `THOUGHT_XPOST` | `xpost` bridge executable; defaults to `xpost` on `PATH` |

`xpost` still accepts its `XPOST_*` environment variables as compatibility
overrides. See the [xpost configuration example](third_party/xpost/README.md#configuration)
for the TOML format.

build the bridge from the checked-out submodule when it is not already
installed:

```sh
go -C third_party/xpost build -o "$HOME/.local/bin/xpost" .
```

the provider compatibility snapshots used by the bridge are recorded in
[`third_party/xpost/UPSTREAM_COMPATIBILITY.md`](third_party/xpost/UPSTREAM_COMPATIBILITY.md).

## Archive

```text
~/thoughts/daily-note/
├── post.md
├── image.png
└── meta.toml
```

the local archive is canonical. editing never publishes. `---` on its own line
separates posts in a thread; the separator is ignored inside fenced code.
`thought` validates every post before sending any of them. once a publication
run starts, the complete `post.md` source is locked by a stored hash: changing
it stops any remaining publication until the metadata is manually recovered.
successful posts are never sent again, and transient failures remain retryable.
a forcibly terminated process may leave a `publishing` entry that must be
checked at the destination before its TOML state is manually repaired.
commands accept either a thought name or its full directory path, so shell tab
completion works with the archive path. when `thought publish` has no thought
argument, it selects the most recent thought and asks for confirmation before
publishing; a fully published latest thought must be named explicitly.

## License

the repository has not declared a license yet.
