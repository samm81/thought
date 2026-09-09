# thought

## Purpose

`thought` is a small, terminal-first personal publishing tool for quickly writing and cross-posting short updates to Bluesky and X.

It is intended for informal work-in-progress notes, small observations, screenshots, experiments, and other things that are too small to justify a full blog post.

The local filesystem is the canonical record. Bluesky and X are publication destinations, not the source of truth.

## Users

The initial product is for a single user publishing from their own machine.

It is optimized for someone comfortable with terminal tools and editing Markdown files directly.

## Product principles

### Local-first and human-readable

All authored content, attachments, and publication state must remain available as ordinary human-readable files.

The user must be able to inspect, edit, search, copy, back up, or version-control their archive without using `thought`.

No database is required to understand or recover the archive.

### Editing never publishes

Editing content must never cause a network side effect.

Creating or editing a draft only changes local files. Publishing is always a separate explicit action.

### Social platforms are outputs

The local archive is canonical.

Publishing sends local content to Bluesky and X, but `thought` does not attempt to keep already-published posts synchronized with later local edits.

### Prefer a small workflow over a social-media client

`thought` is for composing and publishing.

It does not provide feeds, notifications, engagement dashboards, or a replacement interface for interacting with Bluesky or X.

The user can use the native apps for those activities.

## Scope

### In scope

- Creating local draft directories.
- Editing drafts using the user's configured editor.
- Authoring posts in Markdown.
- Attaching local images to posts.
- Publishing to Bluesky and X.
- Cross-posting the same thought to both platforms.
- Publishing multi-post threads.
- Recording publication status and remote identifiers locally.
- Recovering from partial publication failures.
- Retrying posts that have not successfully published.

### Out of scope

- Notifications.
- Background daemons.
- Reading timelines or feeds.
- Likes, reposts, follows, or other social interactions.
- Replying to arbitrary existing Bluesky or X posts.
- Replying to the user's previously published posts outside the thread currently being created.
- Editing already-published remote posts.
- Deleting remote posts.
- Synchronizing later local edits to already-published posts.
- Automatically mapping an X post to an equivalent Bluesky post.
- Managing image-paste behavior inside a particular editor.

## Drafts

Each thought is represented by its own directory.

Every thought contains one authored Markdown file named:

```text
post.md
```

This file contains one post or a complete thread.

A simple thought might contain:

```text
<post-directory>/
├── post.md
├── meta.toml
└── screenshot.png
```

A thread might contain:

```text
<post-directory>/
├── post.md
├── meta.toml
├── screenshot.png
└── detail.png
```

Running `thought` to create or edit a draft may open the user's configured editor, but leaving the editor only saves the draft.

It does not publish it.

### User workflow

The archive root defaults to `~/thoughts`. The user may change it with
`THOUGHT_HOME`.

The product provides these explicit commands:

- `new [name]` creates a thought and opens `post.md` for editing;
- `edit [name-or-directory]` opens `post.md` for editing, defaulting to the most recent thought;
- `publish [name-or-directory] [--target bluesky|x]` publishes to both destinations by
  default, or only to the selected destination. When the thought is omitted,
  `thought` selects the most recent thought and asks for confirmation before
  publishing it;
- `status [name-or-directory]` shows local publication state and remote references,
  defaulting to the most recent thought.
- `completion zsh` prints zsh completion that suggests thought names from the
  configured archive root.

The `edit`, `publish`, and `status` commands accept either the thought's
directory name under the archive root or its full directory path. Full paths
must identify a direct child of the configured archive root. This supports
shell tab completion without allowing a command to select a directory outside
the local archive. When `edit` or `status` is called without a thought name or
directory, it selects the most recent thought.

When `publish` is called without a thought name or directory, it selects the
most recent thought. It does not ask to publish a thought whose selected
destinations are already published; the user must provide an explicit thought
to republish or inspect.

When `new` is called without a name, the thought receives a timestamped
directory name. None of these editing actions publish anything.

## Thread structure

Each section of `post.md` represents exactly one social post.

Sections are separated by a line containing only `---`:

```markdown
the first post

---

the second post

---

the third post
```

Sections are published in the order in which they appear in the file. A
`---` line inside a fenced code block is part of that post and does not split
the thread.

The file must contain at least one post section. Empty sections between
separators are invalid. A newly created empty `post.md` remains an editable
draft and must be filled before it can be published.
Separate numbered Markdown files are not supported.

The position of a section is its local post number for publication state and
thread recovery. The first section is post `01`, the second is post `02`, and
so on.

## Markdown

Markdown is the canonical authoring format, not the format sent directly to social platforms.

Before publication, `thought` converts the authored Markdown into appropriate social-post text.

Markdown formatting must not rely on Bluesky or X rendering Markdown syntax.

Readable textual structure such as paragraphs, lists, links, and code should be preserved where practical when producing platform text.

Image declarations are treated specially as attachments rather than post text.

## Image attachments

Images are stored directly in the same thought directory as `post.md`.

A Markdown image declaration attaches an image to that individual post section:

```markdown
Finally got the reflections working.

![close-up of the result](reflection.png)
![another view](detail.png)
```

The image syntax itself is not included in the published post text.

The Markdown alt text becomes the attachment's alt text where supported by the destination platform.

### Attachment position

Image declarations must form a trailing attachment block at the end of each
post section.

Once the first image declaration appears, only additional image declarations and whitespace may follow.

This is valid:

```markdown
Finally got this working.

Turns out the bug was simpler than I expected.

![main result](result.png)
![detail](detail.png)
```

This is invalid:

```markdown
Finally got this working.

![main result](result.png)

The bug was in the framebuffer.
```

Inline images are deliberately unsupported because the destination social posts do not display attachments inline with arbitrary text positions.

### Attachment paths

Attachment references must:

- use local relative paths;
- refer to files inside the same thought directory;
- resolve to existing regular files;
- remain inside the thought directory after path resolution.

Remote image URLs are not valid attachments.

Absolute paths are not valid attachments.

Paths that escape the thought directory are not valid, including traversal through `..` or equivalent filesystem indirection.

The archive must therefore remain self-contained.

## Publishing

Publishing is an explicit operation separate from editing.

When a thought is published, `thought` reads the post sections in `post.md`.
It publishes them in file order.

For a single-post thought:

```markdown
one post
```

the section is published as a top-level post.

For a thread:

```markdown
first post

---

second post

---

third post
```

the resulting platform structure is:

```text
01: top-level post
02: reply to 01
03: reply to 02
```

Each destination platform receives its own correctly linked thread.

The relationship is derived entirely from the section order. Authors do not
manually specify reply IDs.

### Publishing configuration

Publishing credentials for Bluesky and X are kept in xpost's human-readable
TOML configuration file rather than in the thought archive. The default file
is `~/.config/xpost/config.toml`; xpost may also be given an explicit config
file path.

The configuration contains independent sections for each publishing
destination. A missing default file is allowed, but publishing must report
which destination configuration is missing. Existing environment variables may
override file values for temporary use and compatibility with scripts.

### Publication states

Each post section has an independent state for each destination:

- `pending`: the post is eligible to be published;
- `publishing`: a publication attempt is active and its final remote outcome is
  not known yet;
- `published`: the remote post was accepted and its identifier is recorded;
- `failed`: a transient or otherwise retryable problem prevented completion;
- `rejected`: the post was deterministically refused or failed validation and
  must not be retried automatically.

The normal lifecycle is `pending` → `publishing` → `published`, `failed`, or
`rejected`. `thought` waits for the xpost bridge to return a terminal result or
to time out. Transport errors, including a canceled request that returns
control to `thought`, are recorded as `failed` before the command exits. A
`failed` post may be retried by publishing again. A `rejected` post becomes
eligible only after the user fixes the problem and manually edits its metadata
state back to `pending`.

If the process is forcibly terminated while a post is `publishing`, the user
must check the destination before editing its metadata. The user may record it
as `published` with its remote details, or change it to `failed` or `pending` if
the destination did not accept it. `thought` does not automatically retry an
unresolved `publishing` post because the destination may already contain it.

There is no separate reconciliation command. The human-readable TOML
metadata is the recovery surface.

## Platform independence

Bluesky and X publication state are tracked independently.

A failure on one platform must not undo successful publication to the other platform.

For example:

```text
             X       Bluesky

post 01      ✓          ✓
post 02      ✓          ✗
post 03      ✓       pending
```

is a valid state.

X may continue successfully even though Bluesky encountered a failure.

## Thread failure behavior

Within a given platform, publication stops at the first failed post.

If the source contains:

```markdown
first post

---

second post

---

third post
```

and the second post fails on Bluesky:

- Bluesky post `01` remains published.
- Bluesky post `02` is recorded as failed.
- Bluesky post `03` is not attempted.
- Other platforms may continue independently.

The same stop rule applies when an entry is `rejected` or remains
`publishing`. A later post must not be published without a successfully known
parent.

Later posts must not be published by skipping over a failed parent, because doing so would produce a different thread structure.

## Retry behavior

Running publication again on a partially published thought continues from the unfinished state.

Already successfully published posts are not published again.

Failed posts may be retried.

Transient network errors, timeouts, temporary service failures, and similar
uncertain transport problems are recorded as `failed` and are eligible for a
later retry. A known validation or provider rejection is recorded as
`rejected` and requires an explicit user reset to `pending` after the problem
is fixed.

Posts blocked behind a failed earlier thread entry become eligible only after the required parent has successfully published.

For example, if Bluesky has:

```text
post 01   posted
post 02   failed
post 03   pending
```

a later retry should:

1. retry post `02`;
2. if successful, publish post `03` as a reply to it;
3. stop again if another failure occurs.

## Source and published-post immutability

Once a particular post section has successfully published to a platform,
`thought` treats that remote publication as immutable.

Later changes to the local source or images do not update the remote post.

Running publish again must not duplicate or replace a successfully published remote post.

Before the first publication request in a run, `thought` stores a hash of the
complete `post.md` file. It checks the file against that hash before each later
publication request. If the file changes, `thought` stops before sending the
next request, even if the change only reorders sections or changes whitespace.

The file can still be edited by ordinary tools, but a run that has started
cannot continue with changed source. The user must inspect the destinations
and manually recover the publication metadata before intentionally changing
the source for a new run.

Remote editing or deletion can be performed manually using the platform's own app and is outside `thought`'s responsibility.

If a transient error occurs after a destination accepts a post but before
`thought` receives the success response, a retry may create a duplicate. The
`publishing` state and manual metadata recovery exist to let the user check
the destination before retrying that case.

## Publication metadata

Every thought directory contains:

```text
meta.toml
```

This file records publication state separately from authored content.

It must remain human-readable.

For each post section and each target platform, the metadata records enough information to determine:

- whether publication is pending, publishing, successful, failed, or
  rejected;
- when publication was attempted;
- when successful publication occurred;
- the destination platform's identifier for the published post;
- the public link to the published post when available;
- any information required to correctly link subsequent posts in the same thread;
- the most recent publication error when publication failed.

When publication has begun, metadata also records the hash of the complete
`post.md` source that the run is publishing.

Posts that have not yet been attempted remain pending.

Metadata is intentionally editable with ordinary text tools so the user can
recover an interrupted publication or reset a rejected post after fixing it.

`thought` must not silently overwrite malformed metadata or infer that an
unresolved `publishing` entry is safe to retry.

The exact representation of platform-specific identifiers may differ where required by the platform, but the metadata must retain enough information for subsequent posts in the same local thread to reply correctly.

## Validation

Before making any publication network request, `thought` preflights every
unsent post section for every selected destination. A destination's complete
thread must pass validation before its first post is sent. Preflight validation
must not log in to a provider, upload media, or make any other publication
network request.

Validation includes:

- `post.md` containing valid, non-empty post sections in order;
- `---` separators inside fenced code remaining part of the post;
- attachment placement rules;
- attachment path safety;
- attachment existence;
- content constraints required by the target platform.

A validation problem that applies only to one destination platform does not
prevent another destination that passes its complete preflight from publishing.

Validation errors must identify the affected post and the reason publication cannot proceed.

## Archive durability

A thought directory contains everything needed to understand the authored thought and its publication history:

```text
post.md     authored post or thread
image files attachments
meta.toml   publication state and remote references
```

The archive must remain useful without `thought`.

Users should be able to use ordinary filesystem and text tools to inspect, grep, back up, copy, synchronize, or version-control it.

No hidden database or external service is the authoritative source for authored content or publication history.

## Future directions

Possible future capabilities include additional publication destinations, generating a personal web archive from the local thought collection, and richer publication workflows.

These are not requirements for the current product.

The current file format and local-first model should not unnecessarily prevent additional publishing destinations from being added later.
