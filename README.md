# XDL

Batch downloader for X/Twitter media timelines, written in Go: a desktop
app for macOS and Windows built with [MyGo](https://github.com/egoist/mygo)
(native UI, menu bar icon), a CLI, a headless server for Linux and Docker,
a local API the Chrome extension sends jobs to, and an MCP server for AI
agents.

## Features

- **Download an account's media** — photos, videos, GIFs, optionally text
  tweets and retweets, from its media timeline, tweets, replies, likes or
  your bookmarks: templated file names, parallel downloads, retries,
  metadata embedding (ExifTool) and GIF conversion (FFmpeg)
- **Never twice** — a refetch stops paging where the library's copy begins,
  and the download history skips every photo and video downloaded before by
  its X media ID, without a request, even after files were renamed, moved
  or deleted or the name templates changed (`-redownload` overrides it)
- **Subscriptions** — XDL checks accounts on a schedule (hourly to weekly)
  and downloads what they post; a check missed while XDL was off runs when
  it starts, a failed one again an hour later
- **One queue** for everything: the app, the browser extension, the CLI,
  agents and subscriptions all submit jobs to the same queue
- **Menu bar icon** — a ring spins while fetching, fills with the
  download's progress, and shows a check when it is done; closing the
  window keeps XDL there
- Library of fetched accounts (SQLite), dependency manager (xtractor,
  FFmpeg, ExifTool; Homebrew installs are used when present), auth token in
  the system keychain

## Use

### The app

Build it with `make app` (or run it with `make dev`), add your X
`auth_token` cookie in Settings → General, and install xtractor in
Settings → Dependencies.

### The CLI

```sh
xdl token set                      # the auth_token cookie (keychain)
xdl deps install xtractor
xdl download nasa                  # fetch + download what is new
xdl download nasa -type likes -out ~/Downloads/x
xdl fetch nasa                     # library only
xdl sub add nasa 1d                # check every day
xdl sub ls | xdl jobs | xdl watch
xdl history                        # the download history
xdl config set concurrent_downloads 20
```

With the app (or `xdl serve`) running, `download` hands the job to its
queue and follows it; `-local` runs it in the CLI's process instead.
`XDL.app/Contents/MacOS/XDL` is the CLI too:
`sudo ln -sf /Applications/XDL.app/Contents/MacOS/XDL /usr/local/bin/xdl`.
On Windows, `XDL.exe` has no console; use the release's `xdl.exe`.

### Headless: Linux and Docker

`xdl serve` runs the queue, the subscriptions, the API and MCP without the
GUI. Listening on another address than `127.0.0.1` requires the API token on
every request; `XDL_API_TOKEN` fixes it.

```sh
XDL_AUTH_TOKEN=… XDL_API_TOKEN=change-me docker compose up -d
XDL_API_URL=http://server:47333 XDL_API_TOKEN=change-me xdl sub add nasa 1d
```

| Variable | |
|---|---|
| `XDL_AUTH_TOKEN` | the X `auth_token`, instead of the keychain (no keyring: `xdl token set` writes a 0600 file) |
| `XDL_API_ADDR` | the API's address (default `127.0.0.1:47333`; the image uses `0.0.0.0:47333`) |
| `XDL_API_TOKEN` | the API's bearer token (default: random, in `api.json`) |
| `XDL_DOWNLOAD_DIR` | where downloads go when the setting is empty (default `~/Pictures`) |
| `XDL_API_URL` | for the CLI: an XDL elsewhere |

### The Chrome extension

Load `extension/chrome` unpacked (Settings → Integrations → Show Extension
Folder). Its button sends the profile in the tab to XDL and shows the
progress; right-click it to subscribe. See [extension/README.md](extension/README.md).

### AI agents (MCP)

```sh
claude mcp add xdl -- xdl mcp
```

`xdl mcp` serves MCP over stdio, through the running XDL when there is one.
The API serves it over HTTP too, at `http://127.0.0.1:47333/mcp` with the
bearer token. Tools: `download_account`, `get_job`, `list_jobs`,
`cancel_job`, `search_library`, `subscribe`, `list_subscriptions`,
`update_subscription`, `unsubscribe`, `check_subscription_now`,
`xdl_status`.

### The API

`127.0.0.1:47333`, JSON. Browser extensions (by their `Origin`) and local
programs with the token from `api.json` (in `~/Library/Application
Support/XDL`) may call it; web pages may not.

| | |
|---|---|
| `GET /api/v1/status` | readiness, the running job |
| `POST /api/v1/jobs` | `{"username", "timeline_type", "mode": "download\|fetch\|library", "full", "redownload"}` |
| `GET /api/v1/jobs`, `GET /api/v1/jobs/{id}`, `GET /api/v1/jobs/{id}/items` | |
| `POST /api/v1/jobs/{id}/cancel`, `DELETE /api/v1/jobs/{id}`, `POST /api/v1/jobs/clear` | |
| `GET /api/v1/events` | server-sent events (`?job=ID`, `?items=1`) |
| `GET/POST /api/v1/subscriptions`, `PATCH/DELETE /api/v1/subscriptions/{id}`, `POST …/{id}/run` | `{"username", "timeline_type", "interval": "1d", "enabled"}` |
| `GET /api/v1/library` | |
| `POST /mcp` | MCP (streamable HTTP) |

## Build

Requires Go 1.27.3+. Xcode 26 compiles the macOS icon; nothing else is
needed: MyGo builds without cgo.

```sh
make app       # build/darwin-<arch>/XDL.app and a .dmg
make windows   # build/windows-amd64/XDL.exe (+ installer with NSIS)
make cli       # build/xdl
make release   # tests, the app, and the CLI for macOS, Linux and Windows
make docker    # the headless image
make test
```

Pushing a `v*` tag builds, signs and notarizes everything in GitHub Actions
(`.github/workflows/release.yml`).

Data stays where the original app kept it: the library, download history,
subscriptions and tools in `~/.twitterxmediabatchdownloader/`, settings in
`~/Library/Application Support/XDL/settings.json` (shared by the app, the
CLI and the server; the Objective-C app's preferences are imported once).

## Layout

```
backend/          extraction (xtractor), downloads, library, history, subscriptions
internal/engine   the job queue: fetch → library → download
internal/schedule subscriptions
internal/server   the HTTP API (+ /mcp)
internal/mcp      MCP tools
internal/cli      the CLI
internal/gui      the MyGo app: pages, menu bar icon
cmd/xdl           the CLI binary        cmd/xdl-app   the app (also a CLI)
extension/chrome  the browser extension
```

## Credits

The extraction/download backend is derived from
[Twitter-X-Media-Batch-Downloader](https://github.com/afkarxyz/Twitter-X-Media-Batch-Downloader)
by afkarxyz (GPL-3.0). Timeline extraction uses its companion
[xtractor](https://github.com/afkarxyz/xtractor-binaries) binary. Icons
from [IconMCP](https://iconmcp.com) (MIT).

## License

GPL-3.0 — see [LICENSE](LICENSE).
