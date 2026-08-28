# Planning directory

The contents of this directory are not tracked. Only this README is.

`docs/planning/` holds maintainer-local working drafts: system prompts, roadmaps, locked wording, and cross-repo ADR drafts that cover both `pulsar-core` and `pulsar-app`. They are drafts. They go stale, they contradict each other while a decision is being made, and they are edited without the commit discipline the rest of this repo follows. Publishing them would present working notes as project documentation.

`pulsar-core` applies the same policy, with `docs/planning/` in its `.gitignore` and its cross-repo standards promoted to `docs/` alongside it. `requirements.md` and `roadmap-product.md` were promoted out of this directory for the same reason: they answer what the project promises and where it is going, which are contributor questions rather than maintainer notes.

## What is authoritative instead

Everything a contributor needs is tracked:

| Question | Tracked answer |
|---|---|
| What does the project promise, and to what standard? | `docs/requirements.md` |
| Where is the project going? | `docs/roadmap-product.md` |
| How do I set up, commit, test, and style code here? | `CONTRIBUTING.md` |
| Why is the project built this way? | `.agent/decisions.md`, the ADR log |
| What is the state of the work and what comes next? | `.agent/context.md` |
| What do the domain terms mean? | `.agent/glossary.md` |
| What is in scope for a security report? | `SECURITY.md` |
| What is this project and how is it laid out? | `README.md` |

If a planning draft and a tracked file disagree, the tracked file wins. A decision that matters belongs in the ADR log, where it is reviewed and permanent, not in a draft here.

## For the maintainer

Files expected in this directory locally, none of them tracked:

- `system-prompt.md`, the build guide, with the numbered build sequence in section 12
- `locked-description.md`, the canonical one-paragraph project description
- `adr-006-stellar-optics.md`, `adr-007-two-toolchain.md`, cross-repo ADR drafts

A superseded section in a draft gets a stale marker at the top of that section pointing at the tracked ADR that replaced it, rather than being rewritten in place.

## This directory has lost its contents once

On 2026-08-28 every untracked file here vanished. `README.md` survived, because it is tracked, but it was rewritten with identical content at the same moment, and the `docs/` directory's own mtime moved with it. That combination is the signature of a checkout restoring the tracked file after something removed its untracked siblings.

What was ruled out: no git operation in the reflog corresponds to the time, no configured hook touches this repository (the `PreCompact` hook writes only to `~/.claude/sessions`), and nothing else in the working tree was modified. The cause is still unidentified, and file timestamps disagree with commit timestamps by several days, so correlating precisely is not reliable.

The practical consequence is that this directory is not storage. It already says a decision that matters belongs in the ADR log; treat that as a durability rule and not only an editorial one. Anything here is one unexplained event away from being gone, and the ADR log, `.agent/`, and `docs/` are the parts that survive.
