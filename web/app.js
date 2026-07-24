const elements = {
  addForm: document.querySelector("#add-player-form"),
  formMessage: document.querySelector("#form-message"),
  gamesList: document.querySelector("#games-list"),
  healthStatus: document.querySelector("#health-status"),
  historyDays: document.querySelector("#history-days"),
  historyPeriod: document.querySelector("#history-period"),
  playersList: document.querySelector("#players-list"),
  playingPlayers: document.querySelector("#playing-players"),
  pollButton: document.querySelector("#poll-button"),
  sessionsList: document.querySelector("#sessions-list"),
  totalPlayers: document.querySelector("#total-players"),
};

function escapeHtml(value) {
  return String(value ?? "").replace(/[&<>"']/g, (character) => {
    const entities = {
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      '"': "&quot;",
      "'": "&#39;",
    };
    return entities[character];
  });
}

async function request(path, options = {}) {
  const response = await fetch(`/api/${path}`, options);

  if (response.status === 204) {
    return null;
  }

  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(body.error || response.statusText);
  }

  return body;
}

function formatDuration(seconds) {
  if (seconds < 60) {
    return `${seconds}s`;
  }
  if (seconds < 3600) {
    return `${Math.round(seconds / 60)}m`;
  }
  return `${(seconds / 3600).toFixed(1)}h`;
}

function formatTimestamp(value) {
  if (!value) {
    return "Not polled yet";
  }

  const date = typeof value === "number"
    ? new Date(value * 1000)
    : new Date(value);

  if (Number.isNaN(date.getTime())) {
    return "Not polled yet";
  }

  return date.toLocaleString();
}

function renderEmpty(container, message) {
  container.innerHTML = `<p class="empty-state">${escapeHtml(message)}</p>`;
}

function renderHealth(health) {
  if (health.last_error) {
    elements.healthStatus.className = "health-status health-status-error";
    elements.healthStatus.textContent = `Poll error: ${health.last_error}`;
    return;
  }

  elements.healthStatus.className = "health-status health-status-ok";
  elements.healthStatus.textContent = `Last poll: ${formatTimestamp(health.last_poll)}`;
}

function renderPlayers(players) {
  if (players.length === 0) {
    renderEmpty(elements.playersList, "No players are being monitored yet.");
    return;
  }

  elements.playersList.innerHTML = players.map((player) => {
    const displayName = player.name || player.steam_id;
    const state = player.game_id
      ? `Playing ${player.game || "a game"}`
      : player.state
        ? "Online"
        : "Offline";
    const stateClass = player.game_id ? "status-playing" : player.state ? "status-online" : "status-offline";
    const avatar = player.avatar
      ? `<img class="avatar" src="${escapeHtml(player.avatar)}" alt="">`
      : `<span class="avatar avatar-placeholder" aria-hidden="true"></span>`;

    return `
      <article class="player-row">
        ${avatar}
        <div class="player-details">
          <strong>${escapeHtml(displayName)}</strong>
          <span class="steam-id">${escapeHtml(player.steam_id)}</span>
          <span class="player-status ${stateClass}">${escapeHtml(state)}</span>
        </div>
        <button class="button button-danger remove-player" type="button" data-steam-id="${escapeHtml(player.steam_id)}">Remove</button>
      </article>
    `;
  }).join("");

  document.querySelectorAll(".remove-player").forEach((button) => {
    button.addEventListener("click", () => removePlayer(button.dataset.steamId));
  });
}

function renderGames(games) {
  if (games.length === 0) {
    renderEmpty(elements.gamesList, "Completed sessions will appear after a player exits a game.");
    return;
  }

  elements.gamesList.innerHTML = games.map((game) => `
    <div class="metric-row">
      <span>${escapeHtml(game.name || "Unknown game")}</span>
      <strong>${formatDuration(game.seconds)}</strong>
    </div>
  `).join("");
}

function renderSessions(sessions) {
  if (sessions.length === 0) {
    renderEmpty(elements.sessionsList, "No completed sessions in this period.");
    return;
  }

  elements.sessionsList.innerHTML = sessions.map((session) => `
    <article class="session-row">
      <div>
        <strong>${escapeHtml(session.player)}</strong>
        <span>${escapeHtml(session.game || "Unknown game")}</span>
        <small>Started ${formatTimestamp(session.started_at)}</small>
      </div>
      <strong>${formatDuration(session.seconds)}</strong>
    </article>
  `).join("");
}

function updateSummary(dashboard) {
  elements.totalPlayers.textContent = dashboard.total_players;
  elements.playingPlayers.textContent = dashboard.playing;
  elements.historyPeriod.textContent = `${elements.historyDays.value} days`;
}

function showFormMessage(message, type = "error") {
  elements.formMessage.hidden = false;
  elements.formMessage.className = `form-message form-message-${type}`;
  elements.formMessage.textContent = message;
}

function clearFormMessage() {
  elements.formMessage.hidden = true;
  elements.formMessage.textContent = "";
}

async function refreshDashboard() {
  try {
    const days = elements.historyDays.value;
    const [health, players, dashboard, sessions] = await Promise.all([
      request("health"),
      request("players"),
      request("dashboard?days=7"),
      request(`sessions?days=${days}`),
    ]);

    renderHealth(health);
    renderPlayers(players);
    renderGames(dashboard.top_games || []);
    renderSessions(sessions);
    updateSummary(dashboard);
  } catch (error) {
    elements.healthStatus.className = "health-status health-status-error";
    elements.healthStatus.textContent = `Unable to refresh: ${error.message}`;
  }
}

async function submitPlayers(event) {
  event.preventDefault();
  clearFormMessage();

  const formData = new FormData(elements.addForm);
  const payload = Object.fromEntries(formData.entries());
  const submitButton = elements.addForm.querySelector("button[type=submit]");
  submitButton.disabled = true;

  try {
    const result = await request("players", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });

    elements.addForm.reset();
    showFormMessage(`${result.steam_ids.length} player(s) added. Polling Steam now.`, "success");
    await request("poll", { method: "POST" });
    await refreshDashboard();
  } catch (error) {
    showFormMessage(error.message);
  } finally {
    submitButton.disabled = false;
  }
}

async function removePlayer(steamID) {
  if (!window.confirm("Remove this player from monitoring?")) {
    return;
  }

  try {
    await request(`players/${encodeURIComponent(steamID)}`, { method: "DELETE" });
    await refreshDashboard();
  } catch (error) {
    window.alert(error.message);
  }
}

async function pollNow() {
  elements.pollButton.disabled = true;
  elements.pollButton.textContent = "Polling...";

  try {
    await request("poll", { method: "POST" });
    await refreshDashboard();
  } catch (error) {
    window.alert(error.message);
  } finally {
    elements.pollButton.disabled = false;
    elements.pollButton.textContent = "Poll now";
  }
}

elements.addForm.addEventListener("submit", submitPlayers);
elements.historyDays.addEventListener("change", refreshDashboard);
elements.pollButton.addEventListener("click", pollNow);

refreshDashboard();
window.setInterval(refreshDashboard, 30000);
