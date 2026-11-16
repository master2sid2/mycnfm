# How it works

## Source layout

```
main.go                 entry point; wires SIGINT/SIGTERM to a context
cmd/                    subcommands: root, apply, unlock, passwd, export_import
internal/config/        data model, vault file format, encryption
internal/mycnf/         rendering ~/.my.cnf, hashing, drift reconciliation
internal/tui/           the bubbletea interface
internal/keyring/       Linux session-keyring key cache (stub elsewhere)
internal/watch/         the auto-wipe timer
internal/fsutil/        atomic, private (0600) file writes
internal/version/       version string, set with -ldflags
```

## The vault

`~/.config/mycnfm/vault.enc` (directory `0700`, file `0600`).

```
"MCNF" | version (u32, =1) | salt length (u32) | salt (32 bytes)
       | nonce length (u32) | nonce (12 bytes) | AES-256-GCM ciphertext+tag
```

All integers are big-endian.

- The key is derived from the master password with **Argon2id** (time 2, memory 64 MiB, 4 threads, 32-byte output) and the vault's own random salt.
- The payload is the JSON-encoded store, sealed with **AES-256-GCM**. GCM authenticates the ciphertext, so any modification of it fails to open. The salt and header fields are not authenticated, but changing them yields a different key or a parse error, and never a successful open.
- Saves are atomic: the new vault is written to a randomly named temporary file in the same directory, with its mode set to `0600` explicitly and synced to disk, then renamed over the old one, so a crash cannot leave a half-written vault and the file never inherits wider permissions. `~/.my.cnf` and `export` files are written the same way (`internal/fsutil`).
- A new nonce is generated on **every** save.

### Store contents

- **Folders** — a tree. Top-level folders are the environments.
- **Records** — name, group, port, socket, user, password, database, tags, notes, `GlobalOptions` (a map from option ID to enabled), `CustomOptions`, timestamps. There is deliberately no host field.
- **Options catalog** — `{key, value, section}`; the ID is the bare key for the `client` section and `[section]key` otherwise.
- **Active state** — `ActiveID` (which profile is applied), `ActiveHash` (sha-256 of what was written), `ActiveExpires` (the wipe deadline, omitted when none).

Unknown fields are ignored when reading, so an older build opens a newer
vault — but it drops the fields it does not know on its next save.

## Rendering `~/.my.cnf`

`mycnf.Render` produces the file; `mycnf.Apply` writes it atomically with mode
`0600`.

- Two comment lines head the file: a "managed by mycnfm" notice and `# Active: <label> · applied <time>`.
- `[client]` gets `port`, `socket`, `user`, `password`, `database` in that order (blank ones omitted, port `3306` omitted) and every enabled option with the `client` section.
- Other sections follow as sorted `[section]` blocks.
- The password is always written in double quotes with `\` and `"` escaped, so `#`, `;`, spaces and quotes survive MySQL's option-file parser.

## Drift detection

When a profile is applied, the sha-256 of the exact bytes written is stored.
`mycnf.Reconcile(store, path, now)` compares it with the file on disk and
returns one of:

| Result | Meaning | Effect |
|---|---|---|
| Unchanged | the file matches, or nothing is active | none |
| Expired | the stored deadline has passed and the file still matches | the file is wiped and the active profile cleared |
| FoundCleaned | the file is the wiped placeholder | the active profile is cleared silently |
| *(otherwise)* | edited, replaced or deleted | shown as `⚠ file changed` / `file missing`; nothing is modified |

It runs on every unlock and once a second in the TUI.

## Unlock and the session cache

`cmd/unlock.go` implements one path for every command:

1. `MYCNFM_PASSWORD_FILE` / `MYCNFM_PASSWORD` — derive the key and open.
2. Session cache — look up the vault's cached key and open with `config.OpenWithKey`. A stale or wrong key is discarded and the flow falls through to the prompt.
3. Prompt, derive, open — and only **after a successful open**, cache the key. A wrong password is never cached.

### The keyring

On Linux the derived 32-byte key is stored as a `user` key in the **session
keyring** through the `keyctl` syscalls (`internal/keyring`), with
possessor-only permissions. Consequences:

- It lives in kernel memory, never on disk, never in the environment.
- It is shared by every process in your login session and nobody else's.
- It disappears when the session ends, or on `mycnfm lock`.
- The cached thing is the *key*, not the password: anyone who can read it can open the vault, but cannot learn the password from it.
- The key is bound to the vault's salt, so `passwd` and `import` (which change the salt) invalidate it.
- On other platforms `internal/keyring` is a stub: nothing is cached and every run prompts.

It relies on the session having its own session keyring (`pam_keyinit`, standard
on systemd distributions). Where it does not, the kernel falls back to a
per-user keyring shared by all your sessions. `mycnfm lock` clears either.

## Auto-wipe timer

`internal/watch`, started by every apply that targets the real `~/.my.cnf`.

- **Launch.** `mycnfm` re-executes itself as `mycnfm __wipe-after <path> <hash> <deadline-unix>` under `setsid`, with standard streams closed, so it survives the terminal closing. Its command line holds the path, a hash and a deadline — no secret.
- **Run loop.** It wakes at most every 30 seconds and compares the *wall clock* with the deadline. Wall-clock polling, rather than one long sleep, means suspend/resume and clock changes behave sensibly: after the laptop wakes the file is wiped at the next poll.
- **Wipe condition.** At the deadline it re-hashes `~/.my.cnf`. If it still equals the recorded hash, the file is replaced by the placeholder; if you edited it, or applied another profile, it is left untouched. Either way the watcher exits.
- **One watcher.** Its pid goes in `~/.config/mycnfm/watcher.pid`. A new apply, `clean`, or the TUI's `x` first stops the previous watcher. A stale pid is recognised by checking `/proc/<pid>/cmdline` (or `ps` on macOS) for `__wipe-after`, so a recycled pid is never signalled.
- **Signals.** SIGTERM cancels the run context, so the watcher exits promptly when stopped.
- **Backstop.** The deadline is also stored in the vault (`ActiveExpires`). If the watcher did not survive, the next unlock runs `Reconcile`, which wipes the expired file when it is still exactly what was written.

### Limits

- If your system kills user processes on logout and you never run `mycnfm` again, nothing wipes the file.
- Someone with the same privileges can kill the watcher.
- The timer protects against *forgetting*; it is not a barrier against an attacker who is already on your account.
- Only the real `~/.my.cnf` is timed; files written with `-o <path>` are not.

## Building and versioning

The version comes from `git describe --tags --always --dirty` at build time and
is injected with `-ldflags "-X github.com/master2sid2/mycnfm/internal/version.Version=…"`. A build
outside a git checkout reports `dev`. See [CONTRIBUTING](../CONTRIBUTING.md).
