# XDL Browser Extension

Sends the X/Twitter profile you are viewing to XDL — the desktop app or
`xdl serve` — which fetches the account and downloads what is new, with
XDL's download settings. Accounts are remembered: sending one again only
fetches and downloads what it posted since.

## Install (Chrome / Edge / Arc / Brave)

1. Open `chrome://extensions`
2. Turn on **Developer mode** (top right)
3. Click **Load unpacked** and pick this `chrome` folder
   (in the app: Settings → Integrations → Show Extension Folder)

## Use

- **Click** the XDL button on a profile (`x.com/<user>`, or
  `x.com/<user>/likes` for likes): the badge shows the progress, then `✓`
- **Right-click** the button, or a link to a profile, for
  **Subscribe in XDL (check daily)**: XDL checks the account every day and
  downloads its new posts
- Badges: `?` not a profile page · `✗` XDL is not running · `!` XDL refused

XDL listens on `127.0.0.1:47333` and accepts requests from browser
extensions only (web pages cannot reach it).
