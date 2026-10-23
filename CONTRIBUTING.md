# Contributing

Parachute is spec-driven ([ADR-0001](docs/adr/0001-spec-driven-development.md)). Every change follows the same order:

1. Change the spec first. New behaviour gets a requirement in `docs/specs/` with a new ID; a wrong or ambiguous spec is fixed in its own commit.
2. Write the black-box test from the acceptance criteria in `e2e/`, named after the ID (`TestSEL_001_...`). Tests never import application packages ([ADR-0003](docs/adr/0003-black-box-tests-from-specs.md)).
3. Write the smallest code that makes the test pass. Go code has no comments; names carry the meaning. Collaborators are small interfaces injected through constructors, wired only in `cmd/parachute` and `internal/app`.
4. List the requirement under its milestone in `docs/specs/README.md` once it is implemented.
5. Run `make lint test trace e2e` before opening a pull request. `make e2e` needs a local Docker engine.

## Commit standard

Commits follow [Conventional Commits](https://www.conventionalcommits.org/): `type(scope): subject`, where the scope is optional for repo-wide changes (`docs: fix links`), with the subject in lower case and the imperative mood, no trailing period. Identifiers keep their case, so requirement, ADR, and question IDs may appear in subjects (`docs(spec): add SEL-013`, `docs(adr): accept ADR-0012 (Q19)`).

- **Types:** `feat`, `fix`, `docs`, `test`, `refactor`, `chore`, `ci`, `build`, `perf`.
- **Scopes:** `spec`, `adr`, `config`, `selection`, `schedule`, `detect`, `registry`, `engine`, `update`, `backup`, `verify`, `policy`, `hook`, `notify`, `api`, `daemon`, `status`, `restore`, `e2e`, `ci`, `deps`. Spec and ADR edits use `docs(spec): ...` and `docs(adr): ...`; a correction of a wrong requirement uses `fix(spec): ...`.
- **Breaking changes:** add `!` before the colon (`feat(config)!: remove --foo`) and a `BREAKING CHANGE:` footer saying what operators must change.
- **Pull requests:** `main` is squash-merge only, so the PR title becomes the commit and follows the same format.
- No attribution lines (`Co-Authored-By`, "Generated with", and similar) in commits or PR bodies.
