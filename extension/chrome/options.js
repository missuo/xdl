const DEFAULT_URL = "http://127.0.0.1:47333";
const $ = (id) => document.getElementById(id);

function show(text, ok) {
  $("status").textContent = text;
  $("status").className = ok === undefined ? "" : ok ? "ok" : "bad";
}

function readForm() {
  const url = ($("url").value.trim() || DEFAULT_URL).replace(/\/+$/, "");
  return { url, token: $("token").value.trim() };
}

// A server other than this computer's app needs the permission to reach it.
async function permit(url) {
  if (url === DEFAULT_URL || url === "http://localhost:47333") return true;
  let origin;
  try {
    origin = new URL(url).origin + "/*";
  } catch {
    show("That is not a URL.", false);
    return false;
  }
  if (await chrome.permissions.contains({ origins: [origin] })) return true;
  const granted = await chrome.permissions.request({ origins: [origin] });
  if (!granted) show("Chrome needs the permission to reach " + new URL(url).host + ".", false);
  return granted;
}

async function test() {
  const { url, token } = readForm();
  if (!(await permit(url))) return;
  show("Connecting…");
  try {
    const headers = token ? { Authorization: `Bearer ${token}` } : {};
    const response = await fetch(url + "/api/v1/status", { headers });
    if (response.status === 401) {
      show("XDL answered, but wants the API token.", false);
      return;
    }
    if (!response.ok) {
      show(`XDL answered HTTP ${response.status}.`, false);
      return;
    }
    const st = await response.json();
    const where = st.kind === "serve" ? "xdl serve" : "the app";
    const warnings = [];
    if (!st.auth_token) warnings.push("no X auth token");
    show(`Connected to XDL ${st.version} (${where})` + (warnings.length ? ` — ${warnings.join(", ")}` : "."), warnings.length === 0);
  } catch {
    show("XDL is not reachable at " + url + ".", false);
  }
}

async function save() {
  const { url, token } = readForm();
  if (!(await permit(url))) return;
  await chrome.storage.local.set({ url: url === DEFAULT_URL ? "" : url, token });
  show("Saved.", true);
  test();
}

(async () => {
  const { url, token } = await chrome.storage.local.get(["url", "token"]);
  $("url").value = url || "";
  $("token").value = token || "";
})();
$("test").addEventListener("click", test);
$("save").addEventListener("click", save);
