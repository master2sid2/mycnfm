# Contributing

Thanks for helping. This document describes how the project is built, tested
and released.

## Ground rules

- **Everything is in English**: code, comments, identifiers, UI strings, docs, commit messages.
- Match the surrounding code: naming, comment density, idiom. Comments explain *why*, not *what*.
- A secret must never reach a log, an error message, a command line or a temporary file.
- Keep dependencies minimal; discuss before adding one.

## Getting started

Requires Go 1.26 or newer.

```bash
git clone https://github.com/master2sid2/mycnfm.git && cd mycnfm
git checkout dev
make build          # ./mycnfm with the version baked in
make test           # go test ./...
make vet            # go vet ./...
make fmt            # gofmt -w .
```

Try it without touching your real data:

```bash
HOME=$(mktemp -d) ./mycnfm
```

The vault and `~/.my.cnf` are derived from `$HOME`, so an empty temporary
`HOME` gives you a clean sandbox. On Linux, `keyctl session - bash` gives the
shell its own session keyring, so key-cache experiments do not affect your real
login session.

Other Makefile targets: `run`, `install`, `uninstall`, `tidy`, `clean`,
`mycnf-clean`, `release`, `build-darwin[-arm64|-amd64]`. Each rule has a
`## target: description` comment above it.

## Before you open a change

- `gofmt`-clean, `go vet ./...` clean, `go test ./...` green, and `golangci-lint run` clean (configured in `.golangci.yml`; CI runs all of these on every push and pull request).
- New behaviour comes with tests next to the code it covers (`*_test.go` in the same package). The TUI is tested through its model's `Update`; the vault, renderer and timer have plain unit tests.
- Anything that changes what is written to `~/.my.cnf` or to the vault format must keep old vaults opening (additive, optional, `omitzero` fields) or say loudly in the change why not.
- Update the docs that describe the behaviour, and add a line to the `Unreleased` section of [CHANGELOG.md](CHANGELOG.md).
- Try the change by hand in a real terminal. Unit tests do not tell you whether a screen looks right.

## Git workflow

| Branch | Purpose |
|---|---|
| `main` | Released code only. Every commit that matters is tagged `vX.Y.Z`. |
| `dev` | Integration branch; the day-to-day base. |
| `feature/<name>`, `fix/<name>`, `docs/<name>` | One change each, branched from `dev`. |
| `release/vX.Y.Z` | Cut from `dev` to prepare a release. |

1. Branch from `dev`: `git switch -c feature/thing dev`.
2. Commit in small steps.
3. When the change is reviewed and tested, merge it into `dev` with `git merge --no-ff` so the feature stays visible in history.
4. Never commit directly to `main`.

### Commit messages

`type: short imperative summary`, with `type` one of `feat`, `fix`, `refactor`,
`test`, `docs`, `build`, `chore`. Add a body explaining *why* when it is not
obvious.

```
feat: auto-wipe timer — ~/.my.cnf is erased after an hour by default
```

## Releasing

Only maintainers do this.

1. On a `docs/…` branch off `dev`: move `Unreleased` in the changelog under the new version and date, add `docs/releases/vX.Y.Z.md`, update the README and `docs/README.md` tables. Merge into `dev`.
2. `git switch -c release/vX.Y.Z dev`.
3. `make test vet`, then `make build && ./mycnfm --version` to check the output looks sane.
4. Merge `release/vX.Y.Z` into `main` with `--no-ff`.
5. Tag on `main`: `git tag -a vX.Y.Z -m "mycnfm vX.Y.Z"`.
6. Push `main`, `dev` and the tag to GitHub. The *Release* workflow tests the tag, builds Linux and macOS binaries with GoReleaser and publishes them with `docs/releases/vX.Y.Z.md` as the release notes — so that file must exist on the tag.
7. For the GitLab mirror, push the same refs and attach the output of `make dist` (built from the tag, so `git describe` yields exactly `vX.Y.Z`) to the release by hand.

### Versioning

[Semantic Versioning](https://semver.org/). While the major version is 0, a
minor bump may include behaviour changes; they are always listed under
*Changed* and *Upgrade notes*.

The version string is not stored in the source. `make build` derives it from
`git describe --tags --always --dirty` and injects it via
`-ldflags "-X github.com/master2sid2/mycnfm/internal/version.Version=…"`. A plain `go build` reports
`dev`.

## Project layout

See [docs/internals.md](docs/internals.md#source-layout).

## Reporting bugs and security issues

Bugs: open an issue with the version, platform, and steps to reproduce (never
paste a real vault, password, or `~/.my.cnf`).
Vulnerabilities: follow [SECURITY.md](SECURITY.md), not a public issue.

## License

By contributing you agree that your contribution is licensed under the
[MIT License](LICENSE).
