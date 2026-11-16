# mycnfm

[![CI](https://github.com/master2sid2/mycnfm/actions/workflows/ci.yml/badge.svg)](https://github.com/master2sid2/mycnfm/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/master2sid2/mycnfm)](https://github.com/master2sid2/mycnfm/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/master2sid2/mycnfm)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**An encrypted credential switcher for `~/.my.cnf`.**

If you work with many MySQL/MariaDB servers that each have their own login,
you either edit `~/.my.cnf` by hand all day or keep old passwords lying around
in it as comments. `mycnfm` keeps every login in one encrypted vault, organised
the way you think about your infrastructure (environments, folders, groups,
tags). Pick a profile and it writes `~/.my.cnf` for you; when you are done —
or after an hour, by default — it wipes the file again so no password is left
on disk.

*Illustrative sketch of the interface (not a real screenshot):*

```
🗄  mycnfm  ›  stage                                  ● stage/auth-01 · wipes in 47m   v0.2.1
╭─ list ────────────────────────────╮ ╭─ details ─────────────────────────╮
│ ── AUTH ───────────────────────── │ │ auth-01  ● ACTIVE                 │
│ ▶  auth-01  ●                     │ │ Group     auth                    │
│    app@/run/mysqld/mysqld.sock    │ │ Path      stage                   │
│    [prod] [ro]                    │ │ User      app                     │
│ ── MAIL ───────────────────────── │ │ Password  ••••••••                │
│    mail-01                        │ │ Tags      prod  ro                │
╰───────────────────────────────────╯ ╰───────────────────────────────────╯
```

## Features

- **Encrypted vault** — AES-256-GCM, key derived with Argon2id. Nothing sensitive is stored in plaintext.
- **Organised like your infrastructure** — environments (`stage`, `prod`, …), nested folders, free-text *groups* shown as section headers, coloured *tags*, and a second view that pivots from *env → group* to *group → env*.
- **One key to switch** — `enter` on a profile writes `~/.my.cnf` (atomically, mode `0600`, passwords safely quoted and escaped).
- **Automatic clean-up** — `~/.my.cnf` is wiped an hour after you apply a profile (configurable with `-t`), by a small detached process that only wipes a file that still contains exactly what was written.
- **Notices when the file drifts** — if `~/.my.cnf` is edited, removed or wiped behind its back, the header says so instead of claiming the profile is still active.
- **Per-program option-file sections** — put `default-character-set` (or any option) in `[mysqldump]`, `[mysql]`, … instead of only `[client]`, even the same option in several sections with different values. See [docs/options.md](docs/options.md).
- **Enter the password once per login session** — the derived key is cached in the Linux kernel keyring, never on disk and never in the environment. `mycnfm lock` forgets it.
- **Scriptable** — `mycnfm list`, `mycnfm apply <profile> [-o -]`, non-interactive unlock for cron/CI.
- **Password maintenance** — `passwd` changes the master password in place; `export` / `import` round-trip a plaintext JSON copy for rotation or for handing a configuration to a colleague who re-encrypts it under their own password.

## Requirements

- Linux is the primary platform. macOS builds are produced by `make build-darwin` (see [Platform notes](#platform-notes)).
- To build from source: Go 1.26.1 or newer, and `make`.
- The MySQL/MariaDB client tools you already use (`mysql`, `mysqldump`, …) — they read `~/.my.cnf`.

## Install

```bash
go install github.com/master2sid2/mycnfm@latest
```

Or download a binary for Linux or macOS (amd64, arm64) from the
[releases](https://github.com/master2sid2/mycnfm/releases) page and check it
against `SHA256SUMS`. Or build from source:

```bash
git clone https://github.com/master2sid2/mycnfm.git
cd mycnfm
make build            # ./mycnfm, with the version baked in
make install          # copies it to ~/.local/bin (make sure that is on your PATH)
mycnfm --version
```

`make dist` builds the release binaries for Linux and macOS into `dist/`, and
`make build-darwin` just the macOS ones. All dependencies are pure Go, so no C
toolchain is needed.

## Quick start

```bash
mycnfm
```

1. The first run creates the vault at `~/.config/mycnfm/vault.enc` and asks for a **master password**, twice. You can change it later with `mycnfm passwd`.
2. Press `g`, type `stage`, `enter` — you have created an environment. `enter` again to open it.
3. Press `n` and fill in the host: a name, and whichever of user / password / socket / port / database you need. `ctrl+s` saves.
4. Press `enter` on the profile. `~/.my.cnf` now holds it, the header shows `● stage/auth-01 · wipes in 59m`, and `mysql` just works.
5. Press `x` to wipe `~/.my.cnf` right away, or do nothing — it is wiped after an hour.

Press `?` at any time for the key reference. Full guide: [docs/tui.md](docs/tui.md).

## What gets written

Credentials always go to `[client]`. Options that you assigned to another
section get their own block after it:

```ini
# Managed by mycnfm — DO NOT EDIT MANUALLY, changes will be overwritten.
# Active: stage/auth-01  ·  applied 2026-09-20 17:00:41

[client]
user     = app
password = "s3 cret#"
default-character-set = utf8mb3

[mysqldump]
default-character-set = utf8mb4
```

Blank fields are omitted, and so is port `3306` (it is MySQL's default, so
leaving it out changes nothing). There is deliberately **no `host`** — the same
login usually serves several servers, and you normally choose the server on the
command line (`mysql -h …`), which overrides the option file anyway.

## Command line

| Command | What it does |
|---|---|
| `mycnfm [-t <time>]` | Launch the interactive TUI. `-t` sets the auto-wipe delay for profiles applied in this session. |
| `mycnfm apply <path> [-t <time>] [-o <path>\|-]` | Apply one profile without the UI (by full path such as `stage/auth-01`, or by name if unique). `-o -` prints to stdout instead. |
| `mycnfm list` | Print every profile's full path, one per line. |
| `mycnfm clean` | Wipe `~/.my.cnf` (no master password needed) and stop the auto-wipe timer. |
| `mycnfm lock` | Forget the unlock cached for this login session. |
| `mycnfm passwd` | Change the master password in place. |
| `mycnfm export` / `import` | Decrypt to plaintext JSON / encrypt JSON back into a vault. |
| `mycnfm --version`, `--help` | |

Everything, with flags and examples: [docs/cli.md](docs/cli.md).

## Unlocking and sessions

The master password is asked **once per login session**. After a successful
unlock, `mycnfm` stores the *derived key* (never the password) in the Linux
kernel session keyring, so the next run — TUI, `apply`, `list`, `export` —
opens the vault silently. Logging out forgets it; so does `mycnfm lock`.
`passwd` always asks for the current password.

For unattended jobs there is no login session to cache in, so use
`MYCNFM_PASSWORD_FILE` (or `MYCNFM_PASSWORD`). How to set those **without
leaving the password in your shell history**, for bash, zsh, fish and POSIX
sh, is in [docs/automation.md](docs/automation.md).

## Auto-wipe timer

Every apply arms a timer, **1 hour by default**:

```bash
mycnfm -t 30m                       # every profile applied in this TUI session: 30 minutes
mycnfm apply stage/auth-01 -t 15m   # this one: 15 minutes
mycnfm apply stage/auth-01 -t off   # no timer (0 also disables it)
```

The timer is a detached copy of `mycnfm` that needs no password. At the
deadline it wipes `~/.my.cnf` **only if the file still holds exactly what was
written** — a profile you switched to or a file you edited by hand is never
touched. The deadline is also stored in the vault, so if the timer process did
not survive (reboot, logout) the next `mycnfm` run wipes an expired file.
Details: [docs/internals.md](docs/internals.md#auto-wipe-timer).

## Files

| Path | Purpose |
|---|---|
| `~/.config/mycnfm/vault.enc` | The encrypted vault (mode `0600`; the directory is `0700`). |
| `~/.config/mycnfm/watcher.pid` | PID of the running auto-wipe timer, if any. |
| `~/.my.cnf` | The file `mycnfm` writes and wipes (mode `0600`). |
| `./mycnfm-export.json` | Default output of `export` — **plaintext**, delete it after use. |

## Platform notes

- **Linux** — fully supported, including the session key cache.
- **macOS** — the binaries build and the TUI, `apply`, `list` and `clean` are portable, but the kernel keyring is Linux-only, so the password is asked on every run. The background timer uses `ps` to identify its own process there and has not been exercised on a Mac.
- **Windows** — not supported.

## Security

The vault protects your logins *at rest*. Once a profile is applied,
`~/.my.cnf` is plaintext until it is wiped, and anything running as your user
can read it. Please read [SECURITY.md](SECURITY.md) for exactly what is and is
not protected, and how to report a vulnerability.

## Documentation

- [docs/tui.md](docs/tui.md) — the interactive interface and every key
- [docs/cli.md](docs/cli.md) — command-line reference
- [docs/options.md](docs/options.md) — the options catalog and `[mysqldump]` sections
- [docs/automation.md](docs/automation.md) — non-interactive use, cron/CI, history-safe passwords
- [docs/internals.md](docs/internals.md) — vault format, crypto, keyring cache, timer
- [CHANGELOG.md](CHANGELOG.md) and [docs/releases/](docs/releases/) — what changed in each version
- [CONTRIBUTING.md](CONTRIBUTING.md) — development and the branching/release flow

## Versioning

Releases are tagged `vMAJOR.MINOR.PATCH`; the version is compiled into the
binary (`mycnfm --version`, shown in the TUI header). While the major version
is 0, minor releases may change behaviour — each one lists *Upgrade notes* in
its release notes.

## License

[MIT](LICENSE) © 2026 Vitalii Khutornyi
