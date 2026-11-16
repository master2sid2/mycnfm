# Security policy

## Supported versions

Only the latest release receives fixes.

| Version | Supported |
|---|---|
| 0.2.x | yes |
| 0.1.x | no — upgrade (vaults open unchanged; see the [0.2.0 notes](docs/releases/v0.2.0.md#upgrade-notes)) |

## Reporting a vulnerability

Please **do not open a public issue** for a security problem.

<!-- TODO(maintainer): add a private contact (security e-mail address or the
     repository's private vulnerability reporting link) before publishing. -->

Report it privately to the maintainer, with the version (`mycnfm --version`),
your platform, and steps to reproduce. You should get an acknowledgement within
a few days, and a fix or a decision within a reasonable time after that. Please
allow time for a release before disclosing.

## What mycnfm protects

- **The vault at rest.** Credentials are stored in `~/.config/mycnfm/vault.enc`, encrypted with AES-256-GCM under a key derived from your master password with Argon2id (64 MiB, 2 passes, 4 threads). Without the password, or the cached key, the file is unreadable and tampering is detected.
- **Leftovers in `~/.my.cnf`.** The file is written `0600`, is wiped by the auto-wipe timer (default 1 h), and is only ever wiped when it still contains what mycnfm wrote.
- **Accidents.** Passwords are always quoted and escaped so they cannot break out of the option file; the timer's process carries no secret; `mycnfm clean` needs no password.

## What it does not protect against

Be explicit about these when deciding whether the tool suits you.

- **Root, or malware running as you.** Anyone who can read your process memory, your keyring, or your files can get what you can. The tool is not a defence on a compromised account.
- **The plaintext `~/.my.cnf` while a profile is applied.** MySQL clients need it readable. The timer shortens the window; it does not remove it.
- **The cached key.** While an unlock is cached (Linux session keyring), any process in your login session can open the vault without the password. The cache holds the derived key, not the password, but that is equivalent for the vault. Use `mycnfm lock`, or log out, to drop it. It never leaves kernel memory.
- **Environment variables.** `MYCNFM_PASSWORD` is visible to your other processes (`/proc/<pid>/environ`) and to children of the shell it is exported in. Prefer `MYCNFM_PASSWORD_FILE` with mode `0600`, and set variables without leaving them in shell history — see [automation](docs/automation.md).
- **Revealed passwords.** `ctrl+r` shows a password on screen; it can appear in scrollback, screenshots or screen sharing.
- **Exports.** `mycnfm export` writes **every password in plaintext JSON**. Delete the file when finished; do not commit or send it.
- **Memory.** Secrets are not zeroed after use and may be swapped to disk.
- **Weak master passwords.** None is enforced. Argon2id slows guessing; it cannot rescue a weak password. Use a long one.
- **The timer's visibility.** The background process's command line shows the path of `~/.my.cnf`, a hash of its content and the deadline — no secrets, but not invisible.
- **Concurrent use.** There is no locking between two running instances; the last save wins.
- **Loss of the master password.** It cannot be recovered. Keep an export somewhere safe if that matters to you.

## Good practice

1. Choose a strong, unique master password.
2. Keep `~/.config/mycnfm/` at `0700`, and back up `vault.enc`.
3. Run `mycnfm lock` when leaving a shared machine.
4. Leave the auto-wipe timer on (the default) unless you have a reason not to.
5. Never put the master password on a command line.
