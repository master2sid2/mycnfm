# The interactive interface

Run `mycnfm` (or `mycnfm -t 30m`, see [auto-wipe](cli.md#auto-wipe-timer--t)).
Press `?` at any time for a key reference.

## Layout

```
🗄  mycnfm  ›  stage  ›  api         ● stage/api/db-01 · wipes in 47m   v0.2.0    <- header
╭─ list ───────────────────────────╮ ╭─ details ─────────────────────────╮
│ …                                │ │ …                                 │
╰──────────────────────────────────╯ ╰───────────────────────────────────╯
  ↑↓ navigate  enter apply/open  n new host  …                                    <- footer
```

**Header**

- Left: the app name and a breadcrumb of the folder you are in (`stage › api`). In the group view it reads `grouped by category`.
- Right, from left to right, one *slot* and then the version:
  - **the slot shows the last event** (saved, applied, deleted, moved, …) for four seconds, then reverts to the persistent state below;
  - `● stage/api/db-01 · wipes in 47m` — the active profile and the time left before the auto-wipe timer erases `~/.my.cnf` (the countdown is omitted when the timer is off);
  - `⚠ stage/api/db-01 — file changed` or `— file missing` — the vault says this profile is active, but `~/.my.cnf` no longer matches what was written (edited, deleted, replaced);
  - `○ no active profile`.
  - Errors show as `✗ …` in the same slot.

**List (left).** Folders (amber `▸`), profiles, and non-selectable *group headers* (`── AUTH ──` with a count). A profile row shows `user@socket` (plus `:port` when not 3306, and `/database`), then its tags as coloured chips. `●` marks the active profile (`⚠` when it has drifted); its name is highlighted. In flattened views each row also shows the folder path it lives in.

**Details (right).** For a folder: type (ENV or subfolder) and counts. For a profile: name and `● ACTIVE` badge, group, path, socket, port, user, password (masked — `ctrl+r` reveals), database, every enabled option (with `[section]` when it is not `client`), tags and notes.

**Footer.** Context-sensitive key hints, including the current sort (`sort:name↑`) and view (`by folder` / `by group`).

## Concepts

- **ENV** — a top-level folder (`stage`, `prod`, …). A folder created at the root is an ENV; folders inside are subfolders. There is no depth limit.
- **Profile** — one set of credentials (name, group, port, socket, user, password, database, tags, notes, options). Only the name is required; leave the rest blank to omit them from `~/.my.cnf`.
- **Group** — a free-text label on a profile (`auth`, `mail`, …). Inside a folder, profiles with the same group are listed under a header; profiles without one come first, unheaded.
- **Tag** — a free-text label shown as a coloured chip. The colour is derived from the name, so the same tag always looks the same everywhere.

## Keys — list view

| Key | Action |
|---|---|
| `↑` `↓` (`pgup` `pgdown` `home` `end`) | Move. Group headers are skipped. |
| `enter` | On a folder: open it. On a profile: **apply** it (write `~/.my.cnf`). You stay in the TUI. |
| `esc` | Go up a folder; leave the group view; (in search) clear the search. |
| `n` | New profile in the current folder. |
| `g` | New folder in the current folder (at the root: a new ENV). |
| `e` | Edit the selected profile / rename the selected folder. |
| `m` | Move the selected profile or folder (a folder cannot be moved into itself or its own descendants). |
| `d` | Delete the selected profile or folder — asks `y/n`; deleting a folder deletes everything inside it. |
| `v` | Toggle **by folder** ⇄ **by group**. In the group view the current folder's whole subtree is flattened and clustered by group, each profile showing its folder path. |
| `s` / `S` | Cycle the sort: **name** → **group** (or **env** in flattened views) → **updated**; `S` reverses. |
| `/` | Search (see below). |
| `O` | Open the [options catalog](options.md). |
| `x` | Wipe `~/.my.cnf` now (asks `y/n`); also stops the auto-wipe timer. |
| `ctrl+r` | Reveal / hide the selected profile's password in the details panel. |
| `?` | Key reference. |
| `q` | Quit. `ctrl+c` quits from anywhere. |

### Search

`/` opens a search box. Matching is a case-insensitive substring over the
profile's **name, socket, user, database, group, tags and folder path**, and it
covers the **whole environment you are in** (recursively) — or the entire vault
when you are at the root. Results are flat, each with its path. While
searching, `↑` `↓` still move, `enter` applies the selected result, and `esc`
clears the search. Every other key types into the box (so single-letter
shortcuts are off until you leave it).

## Keys — profile form (`n` / `e`)

Fields, in order: **Name**, Group, Port, Socket, User, Password, Database,
**Options** (checkboxes from the catalog — hidden while the catalog is empty),
**Custom options (this host only)**, Tags, Notes.

| Key | Action |
|---|---|
| `tab` / `↓`, `shift+tab` / `↑` | Next / previous field (inside the two option lists, next / previous row first). |
| `space` | On an option row: toggle it on or off. |
| `enter` | On a custom-option row: edit it; on `[ + Add option ]`: add one; elsewhere: next field. |
| `d` / `delete` | On a custom-option row: remove it. |
| `ctrl+s` | Save. |
| `esc` | Cancel. |

Validation: the name is required; the port must be 1–65535 or blank (blank and
3306 both mean "MySQL's default" and are omitted from the file). The **Tags**
field takes a comma-separated list and shows the tags already in use.

### Option editor

Opened from *Custom options*. Fields: **Key**, **Value** (blank writes a bare
flag such as `quick`), **Section** (blank = `client`; e.g. `mysqldump`), and an
**Enabled** checkbox. `tab` moves between fields, `space` / `enter` toggles the
checkbox, `ctrl+s` saves, `esc` cancels.

## Keys — options catalog (`O`)

| Key | Action |
|---|---|
| `↑` `↓` | Move. |
| `n`, or `enter` on `[ + New option ]` | New option (Key, Value, Section). |
| `enter` / `e` | Edit the selected option. |
| `d` | Delete it — asks `y/n`; it is removed from every profile. |
| `esc` | Close. |

The catalog only *defines* options. Whether one is used is decided per profile,
by its checkbox in the profile form. See [options.md](options.md).

## Keys — move picker (`m`)

| Key | Action |
|---|---|
| `↑` `↓` (`j` `k`) | Move. |
| `enter` | On a folder: go into it. On `[ ✓ Move here ]`: move the item to the folder you are in. |
| `esc` | Back up a level; at the top, cancel. |

## Keys — folder form (`g` / `e` on a folder)

`enter` or `ctrl+s` saves, `esc` cancels. The name is required.

## Confirmations

Delete and clean ask for `y` (yes) or `n` / `esc` (no). Nothing else asks
before acting — in particular, `enter` on a profile replaces whatever
`~/.my.cnf` contained.

## Things that happen on their own

- **Every second** the header re-checks `~/.my.cnf`: it updates the countdown, notices drift, and — if the auto-wipe timer's watcher already wiped the file — drops the active profile and says so.
- **On every unlock**, an active profile whose deadline passed while nothing was running is wiped (see [internals](internals.md#auto-wipe-timer)).
