# Command-line reference

```
mycnfm [-t <time>]                        launch the interactive TUI
mycnfm apply <path> [flags]               apply one profile without the UI
mycnfm list [flags]                       print every profile's path
mycnfm clean                              wipe ~/.my.cnf
mycnfm lock [flags]                       forget the cached unlock
mycnfm passwd [flags]                     change the master password
mycnfm export [flags]                     decrypt the vault to plaintext JSON
mycnfm import [flags]                     encrypt a JSON export into a vault
mycnfm --version | -v                     print the version
mycnfm --help | -h                        print usage
```

Errors are printed to standard error and the exit status is `1`; success is `0`.
Flags may be given before or after the profile path in `apply`.

## Common flags

| Flag | Meaning |
|---|---|
| `-vault <path>` | Vault file. Default `~/.config/mycnfm/vault.enc`. Accepted by `apply`, `list`, `lock`, `passwd`, `export`, `import`. |

## Unlocking

Every command that opens the vault (`mycnfm`, `apply`, `list`, `export`,
`passwd`) obtains the key in this order:

1. **`MYCNFM_PASSWORD`** or **`MYCNFM_PASSWORD_FILE`** if set (see [automation](automation.md)). If it is set but wrong, the command fails — it does not fall back to a prompt.
2. **The session cache** — the derived key kept in the Linux kernel session keyring by an earlier unlock. Not used by `passwd`.
3. **A prompt** for the master password. On success the derived key is cached for the rest of the login session (Linux) and one line is printed to say so.

`clean` and `--version` never open the vault.

## `mycnfm [-t <time>]`

Opens the TUI ([guide](tui.md)). `-t` is the [auto-wipe](#auto-wipe-timer--t)
delay for every profile applied during this session.

## `mycnfm apply <path> [-t <time>] [-o <path>|-] [-vault <path>]`

Resolves one profile and writes it — exactly what `enter` does in the TUI.

`<path>` is the profile's full path as `list` prints it (`stage/api/db-01`).
If nothing matches the full path, a bare **name** is accepted when it is unique
in the whole vault. If several profiles share the name, the command fails and
lists their full paths; use one of those.

| `-o` | Effect |
|---|---|
| *(omitted)* | Write `~/.my.cnf`; record it as the active profile (so the TUI header and drift detection know); arm the auto-wipe timer. |
| `-o <path>` | Write that file instead. Nothing is recorded in the vault and **no timer is armed** — you own that file. |
| `-o -` | Print the rendered file to standard output. Touches neither the vault nor any file. |

Status lines (`Applied … (auto-wipe in 59m59s)`) go to standard error, so
`-o -` output is safe to pipe.

```bash
mycnfm apply stage/api/db-01
mycnfm apply db-01 -t 15m
mycnfm apply stage/api/db-01 -o -        # print, change nothing
```

### Auto-wipe timer — `-t`

`-t <duration>` is how long the profile stays in `~/.my.cnf`:

| Value | Meaning |
|---|---|
| *(omitted)* | 1 hour. |
| `30m`, `1h30m`, `90s`, … | Any Go duration. |
| `0` or `off` | No timer. |

Negative or unparsable values are rejected. Only the real `~/.my.cnf` is
timed; `-o <path>` and `-o -` are not. How it works and its limits:
[internals](internals.md#auto-wipe-timer).

## `mycnfm list [-vault <path>]`

Prints every profile's full path, one per line, sorted, plain text — nothing
else on standard output, so it is safe to script against.

## `mycnfm clean`

Overwrites `~/.my.cnf` with a credential-free placeholder and stops any pending
auto-wipe timer. It does **not** open the vault, so it needs no password and
works even when you cannot unlock. The next time the TUI (or any command that
opens the vault) runs, it notices the wiped file and clears the active profile.

## `mycnfm lock [-vault <path>]`

Forgets the unlock cached for the vault in this login session, so the next
command asks for the password. Nothing cached is not an error. On platforms
without a session keyring it says there is nothing to forget.

## `mycnfm passwd [-vault <path>]`

Asks for the **current** password (always — never satisfied from the session
cache), then the new one twice, and re-encrypts the vault in place under a
fresh salt. Plaintext never touches disk. The cached key for the old password
is dropped.

## `mycnfm export [-vault <path>] [-json <path>]`

Decrypts the vault and writes it as indented plaintext JSON.

- Default output: `./mycnfm-export.json` in the **current directory** (not next to the binary).
- If the output file exists you are asked before it is overwritten.
- The file is written with mode `0600` and **contains every password in clear text**. Delete it when you are done.

## `mycnfm import [-vault <path>] [-json <path>]`

Reads a JSON export and encrypts it into a vault under a **new master password**
you type twice.

- Default input: `./mycnfm-export.json`; default vault: `~/.config/mycnfm/vault.enc`.
- If the vault already exists you are asked before it is overwritten.
- The new password is always entered interactively, even if `MYCNFM_PASSWORD*` is set.
- The imported vault has a fresh salt, so any key cached for the old one simply fails and is replaced by a prompt.

Together with `export` this is the way to share a configuration: you export,
your colleague imports it and chooses their own master password. Your
password is never involved.

## Environment variables

| Variable | Meaning |
|---|---|
| `MYCNFM_PASSWORD` | The master password itself. Convenient, but visible to your other processes through `/proc/<pid>/environ`. |
| `MYCNFM_PASSWORD_FILE` | Path to a file containing the password (a trailing newline is ignored); read once. Prefer this. |

Both are read only when *unlocking* an existing vault; the "new password"
prompts of `import` and `passwd` are always interactive.

## Files

See the table in the [README](../README.md#files).

## The hidden `__wipe-after` command

`mycnfm __wipe-after <path> <hash> <deadline-unix>` is the auto-wipe timer's
own process. It is started for you and is not meant to be run by hand.
