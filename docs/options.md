# Options

Beyond the credentials, a profile can carry extra `my.cnf` settings. There are
two kinds.

| | Catalog option | Custom option |
|---|---|---|
| Defined | once, in the catalog (`O`) | on one profile, in its form |
| Appears | as a checkbox on **every** profile | only on that profile |
| Enabled | per profile, by ticking it | by its own *Enabled* checkbox |
| Use for | things you want everywhere (`default-character-set`) | one-offs (`ssl-mode` on a single host) |

Changing a catalog option's value changes it for every profile that has it
ticked. Deleting it removes it from all of them.

## Anatomy of an option

- **Key** — the option name, e.g. `default-character-set`.
- **Value** — may be blank, which writes a bare flag (`quick`).
- **Section** — the `my.cnf` group it belongs to. Blank means `client`.

A catalog option is identified by **key + section**. The same key can therefore
exist in several sections with different values:

| Key | Section | Value |
|---|---|---|
| `default-character-set` | *(client)* | `utf8mb3` |
| `default-character-set` | `mysqldump` | `utf8mb4` |

Each row has its own checkbox on every profile. Within one section a key can
appear only once; creating or renaming an option onto an existing
key+section is refused. Editing an option's key or section carries every
profile's enabled state over to the new identity.

## Why sections matter

`mysqldump` does not reliably honour settings such as `default-character-set`
from `[client]` across MySQL and MariaDB versions, so a setting meant for dumps
belongs in `[mysqldump]`. Other tools read their own groups too (`[mysql]`,
`[mysqladmin]`, …); the section field is free text, so any of them works.

## What gets written

Credentials and every enabled option whose section is `client` go into
`[client]`. Every other section becomes its own block, after `[client]`, in
alphabetical order. Options keep the order they have in the catalog, then
custom ones follow.

```ini
[client]
socket   = /var/run/mysqld/mysqld.sock
user     = app
password = "s3 cret#"
default-character-set = utf8mb3

[mysqldump]
default-character-set = utf8mb4
quick
```

A section with no enabled options is not written at all. Disabled options are
never written.

## Seeded example

A new vault's catalog contains `default-character-set = utf8mb3` (client
section), disabled everywhere until you tick it. Edit or delete it freely.

## Tips

- Put things that differ per host in *custom* options, and things you want to be able to toggle everywhere in the catalog.
- To have a value applied to dumps only, use a `mysqldump` section option and leave the `client` one alone.
- `mycnfm apply <path> -o -` prints exactly what will be written, which is a fast way to check a section layout.
