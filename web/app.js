const state = {
  filter: "active",
  sort: "dailyCost",
  search: "",
  assets: [],
  summary: null,
  analytics: null,
  editingId: null,
  iconTarget: null,
};

const emojiOptions = ["📦", "💻", "📱", "⌚", "🎧", "📷", "🎮", "🖥️", "💾", "🧰", "📚", "🪑", "🛋️", "⌨️", "🖨️", "🚲", "🎒", "🔋", "🎸", "☕"];

const statusMeta = {
  all: { label: "全部", color: "#111111" },
  active: { label: "服役中", color: "#b4eb4e" },
  idle: { label: "已闲置", color: "#c8b09e" },
  sold: { label: "已售出", color: "#7c7c80" },
};

const sortOptions = {
  dailyCost: "日使用成本",
  price: "总价",
  daysUsed: "使用时长",
};

const currency = new Intl.NumberFormat("zh-CN", {
  style: "currency",
  currency: "CNY",
  minimumFractionDigits: 2,
});

const grid = document.querySelector("#assetGrid");
const filters = document.querySelector("#filters");
const sortSwitch = document.querySelector("#sortSwitch");
const sortMenuBtn = document.querySelector("#sortMenuBtn");
const searchToggleBtn = document.querySelector("#searchToggleBtn");
const searchBox = document.querySelector("#searchBox");
const searchInput = document.querySelector("#searchInput");
const formPanel = document.querySelector("#formPanel");
const modalBackdrop = document.querySelector("#modalBackdrop");
const form = document.querySelector("#assetForm");
const formHint = document.querySelector("#formHint");
const toggleFormBtn = document.querySelector("#toggleFormBtn");
const formIconTrigger = document.querySelector("#formIconTrigger");
const deleteBtn = document.querySelector("#deleteBtn");
const cancelEditBtn = document.querySelector("#cancelEditBtn");
const emojiPicker = document.querySelector("#emojiPicker");
const emojiGrid = document.querySelector("#emojiGrid");
const emojiInput = document.querySelector("#emojiInput");
const applyEmojiBtn = document.querySelector("#applyEmojiBtn");
const closeEmojiPickerBtn = document.querySelector("#closeEmojiPickerBtn");
const panelTitle = document.querySelector("#panelTitle");
const submitBtn = document.querySelector("#submitBtn");
const statusField = document.querySelector("#statusField");
const purchaseDateField = form.elements.purchaseDate;
const soldPriceField = document.querySelector("#soldPriceField");
const soldDateField = document.querySelector("#soldDateField");

toggleFormBtn.addEventListener("click", () => {
  openCreateForm();
});

sortMenuBtn.addEventListener("click", () => {
  const nextHidden = !sortSwitch.hidden;
  sortSwitch.hidden = nextHidden;
  sortMenuBtn.classList.toggle("active", !nextHidden);
});

searchToggleBtn.addEventListener("click", () => {
  const nextHidden = !searchBox.hidden;
  searchBox.hidden = nextHidden;
  searchToggleBtn.classList.toggle("active", !nextHidden);
  if (!nextHidden) {
    requestAnimationFrame(() => searchInput.focus());
    return;
  }
  if (state.search) {
    state.search = "";
    searchInput.value = "";
    renderAssets();
  }
});

searchInput.addEventListener("input", () => {
  state.search = searchInput.value.trim().toLowerCase();
  renderAssets();
});

cancelEditBtn.addEventListener("click", () => {
  resetFormState();
});

modalBackdrop.addEventListener("click", () => {
  closeSortMenu();
  closeEmojiPicker();
  resetFormState();
});

document.addEventListener("click", (event) => {
  if (sortSwitch.hidden) {
    return;
  }
  if (sortSwitch.contains(event.target) || sortMenuBtn.contains(event.target)) {
    return;
  }
  closeSortMenu();
});

document.addEventListener("keydown", (event) => {
  if (event.key === "Escape" && formPanel.classList.contains("open")) {
    closeSortMenu();
    closeEmojiPicker();
    resetFormState();
    return;
  }
  if (event.key === "Escape") {
    closeSortMenu();
  }
});

formIconTrigger.addEventListener("click", () => {
  openEmojiPicker({ type: "form" });
});

closeEmojiPickerBtn.addEventListener("click", () => {
  closeEmojiPicker();
});

applyEmojiBtn.addEventListener("click", async () => {
  await applyCustomEmoji();
});

emojiInput.addEventListener("keydown", async (event) => {
  if (event.key !== "Enter") {
    return;
  }
  event.preventDefault();
  await applyCustomEmoji();
});

[purchaseDateField, soldDateField].forEach((field) => {
  field.addEventListener("change", () => normalizeDateField(field));
  field.addEventListener("blur", () => normalizeDateField(field));
});

deleteBtn.addEventListener("click", async () => {
  if (!state.editingId) {
    return;
  }
  if (!window.confirm("删除后不可恢复，确定删除这条资产吗？")) {
    return;
  }

  try {
    const response = await fetch(`/api/assets/${state.editingId}`, {
      method: "DELETE",
    });
    if (!response.ok) {
      const data = await response.json();
      throw new Error(data.error || "删除失败");
    }
    resetFormState();
    await loadDashboard();
  } catch (error) {
    formHint.textContent = error.message;
  }
});

statusField.addEventListener("change", syncSoldFields);

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  formHint.textContent = state.editingId ? "更新中..." : "保存中...";

  const payload = readFormPayload();
  if (!payload) {
    return;
  }

  const editing = Boolean(state.editingId);
  const url = editing ? `/api/assets/${state.editingId}` : "/api/assets";
  const method = editing ? "PUT" : "POST";

  try {
    const response = await fetch(url, {
      method,
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });

    if (!response.ok) {
      const data = await response.json();
      throw new Error(data.error || "保存失败");
    }

    resetFormState();
    formHint.textContent = editing ? "已更新。" : "已保存。";
    await loadDashboard();
  } catch (error) {
    formHint.textContent = error.message;
  }
});

function readFormPayload() {
  const formData = new FormData(form);
  const payload = {
    name: String(formData.get("name") || "").trim(),
    icon: String(formData.get("icon") || "📦"),
    category: String(formData.get("category") || "").trim(),
    price: Number(formData.get("price")),
    purchaseDate: String(formData.get("purchaseDate") || ""),
    status: String(formData.get("status") || "active"),
  };

  const soldPrice = Number(formData.get("soldPrice"));
  const soldDate = String(formData.get("soldDate") || "");

  if (!isValidDateInput(payload.purchaseDate)) {
    formHint.textContent = "购入日期必须是有效的 YYYY-MM-DD 日期。";
    return null;
  }

  if (payload.status === "sold") {
    if (!(soldPrice >= 0) || !soldDate) {
      formHint.textContent = "已售出资产必须填写售出价格和售出日期。";
      return null;
    }
    if (!isValidDateInput(soldDate)) {
      formHint.textContent = "售出日期必须是有效的 YYYY-MM-DD 日期。";
      return null;
    }
    payload.soldPrice = soldPrice;
    payload.soldDate = soldDate;
  }

  return payload;
}

function syncSoldFields() {
  const isSold = statusField.value === "sold";
  soldPriceField.toggleAttribute("required", isSold);
  soldDateField.toggleAttribute("required", isSold);
  soldPriceField.disabled = !isSold;
  soldDateField.disabled = !isSold;
}

function renderFilters() {
  filters.innerHTML = "";
  ["active", "idle", "sold"].forEach((key) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = `filter-chip${state.filter === key ? " active" : ""}`;
    button.textContent = statusMeta[key].label;
    button.addEventListener("click", () => {
      state.filter = key;
      renderFilters();
      renderAssets();
    });
    filters.appendChild(button);
  });
}

function renderSortSwitch() {
  sortSwitch.innerHTML = "";
  Object.entries(sortOptions).forEach(([key, label]) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = `sort-chip${state.sort === key ? " active" : ""}`;
    button.textContent = label;
    button.addEventListener("click", () => {
      state.sort = key;
      renderSortSwitch();
      renderAssets();
      closeSortMenu();
    });
    sortSwitch.appendChild(button);
  });
}

function closeSortMenu() {
  sortSwitch.hidden = true;
  sortMenuBtn.classList.remove("active");
}

function renderSummary() {
  const summary = state.summary;
  if (!summary) {
    return;
  }

  document.querySelector("#totalAssetValue").textContent = currency.format(summary.totalAssetValue);
  document.querySelector("#averageDailyValue").textContent = `${currency.format(summary.averageDaily)}/天`;

  const totalCount = Math.max(state.assets.length, 1);
  const barContainer = document.querySelector("#statusBars");
  barContainer.innerHTML = "";

  ["active", "idle", "sold"].forEach((key) => {
    const count = summary.counts[key] || 0;
    const ratio = Math.round((count / totalCount) * 100);
    const wrapper = document.createElement("div");
    wrapper.className = "status-card";
    wrapper.innerHTML = `
      <div class="status-line">
        <p class="status-name">${statusMeta[key].label}</p>
        <p class="status-count">${count}</p>
      </div>
      <div class="mini-track">
        <div class="mini-fill" style="width:${ratio}%; background:${statusMeta[key].color};"></div>
      </div>
    `;
    barContainer.appendChild(wrapper);
  });
}

function renderAssets() {
  const template = document.querySelector("#assetCardTemplate");
  const filtered = state.filter === "all" ? state.assets : state.assets.filter((asset) => asset.status === state.filter);
  const searched = state.search
    ? filtered.filter((asset) => {
        const haystack = `${asset.name} ${asset.category} ${asset.icon || ""}`.toLowerCase();
        return haystack.includes(state.search);
      })
    : filtered;
  const items = [...searched].sort(compareAssets);
  grid.innerHTML = "";

  if (items.length === 0) {
    const empty = document.createElement("div");
    empty.className = "empty-state";
    empty.textContent = "当前筛选条件下没有资产。";
    grid.appendChild(empty);
    return;
  }

  items.forEach((asset) => {
    const node = template.content.firstElementChild.cloneNode(true);
    node.title = "点击编辑资产";
    const icon = node.querySelector(".asset-icon");
    icon.textContent = asset.icon || "📦";
    icon.addEventListener("click", (event) => {
      event.preventDefault();
      event.stopPropagation();
    });
    icon.addEventListener("dblclick", (event) => {
      event.preventDefault();
      event.stopPropagation();
      openEmojiPicker({ type: "asset", assetId: asset.id });
    });
    node.querySelector(".asset-category").textContent = asset.category;
    node.querySelector(".asset-name").textContent = asset.name;
    node.querySelector(".asset-meta").textContent = `${currency.format(asset.price)} | 已使用 ${asset.daysUsed} 天`;
    node.querySelector(".asset-daily").textContent = `${currency.format(asset.dailyCost)}/天`;

    const soldLine = node.querySelector(".asset-sold");
    soldLine.textContent =
      asset.status === "sold" && asset.soldPrice && asset.soldDate
        ? `售出 ${currency.format(asset.soldPrice)} · ${asset.soldDate} · 回收 ${asset.recoveredRatio}%`
        : "";

    const chip = node.querySelector(".status-chip");
    chip.textContent = statusMeta[asset.status].label;
    chip.style.setProperty("--chip-color", statusMeta[asset.status].color);
    chip.disabled = true;

    node.addEventListener("click", () => openEditForm(asset.id));
    grid.appendChild(node);
  });
}

function compareAssets(left, right) {
  switch (state.sort) {
    case "price":
      return right.price - left.price || right.daysUsed - left.daysUsed;
    case "daysUsed":
      return right.daysUsed - left.daysUsed || right.price - left.price;
    case "dailyCost":
    default:
      return right.dailyCost - left.dailyCost || right.price - left.price;
  }
}

function renderCharts() {
  renderTrendCharts();
  renderCategoryBars();
}

function renderTrendCharts() {
  const assetValuePoints = buildSnapshotSeries((items) => items.reduce((sum, item) => sum + item.price, 0));

  document.querySelector("#assetValueTrendCurrent").textContent = currency.format(state.summary?.totalAssetValue || 0);
  renderTrendChart("#assetValueTrendChart", assetValuePoints, { mode: "step", valueLabel: "今天" });
}

function buildSnapshotSeries(valueBuilder) {
  if (!state.assets.length) {
    return [];
  }

  const sortedAssets = [...state.assets].sort((left, right) => left.purchaseDate.localeCompare(right.purchaseDate));
  const end = monthStart(new Date().toISOString().slice(0, 10));
  const start = new Date(end.getFullYear(), end.getMonth() - 11, 1);
  const points = [];

  for (let cursor = new Date(start); cursor <= end; cursor = nextMonth(cursor)) {
    const snapshotDate = monthEnd(cursor);
    const snapshotLabel = formatMonthLabel(cursor);
    const items = sortedAssets.filter((asset) => asset.purchaseDate <= snapshotDate);
    points.push({
      label: snapshotLabel,
      value: Number(valueBuilder(items, snapshotDate).toFixed(2)),
    });
  }

  return points;
}

function renderTrendChart(selector, points, options = {}) {
  const container = document.querySelector(selector);
  container.innerHTML = "";

  if (!points.length) {
    container.innerHTML = `<div class="empty-state">暂无趋势数据。</div>`;
    return;
  }

  const width = 860;
  const height = 320;
  const paddingTop = 18;
  const paddingBottom = 42;
  const paddingLeft = 16;
  const paddingRight = 82;
  const maxValue = Math.max(...points.map((item) => item.value), 1);
  const minValue = 0;
  const stepX = points.length > 1 ? (width - paddingLeft - paddingRight) / (points.length - 1) : 0;
  const usableHeight = height - paddingTop - paddingBottom;

  const coordinates = points.map((point, index) => {
    const x = paddingLeft + stepX * index;
    const y = paddingTop + (1 - (point.value - minValue) / (maxValue - minValue || 1)) * usableHeight;
    return { ...point, x, y };
  });

  const path = options.mode === "step" ? toStepPath(coordinates) : toLinePath(coordinates);
  const last = coordinates[coordinates.length - 1];
  const first = coordinates[0];
  const areaPath = `${path} L${last.x} ${height - paddingBottom} L${first.x} ${height - paddingBottom} Z`;
  const gridStroke = "rgba(20,20,20,0.08)";
  const tickText = "rgba(20,20,20,0.36)";
  const axisStroke = "rgba(143,202,34,0.9)";
  const tickValues = [0, 0.25, 0.5, 0.75, 1].map((ratio) => Number((maxValue * ratio).toFixed(0)));
  const tickLines = tickValues
    .map((value) => {
      const y = paddingTop + (1 - value / (maxValue || 1)) * usableHeight;
      return `<g>
        <line x1="${paddingLeft}" y1="${y}" x2="${width - paddingRight}" y2="${y}" stroke="${gridStroke}" stroke-dasharray="4 10"></line>
        <text x="${width - paddingRight + 18}" y="${y + 6}" fill="${tickText}" font-size="13">${formatCompactNumber(value)}</text>
      </g>`;
    })
    .join("");

  const labelPositions = [0, Math.floor((coordinates.length - 1) / 2), coordinates.length - 1]
    .filter((value, index, arr) => arr.indexOf(value) === index)
    .map((index) => coordinates[index]);
  const xLabels = labelPositions
    .map((point) => `<text x="${point.x}" y="${height - 8}" text-anchor="middle" fill="${tickText}" font-size="13">${point.label}</text>`)
    .join("");

  container.innerHTML = `
    <svg viewBox="0 0 ${width} ${height}" role="img" aria-label="趋势图">
      <defs>
        <linearGradient id="${selector.slice(1)}Fill" x1="0" x2="0" y1="0" y2="1">
          <stop offset="0%" stop-color="#8fe51d" stop-opacity="0.42"></stop>
          <stop offset="100%" stop-color="#8fe51d" stop-opacity="0.04"></stop>
        </linearGradient>
      </defs>
      ${tickLines}
      <path d="${areaPath}" fill="url(#${selector.slice(1)}Fill)"></path>
      <path d="${path}" fill="none" stroke="#8fe51d" stroke-width="6" stroke-linecap="round" stroke-linejoin="round"></path>
      <circle cx="${last.x}" cy="${last.y}" r="9" fill="#8fe51d"></circle>
      <circle cx="${last.x}" cy="${last.y}" r="5" fill="#ffffff"></circle>
      <line x1="${width - paddingRight}" y1="${paddingTop}" x2="${width - paddingRight}" y2="${height - paddingBottom}" stroke="${axisStroke}" stroke-width="1"></line>
      ${xLabels}
    </svg>
  `;
}

function toLinePath(points) {
  return points.map((point, index) => `${index === 0 ? "M" : "L"}${point.x} ${point.y}`).join(" ");
}

function toStepPath(points) {
  if (!points.length) {
    return "";
  }
  let path = `M${points[0].x} ${points[0].y}`;
  for (let index = 1; index < points.length; index += 1) {
    path += ` L${points[index].x} ${points[index - 1].y} L${points[index].x} ${points[index].y}`;
  }
  return path;
}

function formatCompactNumber(value) {
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(value);
}

function normalizeDateField(field) {
  const value = String(field.value || "");
  if (!value) {
    return;
  }
  if (!isValidDateInput(value)) {
    field.value = "";
  }
}

function isValidDateInput(value) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    return false;
  }
  const [year, month, day] = value.split("-").map(Number);
  if (year < 1900 || year > 9999) {
    return false;
  }
  const date = new Date(`${value}T00:00:00`);
  if (Number.isNaN(date.getTime())) {
    return false;
  }
  return date.getFullYear() === year && date.getMonth() + 1 === month && date.getDate() === day;
}

function calculateDailyCostAtDate(asset, snapshotDate) {
  const purchaseDate = new Date(`${asset.purchaseDate}T00:00:00`);
  const targetDate = new Date(`${snapshotDate}T00:00:00`);
  const diffDays = Math.max(1, Math.floor((targetDate - purchaseDate) / 86400000) + 1);
  return asset.price / diffDays;
}

function monthStart(dateString) {
  const [year, month] = dateString.slice(0, 7).split("-").map(Number);
  return new Date(year, month - 1, 1);
}

function monthEnd(date) {
  const year = date.getFullYear();
  const month = date.getMonth();
  return new Date(year, month + 1, 0).toISOString().slice(0, 10);
}

function nextMonth(date) {
  return new Date(date.getFullYear(), date.getMonth() + 1, 1);
}

function formatMonthLabel(date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  return `${year}-${month}`;
}

function renderCategoryBars() {
  const container = document.querySelector("#categoryBars");
  const categories = state.analytics?.categories || [];
  const maxValue = Math.max(...categories.map((item) => item.totalValue), 1);
  container.innerHTML = "";

  categories.forEach((item) => {
    const ratio = (item.totalValue / maxValue) * 100;
    const row = document.createElement("div");
    row.className = "bar-row";
    row.innerHTML = `
      <div class="bar-head">
        <strong>${item.name}</strong>
        <span>${currency.format(item.totalValue)}</span>
      </div>
      <div class="bar-track">
        <div class="bar-fill" style="width:${ratio}%"></div>
      </div>
    `;
    container.appendChild(row);
  });
}

function openCreateForm() {
  state.editingId = null;
  form.reset();
  formPanel.classList.add("open");
  modalBackdrop.hidden = false;
  panelTitle.textContent = "新增资产";
  submitBtn.textContent = "保存资产";
  deleteBtn.hidden = true;
  cancelEditBtn.hidden = true;
  formHint.textContent = "";
  form.elements.icon.value = "📦";
  syncFormIcon("📦");
  statusField.value = "active";
  syncSoldFields();
}

function openEditForm(assetId) {
  const asset = state.assets.find((item) => item.id === assetId);
  if (!asset) {
    return;
  }

  state.editingId = asset.id;
  formPanel.classList.add("open");
  modalBackdrop.hidden = false;
  panelTitle.textContent = `编辑资产 #${asset.id}`;
  submitBtn.textContent = "更新资产";
  deleteBtn.hidden = false;
  cancelEditBtn.hidden = false;
  formHint.textContent = "";

  form.elements.id.value = String(asset.id);
  form.elements.name.value = asset.name;
  form.elements.icon.value = asset.icon || "📦";
  form.elements.category.value = asset.category;
  form.elements.price.value = asset.price;
  form.elements.purchaseDate.value = asset.purchaseDate;
  form.elements.status.value = asset.status;
  form.elements.soldPrice.value = asset.soldPrice ?? "";
  form.elements.soldDate.value = asset.soldDate ?? "";
  syncFormIcon(asset.icon || "📦");
  syncSoldFields();
}

function resetFormState() {
  state.editingId = null;
  form.reset();
  formPanel.classList.remove("open");
  modalBackdrop.hidden = true;
  closeEmojiPicker();
  panelTitle.textContent = "新增资产";
  submitBtn.textContent = "保存资产";
  deleteBtn.hidden = true;
  cancelEditBtn.hidden = true;
  formHint.textContent = "";
  form.elements.icon.value = "📦";
  syncFormIcon("📦");
  syncSoldFields();
}

async function saveFullAsset(id, payload) {
  try {
    const response = await fetch(`/api/assets/${id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (!response.ok) {
      const data = await response.json();
      throw new Error(data.error || "更新失败");
    }
    await loadDashboard();
  } catch (error) {
    formHint.textContent = error.message;
  }
}

function renderEmojiPicker() {
  emojiGrid.innerHTML = "";
  emojiOptions.forEach((emoji) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "emoji-option";
    button.textContent = emoji;
    button.addEventListener("click", () => handleEmojiSelect(emoji));
    emojiGrid.appendChild(button);
  });
}

function openEmojiPicker(target) {
  state.iconTarget = target;
  emojiInput.value = "";
  emojiPicker.hidden = false;
  modalBackdrop.hidden = false;
  requestAnimationFrame(() => emojiInput.focus());
}

function closeEmojiPicker() {
  state.iconTarget = null;
  emojiPicker.hidden = true;
  if (!formPanel.classList.contains("open")) {
    modalBackdrop.hidden = true;
  }
}

async function handleEmojiSelect(emoji) {
  const target = state.iconTarget;
  if (!target) {
    return;
  }

  if (target.type === "form") {
    form.elements.icon.value = emoji;
    syncFormIcon(emoji);
    closeEmojiPicker();
    return;
  }

  const asset = state.assets.find((item) => item.id === target.assetId);
  if (!asset) {
    closeEmojiPicker();
    return;
  }

  closeEmojiPicker();
  await saveFullAsset(asset.id, toAssetPayload(asset, { icon: emoji }));
}

async function applyCustomEmoji() {
  const emoji = String(emojiInput.value || "").trim();
  if (!emoji) {
    emojiInput.focus();
    return;
  }
  await handleEmojiSelect(emoji);
}

function syncFormIcon(emoji) {
  formIconTrigger.textContent = emoji || "📦";
}

function toAssetPayload(asset, overrides = {}) {
  const payload = {
    name: asset.name,
    icon: asset.icon || "📦",
    category: asset.category,
    price: asset.price,
    purchaseDate: asset.purchaseDate,
    status: asset.status,
  };
  if (asset.soldPrice) {
    payload.soldPrice = asset.soldPrice;
  }
  if (asset.soldDate) {
    payload.soldDate = asset.soldDate;
  }

  return { ...payload, ...overrides };
}

async function loadDashboard() {
  const response = await fetch("/api/dashboard");
  const data = await response.json();
  state.assets = data.assets || [];
  state.summary = data.summary || null;
  state.analytics = data.analytics || null;
  renderFilters();
  renderSortSwitch();
  renderSummary();
  renderAssets();
  renderCharts();
}

syncSoldFields();
renderEmojiPicker();
form.elements.icon.value = "📦";
syncFormIcon("📦");
loadDashboard().catch((error) => {
  formHint.textContent = error.message;
});
