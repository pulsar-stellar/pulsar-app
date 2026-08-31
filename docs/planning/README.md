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

## Where these files actually live

The working drafts were moved out of this directory on 2026-08-28 into a separate private repository, `pulsar-stellar-planning`, which holds the planning material for all four Pulsar repos under per-repo directories. Its first commit says why: cross-PC continuity.

That move is what emptied this directory. The timeline is unambiguous: the planning repo was initialised at 11:51, its copy of this README was written at 12:17, and the originals disappeared from here at 12:19. Nothing was corrupted and nothing was lost. An earlier note in this file recorded the cause as unidentified, which was wrong; it had simply not been correlated against the other repository yet.

The drafts are mirrored back into this directory so a working session can read them without leaving the repo, and they stay gitignored here. `pulsar-stellar-planning` is the origin. When anything in this directory changes, mirror the change there in the same session, or the next machine picks up a stale copy.

The durability point stands regardless. A decision that matters belongs in the ADR log, which is tracked, reviewed, and present in every clone. This directory is a convenience, and its contents live somewhere else.
