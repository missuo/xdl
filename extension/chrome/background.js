// XDL browser bridge client. Clicking the toolbar button sends the current
// X/Twitter profile's username to the local XDL app (xdl-core listens on
// 127.0.0.1:47333 and only accepts requests from browser extensions).

const BRIDGE = "http://127.0.0.1:47333";

const RESERVED = new Set([
  "home", "explore", "notifications", "messages", "search", "settings",
  "i", "compose", "bookmarks", "jobs", "logout", "login", "tos", "privacy",
  "about", "hashtag", "intent", "share", "account", "communities", "premium",
  "follower_requests", "verified-choose", "download",
]);

function usernameFromURL(rawURL) {
  let url;
  try {
    url = new URL(rawURL);
  } catch {
    return null;
  }
  if (!/^(www\.|mobile\.)?(x|twitter)\.com$/.test(url.hostname)) {
    return null;
  }
  const segments = url.pathname.split("/").filter(Boolean);
  if (segments.length === 0) {
    return null;
  }
  const handle = segments[0];
  if (RESERVED.has(handle.toLowerCase())) {
    return null;
  }
  if (!/^[A-Za-z0-9_]{1,15}$/.test(handle)) {
    return null;
  }
  return handle;
}

function flashBadge(text, color) {
  chrome.action.setBadgeBackgroundColor({ color });
  chrome.action.setBadgeText({ text });
  setTimeout(() => chrome.action.setBadgeText({ text: "" }), 2500);
}

chrome.action.onClicked.addListener(async (tab) => {
  const handle = usernameFromURL(tab?.url || "");
  if (!handle) {
    flashBadge("?", "#FF9500"); // not on a profile page
    return;
  }
  try {
    const response = await fetch(`${BRIDGE}/enqueue`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: handle }),
    });
    if (response.ok) {
      flashBadge("✓", "#34C759");
    } else {
      flashBadge("!", "#FF3B30");
    }
  } catch {
    flashBadge("✗", "#FF3B30"); // XDL app not running
  }
});
