# XDL Browser Extension

Sends the X/Twitter profile you are viewing to XDL — the desktop app or
`xdl serve` — which fetches the account and downloads what is new, with
XDL's download settings. Accounts are remembered: sending one again only
fetches and downloads what it posted since.

## Install (Chrome / Edge / Arc / Brave)

1. Unzip `xdl-chrome-extension-<version>.zip` from the latest release into
   a folder you keep (or use this `chrome` folder, or the app's: Settings →
   Integrations → Show Extension Folder)
2. Open `chrome://extensions`, turn on **Developer mode** (top right)
3. Click **Load unpacked** and pick the folder holding `manifest.json`
4. Pin the button from the puzzle-piece menu

## Connect

- The app on this computer: nothing to set; `127.0.0.1:47333` trusts the
  extension without a token.
- `xdl serve` elsewhere (a server, Docker): right-click the button →
  **Options** → its address and API token (`XDL_API_TOKEN`) → **Save**, then
  **Test**.

## Use

- **Click** the XDL button on a profile (`x.com/<user>`, or
  `x.com/<user>/likes` for likes): the badge shows the progress, then `✓`
- **Right-click** the button, or a link to a profile, for
  **Subscribe in XDL (check daily)**: XDL checks the account every day and
  downloads its new posts
- Badges: `?` not a profile page · `✗` XDL is not running · `!` XDL refused

XDL listens on `127.0.0.1:47333` and accepts requests from browser
extensions only (web pages cannot reach it); a remote `xdl serve` requires
the API token on every request.
