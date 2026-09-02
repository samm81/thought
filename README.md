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
export VISUAL=true

thought new daily-note
thought status daily-note
```

`new` creates `01.md` and `meta.toml`, then opens the configured editor.
`true` keeps this example editor-free; use `VISUAL` or `EDITOR` to choose an
editor for normal use.

## What you can do

- **draft locally:** write one post per numbered Markdown file.
- **build threads:** publish `01.md`, `02.md`, and later files as a linked thread.
- **attach images:** put trailing Markdown image declarations in the thought directory.
- **publish explicitly:** send a thought to both destinations or select one target.
- **recover safely:** inspect and repair per-post, per-target state in `meta.toml`.

## Commands

| command | description |
| --- | --- |
| `thought new [name]` | create a thought and edit its initial post |
| `thought edit <name>` | edit all numbered posts in one editor session |
| `thought publish <name>` | publish to bluesky and x |
| `thought publish <name> --target bluesky` | publish only to bluesky |
| `thought publish <name> --target x` | publish only to x |
| `thought status <name>` | show local states, remote references, and recovery actions |

## Configuration

| variable | purpose |
| --- | --- |
| `THOUGHT_HOME` | archive root; defaults to `~/thoughts` |
| `VISUAL` or `EDITOR` | editor command; defaults to `vi` |
| `THOUGHT_XPOST` | `xpost` bridge executable; defaults to `xpost` on `PATH` |
| `XPOST_BLUESKY_HANDLE` | bluesky handle |
| `XPOST_BLUESKY_APP_PASSWORD` | bluesky app password |
| `XPOST_BLUESKY_PDS_URL` | optional bluesky PDS URL |
| `XPOST_TWITTER_CONSUMER_KEY` | X consumer key |
| `XPOST_TWITTER_CONSUMER_SECRET` | X consumer secret |
| `XPOST_TWITTER_ACCESS_TOKEN` | X access token |
| `XPOST_TWITTER_ACCESS_TOKEN_SECRET` | X access-token secret |

build the bridge from the checked-out submodule when it is not already
installed:

```sh
go -C third_party/xpost build -o "$HOME/.local/bin/xpost" .
```

## Archive

```text
~/thoughts/daily-note/
├── 01.md
├── 02.md
├── image.png
└── meta.toml
```

the local archive is canonical. editing never publishes. successful posts are
never sent again, and transient failures remain retryable. an interrupted
`publishing` entry must be checked at the destination before its TOML state is
manually repaired.

## License

the repository has not declared a license yet.
