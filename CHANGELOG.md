# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/) (while the major version is 0, minor
releases may include behaviour changes — see each release's *Changed* and
*Upgrade notes*).

Longer, narrative release notes live in [docs/releases/](docs/releases/).

## [Unreleased]

### Changed

- **The first run asks for the new master password twice** and creates no vault when the two do not match. A typo used to lock the new vault for good.

### Fixed

- **`~/.my.cnf`, the vault and `export` files always end up with mode `0600`.** They used to be written through a fixed `<file>.tmp` name with `os.WriteFile`, which keeps the mode of an existing file, so a leftover temporary file with wider permissions could leave passwords readable by other users. Writes now go through a randomly named temporary file whose mode is set explicitly, and are synced to disk before the rename.
- **Line breaks in values are escaped.** A newline in a password (or another quoted value) used to end the line in `~/.my.cnf`, turning the rest of the value into an option line of its own. It is now written as `\n` inside the quotes, which MySQL and MariaDB read back as the original character.
- **The socket path is quoted** when it contains spaces or other special characters.
- **A damaged vault reports an error instead of crashing.** A nonce length other than 12 in the file made `mycnfm` panic.

### Security

- Updated `golang.org/x/crypto` (0.24 → 0.59), `x/sys`, `x/term`, `x/text` and `x/sync`. mycnfm only uses Argon2 from `x/crypto`; the update clears scanner reports for other packages of the module.

## [0.2.0] - 2026-09-20

Automation and hygiene: the master password is asked once per session,
profiles can be applied from scripts, `~/.my.cnf` cleans itself up, and
options can target `[mysqldump]` (and other sections).

### Added

- **Auto-wipe timer.** Every apply arms a timer (default **1 hour**) after which `~/.my.cnf` is erased by a detached, password-less watcher process — but only if the file still contains exactly what was written. `-t <duration>` overrides it (`mycnfm -t 30m` for a TUI session, `mycnfm apply <path> -t 15m` for one apply); `-t 0` / `-t off` disables it. The header shows a live `wipes in 47m` countdown.
- **Launch-time backstop for the timer.** The deadline is stored in the vault; if the watcher did not survive (reboot, logout), the next `mycnfm` run wipes an expired, untouched file.
- **Session unlock cache.** After the first successful unlock in a login session, the derived key is held in the Linux kernel session keyring, so later runs do not ask for the password. `mycnfm lock` forgets it; logging out forgets it too. `passwd` never uses the cache.
- **Non-interactive unlock** via `MYCNFM_PASSWORD_FILE` (a file read once) or `MYCNFM_PASSWORD`, for cron/CI.
- **`mycnfm apply <path> [-o <path>|-]`** — apply one profile without the UI, by full path (`stage/auth-01`) or by unique name (ambiguous names list their full paths). `-o -` prints the rendered file to stdout and touches neither the vault nor `~/.my.cnf`.
- **`mycnfm list`** — every profile's full path, one per line, sorted.
- **Drift detection.** The vault remembers a hash of what was written; if `~/.my.cnf` is later edited, deleted or wiped, the header, the list row and the details panel show `⚠ … — file changed` / `— file missing` instead of a false `● active`.
- **Option sections.** Catalog options and per-host custom options have an optional *Section* (default `client`); options for another section (e.g. `mysqldump`) are written as their own `[section]` block after `[client]`.
- **Same option in several sections.** A catalog option is identified by key **and** section, so `default-character-set` can exist in `[client]` and in `[mysqldump]` with different values, enabled independently per host.
- **macOS cross-compilation targets:** `make build-darwin` (and `-arm64` / `-amd64`) produce `dist/mycnfm-darwin-*`.

### Changed

- **Applying a profile now schedules a wipe one hour later by default.** Use `-t off` to keep the previous "stays until cleaned" behaviour.
- `mycnfm clean` (and `x` in the TUI) also stops a pending auto-wipe timer.
- A `~/.my.cnf` that is already the wiped placeholder — by the timer or by `mycnfm clean` from the shell — clears the stale active profile instead of being reported as drift.
- Renaming a catalog option onto an existing one is now refused (it silently overwrote the other entry before). Changing an option's key or section moves every host's enabled state with it.
- Unlocking is centralised: TUI, `apply`, `list` and `export` share one code path (environment → session cache → prompt).

### Vault format

- Additive only: new optional fields `active_hash`, `active_expires` and per-option `section`. Vaults from 0.1.x open unchanged. Do not go back to 0.1.x with a vault that uses sections — the older version ignores and, on its next save, drops them.

## [0.1.1] - 2026-09-16

### Fixed

- The TUI header showed the version as `vv0.1.0` (a second `v` was prepended to a version that already started with one). `mycnfm --version` was unaffected.

## [0.1.0] - 2026-09-16

First release.

### Added

- **Encrypted vault** at `~/.config/mycnfm/vault.enc`: AES-256-GCM with an Argon2id-derived key (64 MiB, 2 passes, 4 threads); atomic writes, mode `0600`.
- **Interactive TUI** with a list panel, a details panel, a header (breadcrumb, active profile / transient status, version) and a footer of key hints; a `?` help overlay.
- **Environments and folders:** top-level folders are environments (`stage`, `prod`, …), nestable to any depth; create, rename, move and delete (with confirmation).
- **Profiles** with name, group, port, socket, user, password, database, tags and notes. There is intentionally no remote host field.
- **Groups** shown as section headers inside a folder, and a view toggle (`v`) between *by folder* and *by group*.
- **Tags** rendered as coloured chips (the colour is derived from the tag name).
- **Search** (`/`) across the whole current environment, and **sorting** (`s` cycles name → group/env → updated, `S` reverses).
- **Apply** (`enter`) writes `~/.my.cnf`: atomic, mode `0600`; passwords always double-quoted and escaped; port `3306` and blank fields omitted. The header always shows the active profile; transient events (saved, applied, …) replace it for four seconds.
- **Options catalog** (`O`): global key/value options defined once and shown as checkboxes on every host (seeded with `default-character-set = utf8mb3`, disabled until you enable it), plus per-host **custom options**.
- **Clean:** `x` in the TUI and `mycnfm clean` wipe `~/.my.cnf` without needing the master password.
- **Maintenance commands:** `mycnfm passwd` (change the master password in place), `mycnfm export` / `mycnfm import` (plaintext JSON round trip; both warn before overwriting).
- `ctrl+r` reveals or hides the selected profile's password in the details panel.
- Version compiled into the binary (`mycnfm --version`); `Makefile` with build, run, test, vet, fmt, tidy, install, uninstall, release and clean targets.
