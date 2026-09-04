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

Every thought begins with a file named:

```text
01.md
```

This is true whether the thought ultimately contains one post or becomes a thread.

A simple thought might contain:

```text
<post-directory>/
├── 01.md
├── meta.toml
└── screenshot.png
```

A thread might contain:

```text
<post-directory>/
├── 01.md
├── 02.md
├── 03.md
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

- `new [name]` creates a thought and opens its initial post for editing;
- `edit <name-or-directory>` opens all numbered Markdown files for that thought together;
- `publish <name-or-directory> [--target bluesky|x]` publishes to both destinations by
  default, or only to the selected destination;
- `status <name-or-directory>` shows local publication state and remote references.

The `edit`, `publish`, and `status` commands accept either the thought's
directory name under the archive root or its full directory path. Full paths
must identify a direct child of the configured archive root. This supports
shell tab completion without allowing a command to select a directory outside
the local archive.

When `new` is called without a name, the thought receives a timestamped
directory name. `edit` opens the numbered files in numerical order in one
editor session. None of these editing actions publish anything.

## Post files

Each numbered Markdown file represents exactly one social post.

Post filenames use two-digit sequential numbering:

```text
01.md
02.md
03.md
...
```

Files must form one contiguous sequence starting at `01.md`.

For example:

```text
01.md
02.md
03.md
```

is valid.

```text
01.md
03.md
```

is invalid.

A thought containing only `01.md` is a normal single post.

A thought containing `01.md` and later numbered files is a thread.

## Markdown

Markdown is the canonical authoring format, not the format sent directly to social platforms.

Before publication, `thought` converts the authored Markdown into appropriate social-post text.

Markdown formatting must not rely on Bluesky or X rendering Markdown syntax.

Readable textual structure such as paragraphs, lists, links, and code should be preserved where practical when producing platform text.

Image declarations are treated specially as attachments rather than post text.

## Image attachments

Images are stored directly in the same thought directory as the Markdown files.

A Markdown image declaration attaches an image to that individual post:

```markdown
Finally got the reflections working.

![close-up of the result](reflection.png)
![another view](detail.png)
```

The image syntax itself is not included in the published post text.

The Markdown alt text becomes the attachment's alt text where supported by the destination platform.

### Attachment position

Image declarations must form a trailing attachment block at the end of each Markdown file.

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

When a thought is published, `thought` discovers all contiguous numbered Markdown files beginning with `01.md`.

It then publishes them in numerical order.

For a single-post thought:

```text
01.md
```

is published as a top-level post.

For a thread:

```text
01.md
02.md
03.md
```

the resulting platform structure is:

```text
01: top-level post
02: reply to 01
03: reply to 02
```

Each destination platform receives its own correctly linked thread.

The relationship is derived entirely from the numbered files. Authors do not manually specify reply IDs.

### Publication states

Each numbered post has an independent state for each destination:

- `pending`: the post is eligible to be published;
- `publishing`: a publication attempt started, but its final remote outcome is
  not known;
- `published`: the remote post was accepted and its identifier is recorded;
- `failed`: a transient or otherwise retryable problem prevented completion;
- `rejected`: the post was deterministically refused or failed validation and
  must not be retried automatically.

The normal lifecycle is `pending` → `publishing` → `published`, `failed`, or
`rejected`. A `failed` post may be retried by publishing again. A `rejected`
post becomes eligible only after the user fixes the problem and manually edits
its metadata state back to `pending`.

If publication is interrupted while a post is `publishing`, the user must
check the destination before editing its metadata. The user may record it as
`published` with its remote details, or change it to `failed` or `pending` if
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

01.md        ✓          ✓
02.md        ✓          ✗
03.md        ✓       pending
```

is a valid state.

X may continue successfully even though Bluesky encountered a failure.

## Thread failure behavior

Within a given platform, publication stops at the first failed post.

If:

```text
01.md
02.md
03.md
```

are being published and `02.md` fails on Bluesky:

- Bluesky `01.md` remains published.
- Bluesky `02.md` is recorded as failed.
- Bluesky `03.md` is not attempted.
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
01.md   posted
02.md   failed
03.md   pending
```

a later retry should:

1. retry `02.md`;
2. if successful, publish `03.md` as a reply to it;
3. stop again if another failure occurs.

## Published posts are immutable

Once a particular numbered post has successfully published to a platform, `thought` treats that remote publication as immutable.

Later changes to the corresponding local Markdown or images do not update the remote post.

Running publish again must not duplicate or replace a successfully published remote post.

The user may still edit their local archive freely. Those edits only change the local canonical record.

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

For each numbered post and each target platform, the metadata records enough information to determine:

- whether publication is pending, publishing, successful, failed, or
  rejected;
- when publication was attempted;
- when successful publication occurred;
- the destination platform's identifier for the published post;
- the public link to the published post when available;
- any information required to correctly link subsequent posts in the same thread;
- the most recent publication error when publication failed.

Posts that have not yet been attempted remain pending.

Metadata is intentionally editable with ordinary text tools so the user can
recover an interrupted publication or reset a rejected post after fixing it.

`thought` must not silently overwrite malformed metadata or infer that an
unresolved `publishing` entry is safe to retry.

The exact representation of platform-specific identifiers may differ where required by the platform, but the metadata must retain enough information for subsequent posts in the same local thread to reply correctly.

## Validation

Before attempting a particular post on a platform, `thought` validates the local source sufficiently to avoid clearly invalid publication attempts.

Validation includes:

- numbered Markdown files forming a contiguous sequence;
- attachment placement rules;
- attachment path safety;
- attachment existence;
- content constraints required by the target platform.

A validation problem that applies only to one destination platform does not need to prevent another valid destination from publishing.

Validation errors must identify the affected post and the reason publication cannot proceed.

## Archive durability

A thought directory contains everything needed to understand the authored thought and its publication history:

```text
NN.md       authored posts
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
