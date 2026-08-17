const connectionStatus = document.getElementById("connection-status");
const incidents = document.getElementById("incidents");

function formatChangeTime(changeTime) {
  const date = new Date(changeTime);
  return Number.isNaN(date.getTime()) ? changeTime : date.toLocaleString();
}

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

function render(items) {
  incidents.replaceChildren();

  if (items.length === 0) {
    incidents.textContent = "No ongoing incidents.";
    return;
  }

  for (const item of items) {
    const table = document.createElement("table");
    table.className = "incident";

    const fields = [
      ["key", item.key],
      ["state", item.state],
      ["comment", item.comment],
      ["change time", formatChangeTime(item.changeTime)],
    ];

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

    incidents.appendChild(table);
    incidents.appendChild(document.createElement("hr"));
  }
}

function connect() {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const socket = new WebSocket(`${protocol}//${window.location.host}/api/v1/wsconnect`);

  socket.onopen = () => {
    connectionStatus.textContent = "Connected";
  };

  socket.onmessage = (event) => {
    const message = JSON.parse(event.data);
    render(message.ongoingIncidents.total);
  };

  socket.onclose = () => {
    connectionStatus.textContent = "Disconnected; reconnecting…";
    window.setTimeout(connect, 1000);
  };

  socket.onerror = () => socket.close();
}

connect();
