# XDL

Batch downloader for X/Twitter media timelines, written in Go: a desktop
app for macOS and Windows built with [MyGo](https://github.com/egoist/mygo)
(native UI, menu bar icon), a CLI, a headless server for Linux and Docker,
a local API the Chrome extension sends jobs to, and an MCP server for AI
agents.

> [!CAUTION]
> **Security notice (2026-11-10): XDL 2.0.0 and earlier ran xtractor, which
> uploads your auth token to its author.** Update to 2.0.1 or later, which
> reads X with its own open-source code, revoke the token you used, and
> delete xtractor — [details and steps](#security-notice-xtractor-steals-auth-tokens).
>
> **安全公告（2026-11-10）：XDL 2.0.0 及更早版本使用的 xtractor 会把你的 auth token 上传给它的作者。**
> 请升级到 2.0.1 或更新版本（改用 XDL 自己的开源代码读取 X），作废用过的 token，并删除 xtractor。[详情和处理步骤](#security-notice-xtractor-steals-auth-tokens)。

## Security notice: xtractor steals auth tokens

XDL fetched timelines with [xtractor](https://github.com/afkarxyz/xtractor-bin),
a closed-source binary by afkarxyz (the author of Twitter-X-Media-Batch-Downloader,
which XDL started from) that XDL downloaded on first use and handed your X
`auth_token`. A static analysis of the binaries on 2026-11-10 found that
xtractor uploads that token to its author:

- **v1.3** (Go, `github.com/afkarxyz/xtractor-build`, the build XDL 2.0.0
  installed) has a `grabber` package: `storeAuthTokenInD1` stores the token
  in a Cloudflare D1 database, `uploadGist` uploads the fetched timeline to
  a GitHub Gist and `sendToDiscord` posts it to a Discord webhook. The token
  upload runs before its own `-no-discord` flag is checked, so that flag does
  not stop it.
- **v1.0.0 of the new `afkarxyz/xtractor-bin`** (Rust) has `src/grabber.rs`,
  the same Discord webhook, and the hosts `auth.twitterdl.app` (`/insert`)
  and `json.twitterdl.app` (`/v1/files`).

Indicators: Discord webhook ID `1364020058397085828`; hosts
`auth.twitterdl.app`, `json.twitterdl.app`; SHA-256
`30021864df15406cbe6c9ecdf99820f5ad25ea9f948160c175731ebb04064a2e` (v1.3,
macOS arm64), `7516675babcc3033e033cbf4f8a3cf366f3a89ef1aa5a1eeba83a855b4b1319d`
(xtractor-bin v1.0.0, macOS arm64),
`682f9e2d060863fe65afb9a1d158523e2a821ab796411fdc992a278c67e1cb9b`
(xtractor-bin v1.0.0, Windows amd64).

XDL's own code sends the token nowhere but x.com. Since 2.0.1, XDL no
longer downloads or runs xtractor: it reads X with its own open-source code
(`backend/x*.go`). 2.0.0 was withdrawn.

**If you entered an auth token in XDL or Twitter-X-Media-Batch-Downloader:**

1. Revoke it: on x.com, Settings → Security and account access → Apps and
   sessions → Sessions → **Log out of all other sessions** (or change your
   password). The old token stops working at once.
2. Delete xtractor: `~/.twitterxmediabatchdownloader/xtractor` (macOS,
   Linux), `%USERPROFILE%\.twitterxmediabatchdownloader\xtractor.exe`
   (Windows), and `extractor` / `extractor.exe` from older versions. Your
   library (`accounts.db`) in the same folder is safe to keep.
3. Uninstall Twitter-X-Media-Batch-Downloader and XDL 2.0.0 or earlier,
   which download xtractor too.
4. Install XDL 2.0.1 or later, and enter a new token there.

**如果你在 XDL 或 Twitter-X-Media-Batch-Downloader 里填过 auth token：**

1. 作废它：在 x.com 打开「设置 → 安全和账户访问 → 应用和会话 → 会话」，点**注销所有其他会话**（或者改密码），旧 token 立即失效。
2. 删除 xtractor：macOS / Linux 是 `~/.twitterxmediabatchdownloader/xtractor`，Windows 是 `%USERPROFILE%\.twitterxmediabatchdownloader\xtractor.exe`，旧版本里叫 `extractor` / `extractor.exe`。同一个目录里的资料库 `accounts.db` 可以保留。
3. 卸载 Twitter-X-Media-Batch-Downloader 和 XDL 2.0.0 及更早版本，它们也会下载 xtractor。
4. 安装 XDL 2.0.1 或更新版本，再填入新的 token。

## Features

- **Download an account's media** — photos, videos, GIFs, optionally text
  tweets and retweets, from its media timeline, tweets, replies, likes or
  your bookmarks: templated file names, parallel downloads, retries,
  metadata embedding (ExifTool) and GIF conversion (FFmpeg)
- **Never twice** — a refetch stops paging where the library's copy begins,
  and the download history skips every photo and video downloaded before by
  its X media ID, without a request, even after files were renamed, moved
  or deleted or the name templates changed (`-redownload` overrides it)
- **Resumable downloads** — a dropped or stopped download continues
  where it stopped (HTTP Range) instead of starting over; files of 50 MB and
  more download in four segments at once; a transfer fails only after 60
  seconds without data, so long videos on slow lines finish
- **Subscriptions** — XDL checks accounts on a schedule (hourly to weekly)
  and downloads what they post; a check missed while XDL was off runs when
  it starts, a failed one again an hour later
- **One queue** for everything: the app, the browser extension, the CLI,
  agents and subscriptions all submit jobs to the same queue
- **Menu bar icon** — a ring spins while fetching, fills with the
  download's progress, and shows a check when it is done; closing the
  window keeps XDL there
- **Its own X client** — XDL reads timelines from X's web API itself, in
  open-source Go (`backend/x*.go`): the auth token goes to x.com and
  nowhere else, and there is no binary to download
- Library of fetched accounts (SQLite), optional FFmpeg and ExifTool
  (Homebrew installs are used when present), auth token in the system
  keychain

## Install

### Homebrew

```sh
# The app for macOS, with the `xdl` command
brew install --cask owo-network/brew/xdl

# Or the CLI alone, for macOS and Linux
brew install owo-network/brew/xdl-cli
brew services start xdl-cli   # optional: `xdl serve` in the background
```

Install one or the other: both put `xdl` on the `PATH`. For a plain
`brew upgrade` to update XDL too, trust the tap once:
`brew trust owo-network/brew`.

### Download

Every [release](https://github.com/missuo/xdl/releases/latest) has:

| | |
|---|---|
| `XDL-<v>-macos-universal.dmg` / `.zip` | the app for macOS 13+ (Apple silicon and Intel), signed and notarized |
| `XDL-<v>-windows-<arch>-setup.exe` / `-portable.zip` | the app for Windows 10+ (amd64, arm64) |
| `xdl_<v>_linux_<arch>.deb`, `xdl-<v>-linux-<arch>.tar.gz` | the app for Linux (amd64, arm64; GTK 3); `install.sh` installs the tarball in your home folder |
| `xdl-cli-<v>-<os>-<arch>.tar.gz` / `.zip` | the `xdl` CLI alone, for macOS, Linux and Windows |
| `xdl-chrome-extension-<v>.zip` | the browser extension |
| `ghcr.io/missuo/xdl:<v>` | the Docker image (`xdl serve`) |
| `xdl-<v>-<platform>.tar.gz`, `update-<platform>.json`, `*.delta` | the signed updates the apps install themselves |

### Updates

The app updates itself, as Sparkle does on the Mac: it checks in the
background once a day, shows a new version's release notes and offers to
install it (Settings → General, or "Check for Updates…" in the menu bar
icon's menu, checks at once). Updates are signed with XDL's release key,
and an update that key did not sign is refused. Copies a package manager
installed, as the `.deb`, update with it instead; the Homebrew cask's app
updates itself.

## Use

### The app

Install it (above, or build it with `make app`), add your X `auth_token`
cookie in Settings → General. FFmpeg (GIF conversion) and ExifTool
(metadata) are optional, in Settings → Dependencies.

### The CLI

```sh
xdl token set                      # the auth_token cookie (keychain)
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

The extension adds an XDL button to Chrome (and Edge, Brave, Arc, Vivaldi):
on an X profile it sends the account to XDL, which downloads what is new,
and the badge shows the progress. Right-click the button, or a link to a
profile, to subscribe instead.

**Install**

1. Get the extension: unzip `xdl-chrome-extension-<version>.zip` from the
   [latest release](https://github.com/missuo/xdl/releases/latest) into a
   folder you keep, or use the copy inside the app (Settings → Integrations
   → Show Extension Folder), or `extension/chrome` in this repository.
2. Open `chrome://extensions` (`edge://extensions` in Edge).
3. Turn on **Developer mode** (top right).
4. Click **Load unpacked** and choose the folder holding `manifest.json`.
5. Pin the XDL button: the puzzle-piece menu next to the address bar → the
   pin beside XDL.

Chrome keeps loading the extension from that folder: don't delete it. To
update, replace the folder's files and click the reload arrow on XDL's card
in `chrome://extensions`.

**Connect it**

- To the app on the same computer: nothing to do. The extension talks to
  `http://127.0.0.1:47333`, which trusts browser extensions without a token.
- To `xdl serve` on a server or in Docker: right-click the XDL button →
  **Options**, enter the server's address (`http://nas.local:47333`) and its
  API token (`XDL_API_TOKEN`), then **Save** — Chrome asks for permission to
  reach that address — and **Test**.

Badges: `↓` queued, a percentage while downloading, `✓` done, `★`
subscribed, `?` not a profile page, `✗` XDL not reachable, `!` refused
(hover the button for why). See also [extension/README.md](extension/README.md).

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
backend/          the X client (x*.go), downloads, library, history, subscriptions
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
by afkarxyz (GPL-3.0). XDL no longer uses its companion xtractor binary
(see the [security notice](#security-notice-xtractor-steals-auth-tokens));
the X client in `backend/x*.go` is XDL's own, and its transaction ID
signing follows the algorithm documented by the open-source
XClientTransaction project. Icons from [IconMCP](https://iconmcp.com) (MIT).

## License

GPL-3.0 — see [LICENSE](LICENSE).
