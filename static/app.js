// Register service worker for PWA installability
if ("serviceWorker" in navigator) {
  navigator.serviceWorker.register("/sw.js").catch((err) => {
    console.error("SW registration failed:", err);
  });
}

// Dynamic favicon based on power state
function updateFavicon(isOn) {
  const color = isOn ? "#FFE46A" : "#1e1e2e";
  const glow = isOn ? "#FAAF63" : "#1a1a28";
  const base = isOn ? "#ABBDDB" : "#1a1a28";
  const baseDark = isOn ? "#6B83A5" : "#111118";
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">
    <rect width="512" height="512" rx="96" fill="#1a1a2e"/>
    <path d="M340 200c0-50-38-90-84-90s-84 40-84 90c0 23 8 44 22 60l19 34c5 9 7 15 15 15h56c8 0 11-6 15-15l20-34c14-16 21-37 21-60z" fill="${color}"/>
    <ellipse cx="256" cy="195" rx="18" ry="28" fill="${glow}" opacity="0.7"/>
    <rect x="222" y="316" width="68" height="14" rx="7" fill="${base}"/>
    <rect x="222" y="336" width="68" height="14" rx="7" fill="${baseDark}"/>
    <rect x="222" y="356" width="68" height="14" rx="7" fill="${base}"/>
    <path d="M246 376c0 0 0 20 10 20s10-20 10-20z" fill="${baseDark}"/>
  </svg>`;
  let link = document.querySelector("link[rel='icon'][type='image/svg+xml']");
  if (!link) {
    link = document.createElement("link");
    link.rel = "icon";
    link.type = "image/svg+xml";
    document.head.appendChild(link);
  }
  link.href = "data:image/svg+xml," + encodeURIComponent(svg);
}

// Parse the Go datetime format "HH:MM DD.MM.YYYY" into a JS Date
function parseDatetime(str) {
  if (!str) return null;
  // format: "HH:MM DD.MM.YYYY"
  const [time, date] = str.split(" ");
  const [hh, mm] = time.split(":");
  const [dd, mo, yyyy] = date.split(".");
  return new Date(+yyyy, +mo - 1, +dd, +hh, +mm);
}

function formatTime(date) {
  const mon = date.toLocaleString("en", { month: "short" });
  const day = date.getDate();
  const hh = String(date.getHours()).padStart(2, "0");
  const mm = String(date.getMinutes()).padStart(2, "0");
  return `${mon} ${day}, ${hh}:${mm}`;
}

function formatDuration(start, end) {
  const ms = end - start;
  const hrs = Math.floor(ms / 3600000);
  const mins = Math.floor((ms % 3600000) / 60000);
  if (hrs > 0) return `${hrs}h ${mins}m`;
  return `${mins}m`;
}

// DOM references
const body = document.body;
const statusBadge = document.getElementById("status-badge");
const statusDot = document.getElementById("status-dot");
const statusText = document.getElementById("status-text");
const ongoingDuration = document.getElementById("ongoing-duration");
const bulbWrap = document.querySelector(".bulb-wrap");
const bulbBody = document.getElementById("bulb-body");
const bulbFilament = document.getElementById("bulb-filament");
const bulbBaseLight1 = document.getElementById("bulb-base-light-1");
const bulbBaseDark = document.getElementById("bulb-base-dark");
const bulbBaseLight2 = document.getElementById("bulb-base-light-2");
const addressEl = document.getElementById("address");
const historyCard = document.getElementById("history-card");
const historyList = document.getElementById("history-list");

function render(data) {
  const isOn = data.grid === "on";
  const isPending = data.grid === "pending";

  // Update favicon to reflect power state
  if (!isPending) updateFavicon(isOn);

  // Body background
  body.className = isPending
    ? "power-pending"
    : isOn
      ? "power-on"
      : "power-off";

  // Status badge
  const stateClass = isPending ? "pending" : isOn ? "on" : "off";
  statusBadge.className = "status-badge " + stateClass;
  statusDot.className = "status-dot " + stateClass;
  statusText.textContent = isPending
    ? "Loading..."
    : isOn
      ? "Power On"
      : "Power Outage";

  // Address
  addressEl.textContent = data.address;

  // Outage expected end
  if (data.outage && data.outage.to) {
    const expectedEnd = parseDatetime(data.outage.to);
    ongoingDuration.textContent = "Expected to end at " + formatTime(expectedEnd);
    ongoingDuration.style.display = "block";
  } else if (data.outage) {
    ongoingDuration.textContent = "Ongoing";
    ongoingDuration.style.display = "block";
  } else {
    ongoingDuration.style.display = "none";
  }

  // Lightbulb glow
  bulbWrap.className = "bulb-wrap" + (isOn ? " on" : "");

  // Lightbulb colors
  if (isOn) {
    bulbBody.setAttribute("fill", "#FFE46A");
    bulbBody.classList.add("bulb-body-on");
    bulbFilament.setAttribute("fill", "#FAAF63");
    bulbFilament.classList.add("filament-on");
    bulbBaseLight1.setAttribute("fill", "#ABBDDB");
    bulbBaseDark.setAttribute("fill", "#6B83A5");
    bulbBaseLight2.setAttribute("fill", "#ABBDDB");
  } else {
    bulbBody.setAttribute("fill", "#1e1e2e");
    bulbBody.classList.remove("bulb-body-on");
    bulbFilament.setAttribute("fill", "#1a1a28");
    bulbFilament.classList.remove("filament-on");
    bulbBaseLight1.setAttribute("fill", "#1a1a28");
    bulbBaseDark.setAttribute("fill", "#111118");
    bulbBaseLight2.setAttribute("fill", "#1a1a28");
  }

  // History card
  historyCard.className = "history-card " + stateClass;

  // History list
  historyList.innerHTML = "";
  const items = [...data.history].reverse();
  for (const item of items) {
    const isOff = item.state === "off";
    const ongoing = !item.to;
    const from = parseDatetime(item.from);
    const to = item.to ? parseDatetime(item.to) : null;

    const row = document.createElement("div");
    row.className = "history-row" + (isOff && ongoing ? " ongoing-off" : "");

    // Dot
    const dot = document.createElement("span");
    const dotState = isOff ? "off" : "on";
    dot.className = "history-dot " + dotState + (ongoing ? " ongoing" : "");
    row.appendChild(dot);

    // Times
    const times = document.createElement("span");
    times.className = "history-times";
    if (to) {
      times.innerHTML = formatTime(from) + " &rarr; " + formatTime(to);
    } else {
      const nowLabel = document.createElement("span");
      nowLabel.className = "now-label " + dotState;
      nowLabel.textContent = "now";
      times.textContent = formatTime(from) + " ";
      times.appendChild(nowLabel);
    }
    row.appendChild(times);

    // Duration
    if (to) {
      const dur = document.createElement("span");
      dur.className = "history-duration";
      dur.textContent = formatDuration(from, to);
      row.appendChild(dur);
    }

    historyList.appendChild(row);
  }
}

async function fetchState() {
  try {
    const res = await fetch("/api/state");
    const data = await res.json();
    render(data);
  } catch (err) {
    console.error("Failed to fetch power state:", err);
  }
}

// Render immediately from server-embedded data, then poll for updates
render(INITIAL_STATE);
setInterval(fetchState, 60000);
