# XDL Browser Extension

Sends the X/Twitter profile you are viewing to the XDL desktop app, which
fetches the account's media timeline and downloads everything automatically
(saved into the Library, using the app's download settings).

## Install (Chrome / Edge / Arc)

1. Open `chrome://extensions`
2. Enable **Developer mode** (top right)
3. Click **Load unpacked** and select the `extension/chrome` folder

## Use

1. Make sure the XDL app is running (it hosts the local bridge on
   `127.0.0.1:47333`, extension-origin requests only)
2. Open any profile on x.com / twitter.com
3. Click the XDL toolbar button:
   - `✓` queued — XDL fetches and downloads in the background;
     a system notification fires when it finishes and the Library refreshes
   - `?` the current tab is not a profile page
   - `✗` the XDL app is not running
