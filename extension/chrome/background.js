// XDL browser extension. The toolbar button sends the X/Twitter profile in
// the current tab to the XDL app (or `xdl serve`) on 127.0.0.1:47333, which
// fetches it and downloads what is new. Right-click the button or a profile
// link to subscribe instead: XDL then checks the account every day.

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
  if (RESERVED.has(handle.toLowerCase()) || !/^[A-Za-z0-9_]{1,15}$/.test(handle)) {
    return null;
  }
  // /<user>/likes queues the likes timeline, the rest the media timeline.
  const timeline = segments[1] === "likes" ? "likes" : "media";
  return { handle, timeline };
}

function badge(text, color, title, clearAfter = 2500) {
  chrome.action.setBadgeBackgroundColor({ color });
  chrome.action.setBadgeText({ text });
  if (title) {
    chrome.action.setTitle({ title });
  }
  if (clearAfter) {
    setTimeout(() => chrome.action.setBadgeText({ text: "" }), clearAfter);
  }
}

async function post(path, body) {
  const response = await fetch(BRIDGE + path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  let data = {};
  try {
    data = await response.json();
  } catch {}
  return { response, data };
}

// follow shows a job's progress on the button until it finishes.
async function follow(jobID, handle) {
  for (let i = 0; i < 1800; i++) {
    await new Promise((r) => setTimeout(r, 2000));
    let job;
    try {
      const response = await fetch(`${BRIDGE}/api/v1/jobs/${jobID}`);
      if (!response.ok) return;
      job = await response.json();
    } catch {
      return;
    }
    if (job.state === "downloading") {
      badge(`${job.percent}%`, "#0A84FF", `XDL: downloading @${handle} — ${job.current}/${job.total}`, 0);
    } else if (job.state === "fetching") {
      badge("…", "#0A84FF", `XDL: fetching @${handle}`, 0);
    } else if (job.state === "done") {
      badge("✓", "#34C759", `XDL: @${handle} — ${job.message}`);
      return;
    } else if (job.state === "failed" || job.state === "cancelled") {
      badge("!", "#FF3B30", `XDL: @${handle} — ${job.error || job.message}`);
      return;
    }
  }
}

async function download(target) {
  try {
    let { response, data } = await post("/api/v1/jobs", {
      username: target.handle, timeline_type: target.timeline, source: "extension",
    });
    if (response.status === 404) {
      // XDL 1.x only knows /enqueue.
      ({ response, data } = await post("/enqueue", { username: target.handle }));
      if (response.ok) badge("✓", "#34C759", `XDL: queued @${target.handle}`);
      else badge("!", "#FF3B30", `XDL: rejected (HTTP ${response.status})`);
      return;
    }
    if (!response.ok) {
      badge("!", "#FF3B30", `XDL: ${data.error || "HTTP " + response.status}`);
      return;
    }
    badge("↓", "#0A84FF", `XDL: queued @${target.handle}`, 0);
    follow(data.job.id, target.handle);
  } catch {
    badge("✗", "#FF3B30", "XDL: the app is not running");
  }
}

async function subscribe(target) {
  try {
    const { response, data } = await post("/api/v1/subscriptions", {
      username: target.handle, timeline_type: target.timeline, interval: "1d",
    });
    if (response.ok) {
      badge("★", "#34C759", `XDL: subscribed to @${target.handle} — checked daily`);
    } else {
      badge("!", "#FF3B30", `XDL: ${data.error || "HTTP " + response.status}`);
    }
  } catch {
    badge("✗", "#FF3B30", "XDL: the app is not running");
  }
}

chrome.action.onClicked.addListener((tab) => {
  const target = usernameFromURL(tab?.url || "");
  if (!target) {
    badge("?", "#FF9500", "XDL: open an X profile page first");
    return;
  }
  download(target);
});

chrome.runtime.onInstalled.addListener(() => {
  const profiles = ["*://x.com/*", "*://*.x.com/*", "*://twitter.com/*", "*://*.twitter.com/*"];
  chrome.contextMenus.create({ id: "download-page", title: "Download this profile with XDL", contexts: ["action"] });
  chrome.contextMenus.create({ id: "subscribe-page", title: "Subscribe in XDL (check daily)", contexts: ["action"] });
  chrome.contextMenus.create({
    id: "download-link", title: "Download this profile with XDL", contexts: ["link"], targetUrlPatterns: profiles,
  });
  chrome.contextMenus.create({
    id: "subscribe-link", title: "Subscribe in XDL (check daily)", contexts: ["link"], targetUrlPatterns: profiles,
  });
});

chrome.contextMenus.onClicked.addListener((info, tab) => {
  const url = info.menuItemId.endsWith("-link") ? info.linkUrl : tab?.url;
  const target = usernameFromURL(url || "");
  if (!target) {
    badge("?", "#FF9500", "XDL: that is not an X profile");
    return;
  }
  if (info.menuItemId.startsWith("subscribe")) subscribe(target);
  else download(target);
});
