# Update Guide

This guide covers normal Memora upgrades and the migration from the original
ChronoFrame project ([HoshinoSuzumi/chronoframe](https://github.com/HoshinoSuzumi/chronoframe)).
Memora is a hard fork with a Go server and a generated static frontend. The
database and Docker data mount are intentionally kept compatible, but the
runtime migration mechanism is different.

## Before Every Upgrade

1. Stop the old process or container. Never open the same SQLite database from
   two application processes.
2. Back up the complete `data/` directory, `.env`, and your Compose or Docker
   command. If the photos are in OpenList, S3, or another service, verify and
   back up that storage separately; those objects are not contained in the
   SQLite backup.

```bash
docker compose down
stamp=$(date +%Y%m%d-%H%M%S)
mkdir -p "backups/$stamp"
cp -a data .env docker-compose.yml "backups/$stamp/"
```

Keep the existing mount exactly as it is:

```text
./data:/app/data
```

Do not delete or recreate `data/app.sqlite3` during an upgrade.

### Container permissions

The production image runs as the dedicated unprivileged `memora` user and
group. Before the first start, make the host-mounted directory writable by
that identity. Resolve the numeric IDs from the image instead of assuming a
fixed UID/GID:

```bash
docker run --rm --entrypoint id ghcr.io/liushen-fork/memora:1.1.2 memora
# Example output: uid=100(memora) gid=101(memora)
sudo chown -R 100:101 data
sudo chmod -R u+rwX data
```

Replace `100:101` with the UID/GID printed by your image. Apply the same
ownership to an existing `data/` directory after migrating from ChronoFrame;
otherwise SQLite, logs, thumbnails, and generated media may fail with
permission errors.

## ChronoFrame to Memora

### Recommended path

The recommended starting point is **ChronoFrame `1.0.0-RC4`**. RC4 contains
the complete Drizzle migration chain used by the original application. If the
installation is older, upgrade ChronoFrame to RC4 first and let that release
finish its migrations while the original application is running.

1. Back up the files listed above.
2. Stop Memora (if it is already installed).
3. Start `ghcr.io/hoshinosuzumi/chronoframe:v1.0.0-rc.4` with the same
   `./data:/app/data` mount, or run RC4's `pnpm db:migrate` against the copied
   database. The RC4 container stays running after migration; stop it after
   you have verified the old gallery.
4. Wait for the RC4 migration to complete and verify that the old gallery can
   log in and browse the library.
5. Stop ChronoFrame, then start Memora with the same data mount.

When RC4 starts, Drizzle reads its migration history table and applies every
pending migration in order. This is why an older ChronoFrame database should
be brought to RC4 before the hand-off. The process still requires a valid
database, an intact migration history, and a backup; it is not a substitute for
those checks.

Example transition with Docker:

```bash
docker run -d --name chronoframe-rc4 \
  -v "$(pwd)/data:/app/data" \
  --env-file .env \
  -e DATABASE_URL=/app/data/app.sqlite3 \
  ghcr.io/hoshinosuzumi/chronoframe:v1.0.0-rc.4

docker logs -f chronoframe-rc4
# After verifying the RC4 gallery, stop the old service:
docker stop chronoframe-rc4
docker rm chronoframe-rc4

docker run -d --name memora -p 3000:3000 \
  -v "$(pwd)/data:/app/data" --env-file .env \
  ghcr.io/liushen-fork/memora:1.1.2
```

The RC4 image normally starts its migration before the Nuxt server. If you use
the source checkout instead, run `pnpm install` and `pnpm db:migrate` from the
ChronoFrame repository with `DATABASE_URL` pointing at the backup database.

### Can an old ChronoFrame database jump directly to Memora?

Do not assume that it can. ChronoFrame used these ordered migrations:

| Migration | Main change |
| --- | --- |
| `0000` | `photos` and `users` tables |
| `0001` | Photo coordinates and country/city fields |
| `0002` | Live Photo flags and video fields |
| `0003` | Photo thumbnail key |
| `0004`-`0006` | Processing queue and queue payload compatibility |
| `0007` | Photo reactions |
| `0008` | Albums and album-photo relations |
| `0009` | Settings and storage-provider settings |
| `0010` | Hidden albums |
| `0011` | Location-language enum update |

The current Go server does **not** include a general Drizzle-style migration
runner. On startup it creates missing tables and indexes and inserts missing
default settings. It does not reliably add a missing column to an existing
table, replay the historical `ALTER TABLE` steps, or repair an incomplete old
migration. Therefore:

- A database already migrated through RC4 is the supported hand-off point.
- A database from an earlier ChronoFrame release should be upgraded to RC4
  first.
- A direct jump is only reasonable when you have independently confirmed that
  the schema already contains all current tables and columns. It is not a
  compatibility guarantee.
- If Memora reports a schema error, stop it and restore the backup. Do not
  delete the database or let a partially repaired copy become the only copy.

Memora also adds missing default settings on startup and turns off the first
launch wizard when existing users are found. Seeing the wizard with a database
that has no users is expected; seeing it with an initialized user table should
be investigated rather than solved by creating a new database.

You do not have to install Memora `1.0.0` before `1.1.2`. Both are Go-based
releases and do not replay the old Drizzle migrations. After RC4 has produced
the supported schema, you may switch directly to a later Memora tag listed as
compatible in its release notes. Running Memora `1.0.0` first is still a valid
staged rollout if you want to verify the hand-off before continuing.

## Updating Memora

Use an explicit image tag for production and move one release at a time when
possible. `latest` is convenient for testing but makes rollback less precise.

```bash
docker compose pull
docker compose up -d
docker compose logs -f memora
```

For Compose, only change the image name or tag; keep the existing volumes and
environment settings. The current release image is:

```text
ghcr.io/liushen-fork/memora:1.1.2
```

The Go server does not need Node.js, Nitro, Drizzle, or a manual migration
command at runtime. Node.js and pnpm are only needed when rebuilding the
frontend from source.

## Release Version and Administration Lists

The checked-in frontend package uses `0.0.0-dev` for local development. The
release workflow accepts a version such as `1.1.2`, creates the `v1.1.2` tag,
and passes it to the frontend build as `MEMORA_VERSION`; no package manifest
edit is required. The same workflow publishes the matching Docker tag and
attaches cross-linked commit notes and binaries to the GitHub release.

The photo library and queue pages request bounded pages instead of loading the
entire table. The API accepts `page` and `pageSize` (maximum `100`) and returns
`data`, `page`, `pageSize`, `total`, and `totalPages`. Existing callers that omit
these parameters continue to receive the legacy array/list shape where
applicable.

## Data and Generated Files

The SQLite file and the mounted `data/` layout are preserved. Original media
stored by a remote provider remains in that provider. Memora may generate or
regenerate thumbnails and Live Photo MP4 files through its processing queue;
those generated objects are implementation details and should not be treated
as a replacement for the original media. Failed Live Photo processing falls
back to an ordinary photo until the task is retried.

Always read the release notes before upgrading across a change that mentions
database fields, storage keys, or generated media.
