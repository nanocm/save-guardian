"use strict";

const $ = (id) => document.getElementById(id);
const state = { config: null, game: null, selected: new Set() };

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
  try {
    const { backups } = await api(`/api/backups?game=${encodeURIComponent(state.game)}`);
    renderBackups(backups || []);
  } catch (e) {
    toast("读取备份失败：" + e.message, true);
  }
}

function renderBackups(list) {
  state.lastList = list || [];
  const wrap = $("backupList");
  wrap.innerHTML = "";
  $("emptyHint").hidden = list.length > 0;
  // Drop selections for backups that no longer exist.
  const present = new Set(list.map((b) => b.timestamp));
  for (const ts of [...state.selected]) if (!present.has(ts)) state.selected.delete(ts);

  for (const b of list) {
    const el = document.createElement("div");
    el.className = "backup " + (b.type || "manual");
    if (state.selected.has(b.timestamp)) el.classList.add("selected");
    const noteHtml = b.note
      ? `<div class="b-note">${escapeHtml(b.note)}</div>`
      : `<div class="b-note empty-note">（无备注）</div>`;
    el.innerHTML = `
      <label class="b-check"><input type="checkbox" /></label>
      <div class="b-main">
        ${noteHtml}
        <div class="b-meta">
          <span class="tag ${b.type || "manual"}">${typeLabel(b.type)}</span>
          <span>${fmtTime(b.createdAt) || b.timestamp}</span>
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
      updateSelectionUI(list);
    };
    cb.onchange = () => setSelected(cb.checked);
    el.onclick = (ev) => {
      // Ignore clicks on the action buttons or the checkbox itself.
      if (ev.target.closest(".b-actions")) return;
      if (ev.target.closest(".b-check")) return;
      setSelected(!state.selected.has(b.timestamp));
    };
    const actions = el.querySelector(".b-actions");
    actions.appendChild(btn("恢复", "primary small", () => askRestore(b)));
    actions.appendChild(btn("备注", "ghost small", () => startEditNote(b, el)));
    actions.appendChild(btn("删除", "ghost small", () => del(b)));
    wrap.appendChild(el);
  }
  updateSelectionUI(list);
}

function updateSelectionUI(list) {
  const n = state.selected.size;
  const delBtn = $("deleteSelectedBtn");
  delBtn.hidden = n === 0;
  delBtn.textContent = `删除选中 (${n})`;
  const all = $("selectAll");
  const total = (list || []).length;
  all.checked = total > 0 && n === total;
  all.indeterminate = n > 0 && n < total;
}

async function deleteSelected() {
  const timestamps = [...state.selected];
  if (!timestamps.length) return;
  if (!confirm(`确认删除选中的 ${timestamps.length} 个备份？此操作不可撤销。`)) return;
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

function btn(text, cls, on) {
  const b = document.createElement("button");
  b.textContent = text;
  b.className = cls;
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

let pendingRestore = null;
function askRestore(b) {
  pendingRestore = b;
  $("restoreTarget").textContent = `${state.game} · ${b.timestamp}${b.note ? " · " + b.note : ""}`;
  $("restoreModal").hidden = false;
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

async function del(b) {
  if (!confirm(`删除该备份？\n${b.timestamp}`)) return;
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
      if (!confirm(`删除档案「${g.name}」？（不会删除已有备份文件）`)) return;
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

function bind() {
  $("gameSelect").onchange = async (e) => {
    state.game = e.target.value;
    state.selected.clear();
    try { state.config = await api("/api/active", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: state.game }) }); } catch {}
    loadBackups();
  };
  $("backupBtn").onclick = () => doBackup("manual");
  $("refreshBtn").onclick = loadBackups;
  $("deleteSelectedBtn").onclick = deleteSelected;
  $("selectAll").onchange = (e) => {
    const list = state.lastList || [];
    if (e.target.checked) list.forEach((b) => state.selected.add(b.timestamp));
    else state.selected.clear();
    renderBackups(list);
  };
  $("manageGamesBtn").onclick = openGames;
  $("closeGamesBtn").onclick = () => ($("gamesModal").hidden = true);
  $("saveGameBtn").onclick = saveGame;
  $("confirmRestoreBtn").onclick = confirmRestore;
  $("cancelRestoreBtn").onclick = () => { $("restoreModal").hidden = true; pendingRestore = null; };
  document.querySelectorAll("[data-pick]").forEach((b) => (b.onclick = () => pickInto(b.dataset.pick)));
  $("noteInput").addEventListener("keydown", (e) => { if (e.key === "Enter") doBackup("manual"); });
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
