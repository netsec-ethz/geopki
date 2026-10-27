// GeoPKI admin dashboard (lab tooling): lists every certificate in the server's
// database with its extensions, inserts certificates, expires them, releases.
// All endpoints need the server's insertion key.
//
// Certificate content comes from whoever inserted it, so it is only ever put
// into the page as text, never as HTML.

const SESSION_STORAGE_KEY = "geopki_admin_key";
// the server caps one listing at this many certificates
const PAGE = 500;

const state = {
  key: sessionStorage.getItem(SESSION_STORAGE_KEY) || "",
  entries: [],
  filter: "all",
  search: "",
  expanded: new Set(),
};

const el = {
  keyInput: document.getElementById("admin-key"),
  connectButton: document.getElementById("connect-button"),
  forgetKeyButton: document.getElementById("forget-key-button"),
  authStatus: document.getElementById("auth-status"),
  dashboard: document.getElementById("dashboard"),
  summary: document.getElementById("summary"),
  filter: document.getElementById("filter"),
  search: document.getElementById("search"),
  refreshButton: document.getElementById("refresh-button"),
  insertToggle: document.getElementById("insert-toggle"),
  releaseButton: document.getElementById("release-button"),
  insertPanel: document.getElementById("insert-panel"),
  insertJson: document.getElementById("insert-json"),
  insertRelease: document.getElementById("insert-release"),
  insertCompact: document.getElementById("insert-compact"),
  insertButton: document.getElementById("insert-button"),
  insertStatus: document.getElementById("insert-status"),
  tableBody: document.getElementById("cert-table-body"),
  empty: document.getElementById("empty"),
};

// --- server API ---

function apiUrl(path, params) {
  const url = new URL(path, location.origin);
  url.searchParams.set("key", state.key);
  for (const [k, v] of Object.entries(params || {})) url.searchParams.set(k, v);
  return url.toString();
}

async function checked(response, what) {
  if (response.status === 401) throw new Error("Invalid insertion key");
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new Error(body.error || `${what} failed with status ${response.status}`);
  }
  return response.json();
}

// every certificate, page by page
async function fetchAllCertificates() {
  const all = [];
  for (let offset = 0; ; offset += PAGE) {
    const data = await checked(
      await fetch(apiUrl("/v1/admin/certificates", { limit: PAGE, offset })),
      "listing"
    );
    all.push(...data.certificates);
    if (data.certificates.length === 0 || all.length >= data.total) return all;
  }
}

async function expireCertificates(hashes) {
  return checked(
    await fetch(apiUrl("/v1/admin/expire"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ hashes }),
    }),
    "delete"
  );
}

// the server gunzips every /v1/insert body
async function gzip(text) {
  if (typeof CompressionStream === "undefined") {
    throw new Error("This browser lacks the Compression Streams API needed for inserting.");
  }
  const stream = new Blob([text]).stream().pipeThrough(new CompressionStream("gzip"));
  return new Response(stream).arrayBuffer();
}

async function insertCertificates(body) {
  return checked(
    await fetch(apiUrl("/v1/insert"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: await gzip(body),
    }),
    "insert"
  );
}

async function release() {
  return checked(await fetch(apiUrl("/v1/release"), { method: "POST" }), "release");
}

// --- small DOM helpers ---

function node(tag, options = {}, ...children) {
  const n = document.createElement(tag);
  if (options.className) n.className = options.className;
  if (options.text !== undefined) n.textContent = options.text;
  if (options.title) n.title = options.title;
  for (const child of children) if (child) n.appendChild(child);
  return n;
}

// appends child unless it is null (line() returns null for an empty value)
function add(parent, child) {
  if (child) parent.appendChild(child);
}

function line(label, value) {
  if (value === undefined || value === null || value === "") return null;
  return node("div", { className: "kv" }, node("span", { className: "k", text: label }), node("span", { text: String(value) }));
}

function setStatus(target, message, kind) {
  target.textContent = message;
  target.className = kind || "";
}

function formatDate(value) {
  if (!value) return "";
  const d = new Date(value);
  return isNaN(d.getTime()) ? String(value) : d.toLocaleString();
}

function plural(n, word) {
  return `${n} ${word}${n === 1 ? "" : "s"}`;
}

// --- certificates ---

function extensionNames(cert) {
  return Object.keys(cert.extensions || {}).sort();
}

function statusOf(entry) {
  const cert = entry.certificate || {};
  if (entry.expired) return { label: "Expired", kind: "expired", note: "removed at the next release" };
  const before = cert.not_valid_before && new Date(cert.not_valid_before);
  if (before && before > new Date()) return { label: "Not yet valid", kind: "pending", note: `from ${formatDate(before)}` };
  return { label: "Valid", kind: "active" };
}

// mean of a ring's vertices without the closing point
function ringCentre(ring) {
  const points = ring.slice(0, -1);
  const sum = points.reduce((acc, [lng, lat]) => [acc[0] + lng, acc[1] + lat], [0, 0]);
  return [sum[0] / points.length, sum[1] / points.length];
}

function areaCell(cert) {
  const td = node("td", { className: "area" });
  const areas = cert.areas || [];
  const ring = areas[0] && areas[0].coordinates && areas[0].coordinates[0] && areas[0].coordinates[0][0];
  if (!ring || ring.length < 2) {
    td.textContent = "-";
    return td;
  }
  const [lng, lat] = ringCentre(ring);
  td.appendChild(node("code", { text: `${lat.toFixed(5)}, ${lng.toFixed(5)}` }));
  const altitude = cert.areas_altitude && cert.areas_altitude[0];
  const notes = [];
  if (altitude) notes.push(`${altitude[0]}-${altitude[1]} m`);
  if (areas.length > 1) notes.push(`+${plural(areas.length - 1, "area")}`);
  if (notes.length) td.appendChild(node("div", { className: "muted", text: notes.join(" · ") }));
  return td;
}

// what a "wifi" extension says, in a few lines
function wifiDetails(wifi) {
  const box = node("div", { className: "ext-details" });
  const title = [wifi.ssid ? `"${wifi.ssid}"` : "(no SSID)", wifi.operator].filter(Boolean).join(" · ");
  box.appendChild(node("div", { className: "ext-title", text: title }));
  add(box, line("Security", (wifi.auth_modes || []).join(", ")));
  if (wifi.ssid_shared) add(box, line("SSID", "shared with other operators"));
  add(box, line("PMF", wifi.pmf));

  for (const domain of (wifi.portal && wifi.portal.domains) || []) {
    const pins = (domain.pinned_spki_sha256 || []).length;
    add(box, line(`Portal (${domain.role || "?"})`, `${domain.name} · ${plural(pins, "pin")}`));
  }

  const enterprise = wifi.enterprise;
  if (enterprise) {
    add(box, line("Enterprise", enterprise.federation ? `federation ${enterprise.federation}` : "closed network"));
    const aaa = enterprise.aaa || [];
    if (aaa.length === 0) add(box, line("Local users", "none (visitors only)"));
    for (const group of aaa) {
      add(box, line("Realms", (group.realms || []).join(", ") || "(any)"));
      for (const server of group.servers || []) {
        const pins = [];
        if ((server.pinned_spki_sha256 || []).length) pins.push(plural(server.pinned_spki_sha256.length, "leaf pin"));
        if ((server.ca_spki_sha256 || []).length) pins.push(plural(server.ca_spki_sha256.length, "CA pin"));
        if ((server.ca_certificates || []).length) pins.push(plural(server.ca_certificates.length, "CA certificate"));
        add(box, line("Server", `${(server.names || []).join(", ") || "(no name)"} · ${pins.join(", ")}`));
      }
    }
  }
  return box;
}

// extensions this dashboard has no summary for: their keys
function genericDetails(name, content) {
  const keys = content && typeof content === "object" ? Object.keys(content) : [];
  return node("div", { className: "ext-details" },
    node("div", { className: "ext-title", text: name }),
    line("Fields", keys.join(", ") || JSON.stringify(content)));
}

function detailsCell(cert) {
  const td = node("td", { className: "details" });
  const names = extensionNames(cert);
  if (names.length === 0) {
    td.appendChild(node("span", { className: "muted", text: "No extensions" }));
    return td;
  }
  for (const name of names) {
    const content = cert.extensions[name];
    td.appendChild(name === "wifi" && content && typeof content === "object" ? wifiDetails(content) : genericDetails(name, content));
  }
  return td;
}

function extensionsCell(cert) {
  const td = node("td");
  const critical = new Set(cert.critical || []);
  for (const name of extensionNames(cert)) {
    td.appendChild(node("span", {
      className: "chip" + (critical.has(name) ? " critical" : ""),
      text: name + (critical.has(name) ? " (critical)" : ""),
      title: critical.has(name) ? "Clients that don't know this extension ignore the certificate" : "",
    }));
  }
  // critical names without the extension would have been rejected on insertion
  for (const name of critical) {
    if (!(cert.extensions || {})[name]) td.appendChild(node("span", { className: "chip error", text: `${name} (critical, missing)` }));
  }
  if (!td.childNodes.length) td.appendChild(node("span", { className: "muted", text: "none" }));
  return td;
}

function renderEntry(entry) {
  const cert = entry.certificate || {};
  const status = statusOf(entry);
  const rows = [];

  const tr = node("tr", { className: "cert-row" });
  tr.tabIndex = 0;
  tr.setAttribute("aria-expanded", String(state.expanded.has(entry.hash)));

  const statusTd = node("td", {}, node("span", { className: `badge ${status.kind}`, text: status.label }));
  if (status.note) statusTd.appendChild(node("div", { className: "muted", text: status.note }));
  tr.appendChild(statusTd);

  tr.appendChild(node("td", {},
    node("div", { className: "cert-id", text: cert.certificate_id || "(no id)" }),
    node("div", { className: "muted", text: cert.domain || "(no domain)" })));
  tr.appendChild(extensionsCell(cert));
  tr.appendChild(detailsCell(cert));
  tr.appendChild(node("td", {},
    cert.not_valid_before ? node("div", { text: `from ${formatDate(cert.not_valid_before)}` }) : null,
    node("div", { text: `until ${formatDate(entry.not_valid_after)}` })));
  tr.appendChild(areaCell(cert));

  const actions = node("td");
  if (!entry.expired) {
    const button = node("button", { className: "danger", text: "Delete" });
    button.type = "button";
    button.addEventListener("click", (e) => {
      e.stopPropagation();
      onDelete(entry, button);
    });
    actions.appendChild(button);
  }
  tr.appendChild(actions);

  const toggle = () => {
    if (state.expanded.has(entry.hash)) state.expanded.delete(entry.hash);
    else state.expanded.add(entry.hash);
    render();
  };
  tr.addEventListener("click", toggle);
  tr.addEventListener("keydown", (e) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      toggle();
    }
  });
  rows.push(tr);

  if (state.expanded.has(entry.hash)) {
    const td = node("td", { className: "raw" });
    td.colSpan = 7;
    add(td, line("Hash (SHA-256 of the stored bytes)", entry.hash));
    td.appendChild(node("pre", { text: JSON.stringify(cert, null, 2) }));
    rows.push(node("tr", { className: "raw-row" }, td));
  }
  return rows;
}

function matchesFilter(entry) {
  const names = extensionNames(entry.certificate || {});
  if (state.filter === "none") return names.length === 0;
  if (state.filter !== "all" && !names.includes(state.filter)) return false;
  if (!state.search) return true;
  // the whole certificate as text: id, domain, SSID, hostnames, realms, ...
  return JSON.stringify(entry.certificate).toLowerCase().includes(state.search);
}

function renderFilter() {
  const counts = new Map();
  let without = 0;
  for (const entry of state.entries) {
    const names = extensionNames(entry.certificate || {});
    if (names.length === 0) without++;
    for (const name of names) counts.set(name, (counts.get(name) || 0) + 1);
  }
  const options = [["all", `All (${state.entries.length})`]];
  for (const [name, n] of [...counts].sort()) options.push([name, `${name} (${n})`]);
  options.push(["none", `No extensions (${without})`]);
  if (!options.some(([value]) => value === state.filter)) state.filter = "all";

  el.filter.textContent = "";
  for (const [value, label] of options) {
    const button = node("button", { className: "toggle", text: label });
    button.type = "button";
    button.setAttribute("aria-pressed", String(value === state.filter));
    button.addEventListener("click", () => {
      state.filter = value;
      render();
    });
    el.filter.appendChild(button);
  }
}

function render() {
  renderFilter();
  const shown = state.entries.filter(matchesFilter);
  el.tableBody.textContent = "";
  for (const entry of shown) for (const row of renderEntry(entry)) el.tableBody.appendChild(row);
  el.empty.hidden = shown.length > 0;

  const expired = state.entries.filter((e) => e.expired).length;
  el.summary.textContent =
    `${plural(state.entries.length, "certificate")}` +
    (expired ? `, ${expired} expired` : "") +
    (shown.length !== state.entries.length ? ` · showing ${shown.length}` : "");
}

async function load() {
  el.summary.textContent = "Loading...";
  try {
    state.entries = await fetchAllCertificates();
    render();
  } catch (e) {
    el.summary.textContent = `Couldn't load certificates: ${e.message}`;
  }
}

// --- actions ---

async function onDelete(entry, button) {
  const label = (entry.certificate && entry.certificate.certificate_id) || entry.hash;
  if (!confirm(`Delete "${label}"?\n\nIts expiry is set to now; it leaves the map at the next release.`)) return;
  button.disabled = true;
  button.textContent = "Deleting...";
  try {
    await expireCertificates([entry.hash]);
    await load();
  } catch (e) {
    alert(`Couldn't delete: ${e.message}`);
    button.disabled = false;
    button.textContent = "Delete";
  }
}

async function onRelease() {
  el.releaseButton.disabled = true;
  el.releaseButton.textContent = "Releasing...";
  try {
    const result = await release();
    el.summary.textContent = `Released (tree size ${result.new_tree_size}).`;
    await load();
  } catch (e) {
    alert(`Release failed: ${e.message}`);
  } finally {
    el.releaseButton.disabled = false;
    el.releaseButton.textContent = "Release";
  }
}

async function onInsert() {
  let parsed;
  try {
    parsed = JSON.parse(el.insertJson.value);
  } catch (e) {
    setStatus(el.insertStatus, `Not valid JSON: ${e.message}`, "error");
    return;
  }
  const list = Array.isArray(parsed) ? parsed : [parsed];
  // compact: the server stores and hashes exactly the bytes it receives
  const body = el.insertCompact.checked ? JSON.stringify(list) : el.insertJson.value.trim().startsWith("[") ? el.insertJson.value : `[${el.insertJson.value}]`;

  el.insertButton.disabled = true;
  setStatus(el.insertStatus, `Inserting ${plural(list.length, "certificate")}...`, "");
  try {
    await insertCertificates(body);
    if (el.insertRelease.checked) {
      setStatus(el.insertStatus, "Inserted, releasing...", "");
      await release();
      setStatus(el.insertStatus, `Inserted and released ${plural(list.length, "certificate")}.`, "ok");
    } else {
      setStatus(el.insertStatus, `Inserted ${plural(list.length, "certificate")}; queryable after the next release.`, "ok");
    }
    el.insertJson.value = "";
    await load();
  } catch (e) {
    // the server's validation message, e.g. which extension rule failed
    setStatus(el.insertStatus, e.message, "error");
  } finally {
    el.insertButton.disabled = false;
  }
}

async function onConnect() {
  const key = el.keyInput.value.trim();
  if (!key) {
    setStatus(el.authStatus, "Enter the insertion key", "error");
    return;
  }
  state.key = key;
  setStatus(el.authStatus, "Connecting...", "");
  try {
    state.entries = await fetchAllCertificates();
    sessionStorage.setItem(SESSION_STORAGE_KEY, key);
    setStatus(el.authStatus, "Connected", "ok");
    el.dashboard.hidden = false;
    render();
  } catch (e) {
    setStatus(el.authStatus, e.message, "error");
    el.dashboard.hidden = true;
  }
}

function onForgetKey() {
  sessionStorage.removeItem(SESSION_STORAGE_KEY);
  state.key = "";
  state.entries = [];
  el.keyInput.value = "";
  el.dashboard.hidden = true;
  setStatus(el.authStatus, "Key forgotten", "");
}

el.connectButton.addEventListener("click", onConnect);
el.forgetKeyButton.addEventListener("click", onForgetKey);
el.keyInput.addEventListener("keydown", (e) => {
  if (e.key === "Enter") onConnect();
});
el.refreshButton.addEventListener("click", load);
el.releaseButton.addEventListener("click", onRelease);
el.insertButton.addEventListener("click", onInsert);
el.insertToggle.addEventListener("click", () => {
  el.insertPanel.hidden = !el.insertPanel.hidden;
  el.insertToggle.setAttribute("aria-expanded", String(!el.insertPanel.hidden));
});
el.search.addEventListener("input", () => {
  state.search = el.search.value.trim().toLowerCase();
  render();
});

// reconnect with the key stored for this tab
if (state.key) {
  el.keyInput.value = state.key;
  onConnect();
}
