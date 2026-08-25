const connectionStatus = document.getElementById("connection-status");
const themeToggle = document.getElementById("theme-toggle");
const statusSummary = document.getElementById("status-summary");
const serverSummary = document.getElementById("server-summary");
const servers = document.getElementById("servers");
const incidents = document.getElementById("incidents");
const incidentSummary = document.getElementById("incident-summary");
const snoozedSummary = document.getElementById("snoozed-summary");
const snoozedIncidents = document.getElementById("snoozed-incidents");
const serverDetailsVisibilityKey = "salmon-watch.serverDetailsVisible";
const incidentDetailsVisibilityKey = "salmon-watch.incidentDetailsVisible";
const snoozedVisibilityKey = "salmon-watch.snoozedVisible";
const themeKey = "salmon-watch.theme";

function themePreference() {
  try {
    return localStorage.getItem(themeKey) === "light" ? "light" : "dark";
  } catch (_) {
    return "dark";
  }
}

function applyTheme(theme) {
  document.documentElement.dataset.theme = theme === "light" ? "light" : "dark";
  const nextTheme = theme === "dark" ? "light" : "dark";
  themeToggle.textContent = nextTheme === "light" ? "☀" : "☾";
  themeToggle.setAttribute("aria-label", `Switch to ${nextTheme} scheme`);
  themeToggle.title = `Switch to ${nextTheme} scheme`;
}

let theme = themePreference();
applyTheme(theme);

themeToggle.addEventListener("click", () => {
  theme = theme === "dark" ? "light" : "dark";
  applyTheme(theme);
  try {
    localStorage.setItem(themeKey, theme);
  } catch (_) {
    // Keep the current-page setting working when browser storage is unavailable.
  }
});

function visibilityPreference(key, defaultVisible = false) {
  try {
    const value = localStorage.getItem(key);
    return value === null ? defaultVisible : value === "true";
  } catch (_) {
    return defaultVisible;
  }
}

function saveVisibilityPreference(key, visible) {
  try {
    localStorage.setItem(key, String(visible));
  } catch (_) {
    // Keep the current-page setting working when browser storage is unavailable.
  }
}

let serverDetailsVisible = visibilityPreference(serverDetailsVisibilityKey, true);
let incidentDetailsVisible = visibilityPreference(incidentDetailsVisibilityKey, true);

function setSectionSummary(summary, text, visible) {
  summary.replaceChildren(document.createTextNode(`▪ ${text} ${visible ? "▼" : "▲"}`));
}

function formatIncidentStartedAt(incidentStartedAt) {
  const date = new Date(incidentStartedAt);
  return Number.isNaN(date.getTime())
    ? incidentStartedAt
    : `${date.toLocaleString()} (${formatTimeAgo(incidentStartedAt)})`;
}

function formatTimeAgo(incidentStartedAt) {
  const date = new Date(incidentStartedAt);
  if (Number.isNaN(date.getTime())) {
    return "";
  }

  const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
  if (seconds < 0) {
    return "in the future";
  }
  if (seconds < 60) {
    return "just now";
  }
  if (seconds < 3600) {
    return `${Math.floor(seconds / 60)}m ago`;
  }
  if (seconds < 86400) {
    return `${Math.floor(seconds / 3600)}h ago`;
  }
  return `${Math.floor(seconds / 86400)}d ago`;
}

function formatTimeUntil(snoozedUntil) {
  const date = new Date(snoozedUntil);
  if (Number.isNaN(date.getTime())) {
    return "";
  }

  const seconds = Math.floor((date.getTime() - Date.now()) / 1000);
  if (seconds <= 0) {
    return "now";
  }
  if (seconds < 60) {
    return `in ${seconds} sec${seconds === 1 ? "" : "s"}`;
  }
  if (seconds < 3600) {
    const minutes = Math.floor(seconds / 60);
    return `in ${minutes} min${minutes === 1 ? "" : "s"}`;
  }
  if (seconds < 86400) {
    const hours = Math.floor(seconds / 3600);
    return `in ${hours} hour${hours === 1 ? "" : "s"}`;
  }
  const days = Math.floor(seconds / 86400);
  return `in ${days} day${days === 1 ? "" : "s"}`;
}

function formatServerTime(value) {
  if (!value) {
    return "never";
  }
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function formatIncidentCount(count) {
  return `${count} ${count === 1 ? "incident" : "incidents"}`;
}

function formatSnoozedIncidentCount(count) {
  return `${count} snoozed ${count === 1 ? "incident" : "incidents"}`;
}

function renderServers(items) {
  items = items || [];
  lastServerItems = items;
  const online = items.filter((item) => item.connected).length;
  setSectionSummary(serverSummary, `${online}/${items.length} servers are online`, serverDetailsVisible);
  if (online === items.length) {
    serverSummary.className = "section-summary status-connected";
  } else if (online === 0) {
    serverSummary.className = "section-summary status-disconnected";
  } else {
    serverSummary.className = "section-summary status-warning";
  }
  serverSummary.hidden = items.length === 0;
  servers.hidden = !serverDetailsVisible || items.length === 0;
  servers.replaceChildren();

  const table = document.createElement("table");
  table.className = "server-status-table";
  const header = table.insertRow();
  for (const column of ["server", "connection", "last heartbeat"]) {
    const cell = document.createElement("th");
    cell.textContent = column;
    header.appendChild(cell);
  }
  for (const item of items) {
    const row = table.insertRow();
    for (const [columnIndex, value] of [
      item.id,
      item.connected ? "online" : "offline",
      formatServerTime(item.lastHeartbeatTime),
    ].entries()) {
      const cell = row.insertCell();
      cell.textContent = value;
      if (columnIndex === 1) {
        cell.className = item.connected ? "status-connected" : "status-disconnected";
      }
    }
  }
  servers.appendChild(table);
  servers.appendChild(document.createElement("hr"));
}

serverSummary.addEventListener("click", () => {
  serverDetailsVisible = !serverDetailsVisible;
  saveVisibilityPreference(serverDetailsVisibilityKey, serverDetailsVisible);
  renderServers(lastServerItems);
});

function iconURL(item) {
  if (item.state === "ok") {
    return "/icons/salmon_green.png";
  }

  if (item.key.startsWith("internal.")) {
    return "/icons/salmon_magenta.png";
  }

  if (item.state === "warning") {
    return "/icons/salmon_yellow.png";
  }

  return "/icons/salmon_red.png";
}

let snoozedVisible = visibilityPreference(snoozedVisibilityKey);
let lastServerItems = [];
let lastAlertingItems = [];
let lastSnoozedItems = [];

function renderIncidentList(items, container, isSnoozed) {
  for (const item of items) {
    const table = document.createElement("table");
    table.className = isSnoozed ? "incident snoozed-incident" : "incident";

    const fields = [
      ["key", item.key],
      ["state", item.state],
      ["details", item.details],
      ["started at", formatIncidentStartedAt(item.incidentStartedAt)],
    ];
    if (isSnoozed) {
      fields.push(["snoozed until", `${new Date(item.snoozedUntil).toLocaleString()} (${formatTimeUntil(item.snoozedUntil)})`]);
    }

    for (const [fieldIndex, [label, value]] of fields.entries()) {
      const row = table.insertRow();

      if (fieldIndex === 0) {
        const iconCell = row.insertCell();
        iconCell.className = "incident-icon";
        iconCell.rowSpan = fields.length;

        const icon = document.createElement("img");
        icon.src = iconURL(item);
        icon.alt = item.state;
        iconCell.appendChild(icon);
      }

      const labelCell = document.createElement("th");
      labelCell.textContent = `${label}:`;
      labelCell.className = "incident-label";
      row.appendChild(labelCell);

      const valueCell = row.insertCell();
      valueCell.textContent = value;
    }

    container.appendChild(table);

    const controls = document.createElement("div");
    controls.className = "snooze-controls";
    if (isSnoozed) {
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = "Unsnooze";
      button.addEventListener("click", () => unsnoozeIncident(item.key));
      controls.appendChild(button);
    } else {
      controls.appendChild(document.createTextNode("Snooze for: "));
      for (const duration of ["30m", "1h", "4h", "6h", "12h", "1d", "7d", "forever"]) {
        const button = document.createElement("button");
        button.type = "button";
        button.textContent = duration;
        button.addEventListener("click", () => snoozeIncident(item.key, duration));
        controls.appendChild(button);
      }
    }
    container.appendChild(controls);
    container.appendChild(document.createElement("hr"));
  }
}

function render(alertingItems, snoozedItems) {
  alertingItems = alertingItems || [];
  snoozedItems = snoozedItems || [];
  lastAlertingItems = alertingItems;
  lastSnoozedItems = snoozedItems;

  setSectionSummary(incidentSummary, formatIncidentCount(alertingItems.length), incidentDetailsVisible);
  if (alertingItems.length > 0) {
    incidentSummary.className = "section-summary status-disconnected";
  } else {
    incidentSummary.className = "section-summary status-connected";
  }

  setSectionSummary(snoozedSummary, formatSnoozedIncidentCount(snoozedItems.length), snoozedVisible);
  snoozedSummary.className = snoozedItems.length > 0
    ? "section-summary status-warning"
    : "section-summary status-connected";

  incidents.replaceChildren();

  if (alertingItems.length === 0) {
    const emptyMessage = document.createElement("div");
    emptyMessage.className = "incident-empty";
    emptyMessage.textContent = "No ongoing incidents.";
    incidents.appendChild(emptyMessage);
  } else {
    renderIncidentList(alertingItems, incidents, false);
  }

  snoozedIncidents.replaceChildren();
  snoozedIncidents.className = "";
  if (snoozedItems.length === 0) {
    const emptyMessage = document.createElement("div");
    emptyMessage.className = "incident-empty";
    emptyMessage.textContent = "No snoozed incidents.";
    snoozedIncidents.appendChild(emptyMessage);
  } else {
    snoozedIncidents.className = "snoozed-list";
    renderIncidentList(snoozedItems, snoozedIncidents, true);
  }

  incidents.hidden = !incidentDetailsVisible;
  snoozedIncidents.hidden = !snoozedVisible;
}

incidentSummary.addEventListener("click", () => {
  incidentDetailsVisible = !incidentDetailsVisible;
  saveVisibilityPreference(incidentDetailsVisibilityKey, incidentDetailsVisible);
  render(lastAlertingItems, lastSnoozedItems);
});

snoozedSummary.addEventListener("click", () => {
  snoozedVisible = !snoozedVisible;
  saveVisibilityPreference(snoozedVisibilityKey, snoozedVisible);
  render(lastAlertingItems, lastSnoozedItems);
});

async function snoozeIncident(key, duration) {
  const response = await fetch("/api/v1/snooze", {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({key, duration}),
  });

  if (!response.ok) {
    connectionStatus.textContent = "Failed to snooze incident";
  }
}

async function unsnoozeIncident(key) {
  const response = await fetch("/api/v1/unsnooze", {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({key}),
  });

  if (!response.ok) {
    connectionStatus.textContent = "Failed to unsnooze incident";
  }
}

function connect() {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const socket = new WebSocket(`${protocol}//${window.location.host}/api/v1/wsconnect`);

  socket.onopen = () => {
    connectionStatus.textContent = "▪ UI online";
    connectionStatus.className = "status-connected";
  };

  socket.onmessage = (event) => {
    const message = JSON.parse(event.data);
    renderServers(message.servers);
    render(message.ongoingIncidents.alerting, message.ongoingIncidents.snoozed);
    statusSummary.hidden = false;
  };

  socket.onclose = () => {
    connectionStatus.textContent = "× UI offline; reconnecting...";
    connectionStatus.className = "status-disconnected";
    serverSummary.replaceChildren();
    statusSummary.hidden = true;
    servers.hidden = true;
    window.setTimeout(connect, 1000);
  };

  socket.onerror = () => socket.close();
}

connect();
