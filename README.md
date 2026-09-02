# thought

**write markdown thoughts locally, then publish them deliberately to bluesky and x**

keep source files and publication history in a readable local archive.

> early development: the command-line surface is scaffolded, but the product
> workflows are not implemented yet.

## install

the project is not released as a package yet. install the current command
from the main branch with:

```sh
go install github.com/samm81/thought/cmd/thought@main
```

requires Go 1.27.1.

## quickstart

inspect the current command surface:

```sh
thought --help
```

the planned commands are `new`, `edit`, `publish`, and `status`.

## workflow

- **draft locally:** keep each thought in its own directory under `~/thoughts`.
- **write in markdown:** use numbered files such as `01.md`, `02.md`, and
  `03.md` for a single post or a thread.
- **edit without publishing:** change local files freely; editing never makes
  a network request.
- **publish explicitly:** send a thought to both bluesky and x, or select one
  destination.
- **recover from partial publication:** keep per-post, per-target state in
  human-readable TOML so interrupted work can be checked and repaired.

## archive

```text
~/thoughts/daily-note/
├── 01.md
├── 02.md
├── image.png
└── meta.toml
```

each numbered markdown file becomes one social post. trailing markdown image
declarations become local attachments. `meta.toml` records publication state,
remote identifiers, thread relationships, and errors.

the archive remains useful with ordinary filesystem and text tools. the
[product specification](spec/SPEC.md) defines the file format, attachment
rules, thread behavior, and recovery semantics.
