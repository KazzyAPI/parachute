# ADR-0007: Safe by default

- **Status:** Accepted
- **Date:** 2026-10-22

## Context
A tool that updates containers can destroy data. Defaults that optimise for "everything stays current" with no guard make operators try automatic updates once and turn them off. Trust is our product.

## Decision
1. **Dry-run first.** Parachute starts unarmed: it reports what it *would* update and changes nothing. It is armed only by its settings, read at every start: `--armed` / `PARACHUTE_ARMED=true`, the label `parachute.armed=true` on Parachute's own container, or the `convenient` profile (CFG-011). Arming is never remembered and never read from the state file, so a copied or restored state file cannot arm an instance (CFG-006).
2. **Data stores are protected by default.** Containers recognised as databases (by image name, with a label to override) are never updated automatically; they are reported as "update available, approval required".
3. **Unpinned means ask.** Containers using `:latest` or no tag are updated; containers pinned to a version tag are never moved to a different tag unless the operator opts in.
4. **No update without a restore point.** If a snapshot cannot be taken, the update does not run (BKP-001), unless the operator sets `parachute.backup=none` on that container.
5. **Never delete what we did not create.** Parachute removes only containers it is replacing and snapshots it made (UPD-017).
6. **Quiet by default.** One notification digest per run, not one per container (NTF-002).

These defaults can be relaxed per container or globally, and each relaxation is explicit and logged at startup (POL-009).

## Consequences
- First-time users see "nothing happened" until they arm it. This is intentional and the README must say so.
- Operators used to tools that update everything at once will see fewer and later updates; the migration guide must list every default and how to relax it (see `open-questions.md` Q1).

## Specs affected
CFG-006, CFG-011, POL-*, BKP-001, NTF-*, UPD-017.
