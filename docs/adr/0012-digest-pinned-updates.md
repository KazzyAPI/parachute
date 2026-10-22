# ADR-0012: Digest-pinned updates

- **Status:** Accepted
- **Date:** 2026-10-23

## Context
Detection resolves a tag (for example `app:1`) to a manifest digest. The delay (POL-002), approval (POL-006, API-007), and failed-digest hold (POL-010) are all decided about that digest. If the update then pulls the **tag**, the registry can serve a different image than the one the policies cleared: a tag pushed between detection and pull (by accident or by an attacker with push access) skips the delay and the approval entirely. Registry responses are untrusted ([glossary: Trust boundaries](../specs/glossary.md#trust-boundaries)), so the gap between "checked" and "installed" must be closed by construction, not by timing.

## Decision
- Detection records exactly one **candidate digest** per detected update, validated as `sha256:[0-9a-f]{64}` and, for a `GET`, checked against the body's hash (DET-008).
- Every policy decision and stored record (first-seen time, approval, failed-digest hold) is keyed on that digest.
- An update pulls `<repository>@<candidate digest>`, never `<repository>:<tag>`.
- After the pull, Parachute verifies that the pulled image's repository digests contain `<repository>@<candidate digest>`. If they do not, the transaction ends `failed` with reason `pull_failed` before any container is stopped.
- Parachute then points the original tag reference at the verified image (what a tag pull would have done) and creates the replacement with the original tag reference, so its configured image is unchanged and the next cycle's detection works as before (a replacement configured as `repo@digest` would be `digest_pinned` forever, DET-005). If the created replacement's image ID differs from the verified image ID (the tag was moved locally in between), the replacement is removed and the transaction rolls back.
- The same applies to self-update (UPD-018).

## Consequences
- A tag moved after detection is only seen by the next cycle; it starts its own delay and needs its own approval.
- An approval or delay can never be satisfied by one image and applied to another.
- Parachute writes one local tag per update. The local tag and the running image agree after a commit, as with a tag pull.
- A registry that cannot serve a manifest by digest cannot be updated from; this is a `pull_failed`, not a silent fallback to the tag.
- Multi-platform images: the candidate digest is the index digest the tag points to; the engine selects the platform manifest from it, as it does for a tag pull.

## Alternatives considered
- **Pull the tag, then compare the result with the candidate digest.** Detects the race but turns it into a spurious failure and still downloads an unapproved image; rejected.
- **Pull the tag and trust the short window.** The window is as long as the delay and approval holds (days), not seconds; rejected.
- **Create the replacement as `repo@digest` and leave the tag alone.** Avoids writing a local tag, but changes the container's configured image and makes every later cycle report it `digest_pinned` (DET-005); rejected.
- **Content trust / signature verification.** Answers a different question (who published the image), not "is this the image that was approved"; may be added later on top of this decision.

## Specs affected
DET-007, DET-008, POL-002, POL-006, POL-010, API-007, UPD-001, UPD-005, UPD-014, UPD-018.
