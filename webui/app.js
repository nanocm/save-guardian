"use strict";

const $ = (id) => document.getElementById(id);
const state = {
  config: null,
  game: null,
  selected: new Set(),
  lastList: [],          // full backup list (unfiltered) for the active game
  lastPreRestore: null,  // newest pre-restore snapshot, for "undo restore"
  filter: { text: "", type: "all" },
};

// Small inline icons (stroke uses currentColor so they inherit button color).
const ICON = {
  restore: '<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/></svg>',
  note: '<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/></svg>',
  verify: '<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l7 4v5c0 5-3.5 7.5-7 9-3.5-1.5-7-4-7-9V7Z"/><path d="M9 12l2 2 4-4"/></svg>',
  del: '<svg class="ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18"/><path d="M8 6V4h8v2"/><path d="M6 6l1 14h10l1-14"/></svg>',
};

async function api(path, opts) {
  const res = await fetch(path, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

function toast(msg, isErr) {
  const t = $("toast");
  t.textContent = msg;
  t.className = "toast" + (isErr ? " err" : "");
  t.hidden = false;
  clearTimeout(toast._t);
  toast._t = setTimeout(() => (t.hidden = true), 3200);
}

function fmtBytes(n) {
  if (!n) return "0 B";
  const u = ["B", "KB", "MB", "GB"];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return n.toFixed(i ? 1 : 0) + " " + u[i];
}

function fmtTime(iso) {
  if (!iso) return "";
  const d = new Date(iso);
  if (isNaN(d)) return iso;
  const p = (x) => String(x).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

function fmtRelative(iso) {
  const d = new Date(iso);
  if (isNaN(d)) return "";
  const s = Math.floor((Date.now() - d.getTime()) / 1000);
  if (s < 0) return "";
  if (s < 60) return "刚刚";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} 分钟前`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h} 小时前`;
  const days = Math.floor(h / 24);
  if (days < 30) return `${days} 天前`;
  const mo = Math.floor(days / 30);
  if (mo < 12) return `${mo} 个月前`;
  return `${Math.floor(mo / 12)} 年前`;
}

async function loadConfig() {
  state.config = await api("/api/config");
  $("hotkeyHint").textContent = state.config.hotkey ? `热键：${state.config.hotkey}` : "";
  renderGameSelect();
}

function renderGameSelect() {
  const sel = $("gameSelect");
  const games = state.config.games || [];
  sel.innerHTML = "";
  if (games.length === 0) {
    const o = document.createElement("option");
    o.textContent = "（无档案，请先管理档案）";
    o.value = "";
    sel.appendChild(o);
    state.game = null;
    renderBackups([]);
    return;
  }
  for (const g of games) {
    const o = document.createElement("option");
    o.value = g.name;
    o.textContent = g.name;
    sel.appendChild(o);
  }
  state.game = state.config.activeGame || games[0].name;
  sel.value = state.game;
  loadBackups();
}

async function loadBackups() {
  if (!state.game) { renderBackups([]); return; }
  $("loading").hidden = false;
  try {
    const { backups } = await api(`/api/backups?game=${encodeURIComponent(state.game)}`);
    renderBackups(backups || []);
  } catch (e) {
    toast("读取备份失败：" + e.message, true);
  } finally {
    $("loading").hidden = true;
  }
}

// applyFilter derives the visible subset from the full list using the current
// search text and type chip.
function applyFilter(list) {
  const t = state.filter.text.trim().toLowerCase();
  const ty = state.filter.type;
  return list.filter((b) => {
    if (ty !== "all" && (b.type || "manual") !== ty) return false;
    if (t) {
      const hay = ((b.note || "") + " " + (b.timestamp || "") + " " + (fmtTime(b.createdAt) || "")).toLowerCase();
      if (!hay.includes(t)) return false;
    }
    return true;
  });
}

function renderBackups(list) {
  state.lastList = list || [];
  renderStats(state.lastList);
  updateUndo();

  const visible = applyFilter(state.lastList);
  const wrap = $("backupList");
  wrap.innerHTML = "";
  const hasAny = state.lastList.length > 0;
  $("emptyHint").hidden = hasAny;
  $("noMatch").hidden = !(hasAny && visible.length === 0);

  // Drop selections for backups that no longer exist.
  const present = new Set(state.lastList.map((b) => b.timestamp));
  for (const ts of [...state.selected]) if (!present.has(ts)) state.selected.delete(ts);

  for (const b of visible) {
    const el = document.createElement("div");
    el.className = "backup " + (b.type || "manual");
    if (state.selected.has(b.timestamp)) el.classList.add("selected");
    const badge = b._verified === "ok"
      ? `<span class="b-badge ok" title="校验通过">✓ 完整</span>`
      : b._verified === "bad"
        ? `<span class="b-badge bad" title="校验未通过">⚠ 损坏</span>`
        : "";
    const noteHtml = b.note
      ? `<div class="b-note">${escapeHtml(b.note)}${badge}</div>`
      : `<div class="b-note empty-note">（无备注）${badge}</div>`;
    const rel = fmtRelative(b.createdAt);
    el.innerHTML = `
      <label class="b-check"><input type="checkbox" /></label>
      <div class="b-main">
        ${noteHtml}
        <div class="b-meta">
          <span class="tag ${b.type || "manual"}">${typeLabel(b.type)}</span>
          <span>${fmtTime(b.createdAt) || b.timestamp}</span>
          ${rel ? `<span>${rel}</span>` : ""}
          <span>${fmtBytes(b.totalSize)}</span>
          <span>${(b.files || []).length} 个文件</span>
        </div>
      </div>
      <div class="b-actions"></div>`;
    const cb = el.querySelector(".b-check input");
    cb.checked = state.selected.has(b.timestamp);
    const setSelected = (on) => {
      if (on) state.selected.add(b.timestamp);
      else state.selected.delete(b.timestamp);
      cb.checked = on;
      el.classList.toggle("selected", on);
      updateSelectionUI();
    };
    cb.onchange = () => setSelected(cb.checked);
    el.onclick = (ev) => {
      if (ev.target.closest(".b-actions")) return;
      if (ev.target.closest(".b-check")) return;
      setSelected(!state.selected.has(b.timestamp));
    };
    const actions = el.querySelector(".b-actions");
    actions.appendChild(btn(ICON.restore + "恢复", "primary small", () => askRestore(b), "恢复到此备份"));
    actions.appendChild(btn(ICON.note + "备注", "ghost small", () => startEditNote(b, el), "编辑备注"));
    actions.appendChild(btn(ICON.verify + "校验", "ghost small", () => verifyBackup(b), "校验完整性"));
    actions.appendChild(btn(ICON.del + "删除", "ghost small", () => del(b), "删除此备份"));
    wrap.appendChild(el);
  }
  updateSelectionUI();
}

function renderStats(list) {
  const bar = $("statsbar");
  if (!list.length) { bar.hidden = true; return; }
  bar.hidden = false;
  const total = list.reduce((a, b) => a + (b.totalSize || 0), 0);
  $("stats").textContent = `共 ${list.length} 个备份 · 占用 ${fmtBytes(total)}`;
}

function updateUndo() {
  // The list is newest-first, so the first pre-restore entry is the latest.
  const pr = (state.lastList || []).find((b) => b.type === "pre-restore");
  state.lastPreRestore = pr || null;
  $("undoBtn").hidden = !pr;
}

function updateSelectionUI() {
  const n = state.selected.size;
  const delBtn = $("deleteSelectedBtn");
  delBtn.hidden = n === 0;
  delBtn.textContent = `删除选中 (${n})`;
  const all = $("selectAll");
  const visible = applyFilter(state.lastList);
  const total = visible.length;
  const selectedVisible = visible.filter((b) => state.selected.has(b.timestamp)).length;
  all.checked = total > 0 && selectedVisible === total;
  all.indeterminate = selectedVisible > 0 && selectedVisible < total;
}

async function deleteSelected() {
  const timestamps = [...state.selected];
  if (!timestamps.length) return;
  if (!(await confirmDialog(`确认删除选中的 ${timestamps.length} 个备份？此操作不可撤销。`))) return;
  try {
    const res = await api("/api/backups/batch-delete", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ game: state.game, timestamps }),
    });
    state.selected.clear();
    const failed = (res && res.failed) || [];
    if (failed.length) toast(`已删除，但有 ${failed.length} 个失败`, true);
    else toast(`已删除 ${timestamps.length} 个备份`);
    loadBackups();
  } catch (e) {
    toast("批量删除失败：" + e.message, true);
  }
}

function typeLabel(t) {
  return { manual: "手动", hotkey: "热键", "pre-restore": "安全快照" }[t] || t || "手动";
}

function btn(html, cls, on, title) {
  const b = document.createElement("button");
  b.innerHTML = html;
  b.className = cls;
  if (title) b.title = title;
  b.onclick = on;
  return b;
}

function escapeHtml(s) {
  return s.replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

async function doBackup(type) {
  if (!state.game) { toast("请先选择游戏档案", true); return; }
  try {
    await api("/api/backup", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ game: state.game, note: $("noteInput").value.trim(), type: type || "manual" }),
    });
    $("noteInput").value = "";
    toast("备份成功");
    loadBackups();
  } catch (e) {
    toast("备份失败：" + e.message, true);
  }
}

async function verifyBackup(b) {
  try {
    const r = await api(`/api/verify?game=${encodeURIComponent(state.game)}&timestamp=${encodeURIComponent(b.timestamp)}`);
    b._verified = r.ok ? "ok" : "bad";
    if (r.ok) toast("校验通过：备份完整");
    else toast(`校验未通过：${(r.corrupt || []).length} 个文件损坏`, true);
    renderBackups(state.lastList);
  } catch (e) {
    toast("校验失败：" + e.message, true);
  }
}

let pendingRestore = null;
function askRestore(b) {
  pendingRestore = b;
  $("restoreTarget").textContent = `${state.game} · ${b.timestamp}${b.note ? " · " + b.note : ""}`;
  $("restoreModal").hidden = false;
  $("confirmRestoreBtn").focus();
}

async function confirmRestore() {
  $("restoreModal").hidden = true;
  if (!pendingRestore) return;
  try {
    await api("/api/restore", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ game: state.game, timestamp: pendingRestore.timestamp }),
    });
    toast("已恢复。已自动创建恢复前安全快照。");
    loadBackups();
  } catch (e) {
    toast("恢复失败：" + e.message, true);
  }
  pendingRestore = null;
}

async function undoRestore() {
  const pr = state.lastPreRestore;
  if (!pr) return;
  const when = fmtTime(pr.createdAt) || pr.timestamp;
  if (!(await confirmDialog(`撤销上次恢复？将把存档回退到恢复前的安全快照：\n${when}`, { okText: "撤销恢复" }))) return;
  try {
    await api("/api/restore", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ game: state.game, timestamp: pr.timestamp }),
    });
    toast("已撤销：存档已回到恢复前状态");
    loadBackups();
  } catch (e) {
    toast("撤销失败：" + e.message, true);
  }
}

async function del(b) {
  if (!(await confirmDialog(`删除该备份？\n${b.timestamp}${b.note ? " · " + b.note : ""}`))) return;
  try {
    await api(`/api/backups?game=${encodeURIComponent(state.game)}&timestamp=${encodeURIComponent(b.timestamp)}`, { method: "DELETE" });
    toast("已删除");
    loadBackups();
  } catch (e) {
    toast("删除失败：" + e.message, true);
  }
}

function startEditNote(b, el) {
  const main = el.querySelector(".b-main");
  const noteEl = main.querySelector(".b-note");
  if (!noteEl || main.querySelector(".note-edit")) return; // already editing

  const input = document.createElement("input");
  input.type = "text";
  input.className = "note-edit";
  input.maxLength = 120;
  input.value = b.note || "";
  input.placeholder = "输入备注，回车保存，Esc 取消";
  noteEl.replaceWith(input);
  input.focus();
  input.select();

  let done = false;
  const finish = async (save) => {
    if (done) return;
    done = true;
    const next = input.value.trim();
    if (save && next !== (b.note || "")) {
      try {
        await api(`/api/backups?game=${encodeURIComponent(state.game)}`, {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ timestamp: b.timestamp, note: next }),
        });
        b.note = next;
        toast("备注已保存");
      } catch (e) {
        toast("修改失败：" + e.message, true);
      }
    }
    loadBackups();
  };

  input.addEventListener("click", (ev) => ev.stopPropagation());
  input.addEventListener("keydown", (ev) => {
    ev.stopPropagation();
    if (ev.key === "Enter") { ev.preventDefault(); finish(true); }
    else if (ev.key === "Escape") { ev.preventDefault(); finish(false); }
  });
  input.addEventListener("blur", () => finish(true));
}

// ---- themed confirm dialog (replaces native confirm) ----
let confirmResolver = null;
function confirmDialog(msg, opts = {}) {
  return new Promise((resolve) => {
    // If a previous confirm is somehow open, resolve it false first.
    if (confirmResolver) confirmResolver(false);
    confirmResolver = resolve;
    $("confirmMsg").textContent = msg;
    const ok = $("confirmOkBtn");
    ok.textContent = opts.okText || "确认";
    ok.className = opts.danger === false ? "primary" : "danger";
    $("confirmModal").hidden = false;
    ok.focus();
  });
}
function closeConfirm(result) {
  $("confirmModal").hidden = true;
  const r = confirmResolver;
  confirmResolver = null;
  if (r) r(result);
}

// ---- Games management ----
function openGames() {
  renderGamesTable();
  $("gName").value = "";
  $("gSource").value = "";
  $("gBackup").value = "";
  $("gamesModal").hidden = false;
}

function renderGamesTable() {
  const t = $("gamesTable");
  t.innerHTML = "";
  const games = state.config.games || [];
  if (!games.length) {
    t.innerHTML = `<p class="empty">还没有游戏档案。请在下方新增。</p>`;
    return;
  }
  for (const g of games) {
    const row = document.createElement("div");
    row.className = "game-row";
    row.innerHTML = `<div><div class="g-name">${escapeHtml(g.name)}</div>
      <div class="g-paths">备份文件夹：${escapeHtml(g.source)}<br/>备份到：${escapeHtml(g.backupRoot)}</div></div>`;
    const acts = document.createElement("div");
    acts.appendChild(btn("编辑", "ghost small", () => {
      $("gName").value = g.name; $("gSource").value = g.source; $("gBackup").value = g.backupRoot;
    }));
    acts.appendChild(btn("删除", "ghost small", async () => {
      if (!(await confirmDialog(`删除档案「${g.name}」？（不会删除已有备份文件）`))) return;
      state.config = await api("/api/games?name=" + encodeURIComponent(g.name), { method: "DELETE" });
      renderGamesTable(); renderGameSelect();
    }));
    row.appendChild(acts);
    t.appendChild(row);
  }
}

async function saveGame() {
  const g = { name: $("gName").value.trim(), source: $("gSource").value.trim(), backupRoot: $("gBackup").value.trim() };
  if (!g.name || !g.source || !g.backupRoot) { toast("请填写名称、要备份的文件夹、备份目标目录", true); return; }
  try {
    state.config = await api("/api/games", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(g),
    });
    toast("档案已保存");
    renderGamesTable();
    renderGameSelect();
  } catch (e) {
    toast("保存失败：" + e.message, true);
  }
}

async function pickInto(inputId) {
  try {
    const { path } = await api("/api/pick-folder", { method: "POST" });
    if (path) $(inputId).value = path;
  } catch (e) {
    toast("此平台不支持文件夹选择，请手动粘贴路径", true);
  }
}

// ---- modal close infrastructure (Esc + backdrop click) ----
const MODALS = ["gamesModal", "restoreModal", "confirmModal"];
function closeModal(id) {
  if (id === "confirmModal") { closeConfirm(false); return; }
  $(id).hidden = true;
  if (id === "restoreModal") pendingRestore = null;
}
function topOpenModal() {
  // confirmModal sits on top of the others when stacked.
  if (!$("confirmModal").hidden) return "confirmModal";
  return MODALS.find((id) => !$(id).hidden);
}

function bind() {
  $("gameSelect").onchange = async (e) => {
    state.game = e.target.value;
    state.selected.clear();
    state.filter = { text: "", type: "all" };
    $("filterInput").value = "";
    syncChips();
    try { state.config = await api("/api/active", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: state.game }) }); } catch {}
    loadBackups();
  };
  $("backupBtn").onclick = () => doBackup("manual");
  $("undoBtn").onclick = undoRestore;
  $("refreshBtn").onclick = loadBackups;
  $("deleteSelectedBtn").onclick = deleteSelected;
  $("selectAll").onchange = (e) => {
    const visible = applyFilter(state.lastList);
    if (e.target.checked) visible.forEach((b) => state.selected.add(b.timestamp));
    else visible.forEach((b) => state.selected.delete(b.timestamp));
    renderBackups(state.lastList);
  };
  $("manageGamesBtn").onclick = openGames;
  $("closeGamesBtn").onclick = () => ($("gamesModal").hidden = true);
  $("saveGameBtn").onclick = saveGame;
  $("confirmRestoreBtn").onclick = confirmRestore;
  $("cancelRestoreBtn").onclick = () => { $("restoreModal").hidden = true; pendingRestore = null; };
  $("confirmOkBtn").onclick = () => closeConfirm(true);
  $("confirmCancelBtn").onclick = () => closeConfirm(false);
  document.querySelectorAll("[data-pick]").forEach((b) => (b.onclick = () => pickInto(b.dataset.pick)));
  $("noteInput").addEventListener("keydown", (e) => { if (e.key === "Enter") doBackup("manual"); });

  // Filter: search box + type chips.
  $("filterInput").addEventListener("input", (e) => {
    state.filter.text = e.target.value;
    renderBackups(state.lastList);
  });
  $("typeChips").addEventListener("click", (e) => {
    const chip = e.target.closest(".chip");
    if (!chip) return;
    state.filter.type = chip.dataset.type;
    syncChips();
    renderBackups(state.lastList);
  });

  // Esc closes the top-most modal; clicking the backdrop closes that modal.
  document.addEventListener("keydown", (e) => {
    if (e.key !== "Escape") return;
    const open = topOpenModal();
    if (open) { e.preventDefault(); closeModal(open); }
  });
  MODALS.forEach((id) => {
    $(id).addEventListener("mousedown", (e) => { if (e.target === $(id)) closeModal(id); });
  });
}

function syncChips() {
  document.querySelectorAll("#typeChips .chip").forEach((c) => {
    c.classList.toggle("active", c.dataset.type === state.filter.type);
  });
}

function startEvents() {
  try {
    const es = new EventSource("/api/events");
    es.onmessage = (e) => {
      loadBackups();
      if (e.data === "hotkey") toast("热键备份成功");
    };
    es.onerror = () => { /* browser auto-reconnects using server retry hint */ };
  } catch (_) {
    // EventSource unsupported; manual refresh button still works.
  }
}

bind();
startEvents();
loadConfig().catch((e) => toast("初始化失败：" + e.message, true));
