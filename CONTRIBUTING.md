# Contributing

Parachute is spec-driven ([ADR-0001](docs/adr/0001-spec-driven-development.md)). Every change follows the same order:

1. Change the spec first. New behaviour gets a requirement in `docs/specs/` with a new ID; a wrong or ambiguous spec is fixed in its own commit.
2. Write the black-box test from the acceptance criteria in `e2e/`, named after the ID (`TestSEL_001_...`). Tests never import application packages ([ADR-0003](docs/adr/0003-black-box-tests-from-specs.md)).
3. Write the smallest code that makes the test pass. Go code has no comments; names carry the meaning. Collaborators are small interfaces injected through constructors, wired only in `cmd/parachute` and `internal/app`.
4. List the requirement under its milestone in `docs/specs/README.md` once it is implemented.
5. Run `make lint test trace e2e` before opening a pull request. `make e2e` needs a local Docker engine.
