# Automation and scripting

## How `mycnfm` gets the master password

In order:

1. `MYCNFM_PASSWORD_FILE` — path to a file holding the password (one trailing newline is ignored).
2. `MYCNFM_PASSWORD` — the password itself.
3. The login-session key cache (Linux): after one interactive unlock, later runs need nothing.
4. An interactive prompt.

If an environment variable is set and wrong, the command fails; it does not fall
back to a prompt.

For your own terminal the session cache is usually all you need: type the
password once, and every later `mycnfm`, `mycnfm apply …` or `mycnfm list`
works until you log out or run `mycnfm lock`. The rest of this page is for
places with no login session: cron, CI, containers, and remote commands.

## Prefer a file over a variable

```bash
umask 077
printf '%s\n' 'the master password' > ~/.config/mycnfm/pw    # or write it from your secret store
chmod 600 ~/.config/mycnfm/pw
MYCNFM_PASSWORD_FILE=~/.config/mycnfm/pw mycnfm apply stage/api/db-01
```

A file's contents are not exposed through `/proc/<pid>/environ` and are not
inherited by child processes. Anyone who can read the file can open the vault,
so treat it exactly like a private key.

Process substitution avoids a file on disk entirely:

```bash
MYCNFM_PASSWORD_FILE=<(pass show mycnfm) mycnfm apply stage/api/db-01
```

## Setting `MYCNFM_PASSWORD` without leaving it in shell history

Typing `export MYCNFM_PASSWORD=secret` stores `secret` in your history file.
Read it with a silent prompt instead, so it is never part of a command line.

**bash** *(tested)*

```bash
read -rsp 'Master password: ' MYCNFM_PASSWORD && export MYCNFM_PASSWORD; echo
```

**POSIX sh / dash** *(tested)*

```sh
stty -echo; printf 'Master password: '; read MYCNFM_PASSWORD; stty echo; echo
export MYCNFM_PASSWORD
```

**zsh** *(from the documented syntax; not tested)*

```zsh
read -rs "MYCNFM_PASSWORD?Master password: " && export MYCNFM_PASSWORD; echo
```

**fish** *(from the documented syntax; not tested)*

```fish
read --silent --export --prompt-str "Master password: " MYCNFM_PASSWORD
```

**PowerShell 7** *(from the documented syntax; not tested — and mycnfm itself is not supported on Windows, this is for pwsh on Linux/macOS)*

```powershell
$s = Read-Host -AsSecureString 'Master password'
$env:MYCNFM_PASSWORD = [System.Net.NetworkCredential]::new('', $s).Password
```

When you are done: `unset MYCNFM_PASSWORD` (fish: `set -e MYCNFM_PASSWORD`).

### Weaker options

- **Leading space.** With `HISTCONTROL=ignorespace` (bash) or `setopt HIST_IGNORE_SPACE` (zsh), a command that starts with a space is not saved. It is easy to forget the space, and the secret is still in the process's command line while it runs.
- **Scope it to one command:** `MYCNFM_PASSWORD="$(…)" mycnfm apply …` keeps it out of your shell's environment afterwards, but the command line itself may still be recorded.

## Cron and CI

```cron
# Refresh a profile every morning; the wiper is not needed for a job that
# manages its own file, so give it an explicit path and no timer.
0 7 * * 1-5  MYCNFM_PASSWORD_FILE=/home/ci/.mycnfm-pw /usr/local/bin/mycnfm apply stage/api/db-01 -o /home/ci/.my-stage.cnf
```

Notes:

- With `-o <path>` nothing is recorded in the vault and **no auto-wipe timer is armed**; you decide when the file goes away.
- Without `-o`, the real `~/.my.cnf` is written and the default one-hour wipe is armed. Under cron, where the session may end right away, pass `-t` explicitly or `-t off` and clean up yourself. See [internals](internals.md#auto-wipe-timer).
- There is no login session in cron, so no key cache; the environment variable or file is what unlocks.

## Scripting recipes

Print the rendered file to standard output; nothing is written and the vault
is not modified:

```bash
mycnfm apply stage/api/db-01 -o -
```

Iterate over profiles (`list` prints only paths, one per line):

```bash
mycnfm list | grep '^stage/' | while read -r p; do
    mycnfm apply "$p" -o "/tmp/cnf/$(basename "$p").cnf"
done
```

Give a client a one-off config without touching `~/.my.cnf` *(not tested with real MySQL tools)*:

```bash
mysqldump --defaults-extra-file=<(mycnfm apply stage/api/db-01 -o -) mydb > dump.sql
```

`--defaults-extra-file` must be the **first** option on the command line for
MySQL client programs. The process substitution keeps the password off disk,
although it is visible in the temporary `/dev/fd` path to your own processes.

Write a profile to a private file and remove it afterwards:

```bash
mycnfm apply stage/api/db-01 -o ~/.my-stage.cnf
mysql --defaults-file=~/.my-stage.cnf …
rm -f ~/.my-stage.cnf
```

## Exit codes

`0` on success, `1` on any error, with the message on standard error. `apply`'s
status lines are also on standard error, so standard output carries only the
rendered file for `-o -`, and only paths for `list`.

## Forgetting things

```bash
mycnfm lock        # drop the cached unlock
mycnfm clean       # wipe ~/.my.cnf (needs no password)
unset MYCNFM_PASSWORD
```
