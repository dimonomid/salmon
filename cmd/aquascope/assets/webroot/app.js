const connectionStatus = document.getElementById("connection-status");
const statusSummary = document.getElementById("status-summary");
const serverSummary = document.getElementById("server-summary");
const serverDetailsToggle = document.getElementById("server-details-toggle");
const hosts = document.getElementById("hosts");
const incidents = document.getElementById("incidents");
const incidentSummary = document.getElementById("incident-summary");
let hostDetailsVisible = false;

function formatChangeTime(changeTime) {
  const date = new Date(changeTime);
  return Number.isNaN(date.getTime())
    ? changeTime
    : `${date.toLocaleString()} (${formatTimeAgo(changeTime)})`;
}

function formatTimeAgo(changeTime) {
  const date = new Date(changeTime);
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

function formatHostTime(value) {
  if (!value) {
    return "never";
  }
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function formatIncidentCount(count) {
  return `${count} ${count === 1 ? "incident" : "incidents"}`;
}

function renderHosts(items) {
  items = items || [];
  const online = items.filter((item) => item.connected).length;
  serverSummary.textContent = `${online}/${items.length} hosts are online`;
  if (online === items.length) {
    serverSummary.className = "status-connected";
  } else if (online === 0) {
    serverSummary.className = "status-disconnected";
  } else {
    serverSummary.className = "status-warning";
  }
  serverDetailsToggle.hidden = items.length === 0;
  serverDetailsToggle.textContent = hostDetailsVisible ? "Hide" : "Details";
  hosts.hidden = !hostDetailsVisible || items.length === 0;
  hosts.replaceChildren();

  const table = document.createElement("table");
  table.className = "host-status-table";
  const header = table.insertRow();
  for (const column of ["server", "status", "last status change", "last heartbeat"]) {
    const cell = document.createElement("th");
    cell.textContent = column;
    header.appendChild(cell);
  }
  for (const item of items) {
    const row = table.insertRow();
    for (const [columnIndex, value] of [
      item.id,
      item.connected ? "online" : "offline",
      formatHostTime(item.lastStatusChangeTime),
      formatHostTime(item.lastHeartbeatTime),
    ].entries()) {
      const cell = row.insertCell();
      cell.textContent = value;
      if (columnIndex === 1) {
        cell.className = item.connected ? "status-connected" : "status-disconnected";
      }
    }
  }
  hosts.appendChild(table);
  hosts.appendChild(document.createElement("hr"));
}

serverDetailsToggle.addEventListener("click", () => {
  hostDetailsVisible = !hostDetailsVisible;
  serverDetailsToggle.textContent = hostDetailsVisible ? "Hide" : "Details";
  hosts.hidden = !hostDetailsVisible;
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

let snoozedVisible = false;

function renderIncidentList(items, container, isSnoozed) {
  for (const item of items) {
    const table = document.createElement("table");
    table.className = isSnoozed ? "incident snoozed-incident" : "incident";

    const fields = [
      ["key", item.key],
      ["state", item.state],
      ["comment", item.comment],
      ["change time", formatChangeTime(item.changeTime)],
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

  incidentSummary.textContent = formatIncidentCount(alertingItems.length);
  if (snoozedItems.length > 0) {
    incidentSummary.appendChild(document.createTextNode(` + ${snoozedItems.length} snoozed`));
  }
  if (alertingItems.length > 0) {
    incidentSummary.className = "status-disconnected";
  } else if (snoozedItems.length > 0) {
    incidentSummary.className = "status-warning";
  } else {
    incidentSummary.className = "status-connected";
  }

  incidents.replaceChildren();

  if (alertingItems.length === 0) {
    incidents.appendChild(document.createTextNode("No ongoing incidents."));
  } else {
    renderIncidentList(alertingItems, incidents, false);
  }

  const snoozedSummary = document.createElement("div");
  snoozedSummary.className = "snoozed-summary";
  snoozedSummary.appendChild(document.createTextNode(`Snoozed: ${snoozedItems.length}`));

  if (snoozedItems.length > 0) {
    const toggle = document.createElement("button");
    toggle.type = "button";
    toggle.textContent = snoozedVisible ? "Hide" : "Show";
    toggle.addEventListener("click", () => {
      snoozedVisible = !snoozedVisible;
      render(alertingItems, snoozedItems);
    });
    snoozedSummary.appendChild(toggle);
  }
  incidents.appendChild(snoozedSummary);

  if (snoozedVisible && snoozedItems.length > 0) {
    const snoozedList = document.createElement("div");
    snoozedList.className = "snoozed-list";
    renderIncidentList(snoozedItems, snoozedList, true);
    incidents.appendChild(snoozedList);
  }
}

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
    connectionStatus.textContent = "● UI online";
    connectionStatus.className = "status-connected";
  };

  socket.onmessage = (event) => {
    const message = JSON.parse(event.data);
    renderHosts(message.hosts);
    render(message.ongoingIncidents.alerting, message.ongoingIncidents.snoozed);
    statusSummary.hidden = false;
  };

  socket.onclose = () => {
    connectionStatus.textContent = "× UI offline; reconnecting...";
    connectionStatus.className = "status-disconnected";
    serverSummary.replaceChildren();
    statusSummary.hidden = true;
    hosts.hidden = true;
    window.setTimeout(connect, 1000);
  };

  socket.onerror = () => socket.close();
}

connect();
