<div align="center">

# SaveGuardian

**Manual save backup / restore tool for Souls-like games — back up at key moments, roll back after a bad run in one click.**

[English](README.en.md) · [简体中文](README.md)

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows-0078D6.svg)](https://github.com/nanocm/save-guardian/releases)
[![Made with Go](https://img.shields.io/badge/made%20with-Go-00ADD8.svg)](https://go.dev)
[![Release](https://img.shields.io/github/v/release/nanocm/save-guardian?include_prereleases&sort=semver)](https://github.com/nanocm/save-guardian/releases)
[![Stars](https://img.shields.io/github/stars/nanocm/save-guardian?style=social)](https://github.com/nanocm/save-guardian)

</div>

---

Souls-like games autosave on death and impose a penalty you can't stop from inside the game. **SaveGuardian** lets you manually back up the **entire save folder** at key moments and restore it in one click when things go wrong — sidestepping the death penalty.

- 🎮 Works with any game: backs up a **whole folder** as a unit, independent of any save format
- 🖱️ A single `.exe` — **no install, just double-click**, no runtime required
- 🌐 A **web UI** that opens automatically and refreshes in real time

## ✨ Features

| | Feature |
|---|---|
| ⌨️ | **Global hotkey** backup (default `Ctrl+Alt+S`) with a soft chime; the **hotkey can be changed right in the UI** |
| 🖼️ | **Web UI**: view / restore / delete / batch-delete backups, with **live auto-refresh** over SSE |
| ↩️ | **Undo last restore** in one click, in case you rolled back to the wrong point |
| 🛡️ | **Automatic pre-restore snapshot** + **SHA-256 integrity check** (corrupt backups are refused; verify on demand) |
| 🔎 | **Search & type filters**, total backup count / size stats, relative timestamps |
| ✏️ | **Inline note editing**: edit right on the record, Enter to save |
| 📁 | **Choosable backup destination** (native folder picker) |
| 🎯 | **Multiple game profiles**: manage several games from one tool |

> To restore, just **return to the game's title screen** — no need to fully quit the game.

## 🚀 Quick start (no technical background needed)

1. Download `SaveGuardian.exe` from [Releases](../../releases), drop it in any folder, and **double-click** it.
2. It opens the web UI automatically (if not, visit `http://127.0.0.1:8787`).
3. Click **“Manage profiles” → add a profile**:
   - **Name**: anything, e.g. `Project Plague`.
   - **Folder to back up**: the folder you want backed up as a whole (all files inside are included), e.g. a save-slot directory:
     `C:\Users\<you>\AppData\Local\Project_Plague\Saved\<number>\GameSlots\maingame1`
   - **Backup destination**: where backups are stored, e.g. `D:\MySaveBackups\ProjectPlague`.
   - Pick paths with the native folder picker (“Choose…”) or paste them directly.
   - The tool **does not auto-detect slots** — whatever folder you point at is backed up whole. To back up multiple slots, create one profile per slot.
4. Select the active **game profile** at the top.
5. **Back up**: add an optional note and click **“Back up now”**, or just press the hotkey **`Ctrl+Alt+S`**.
6. **Restore**: **return to the title screen** in-game, then find the target backup and click **“Restore”** and confirm. A safety snapshot of the current save is taken automatically first; if you restored the wrong one, click **“Undo last restore”**.

> Note: restoring overwrites the folder you configured with the backup's contents. Original files are only ever copied, **never moved or deleted**.

## ⚙️ Settings & configuration

Click **“Settings”** in the top-right to **change the global hotkey**: focus the field and press the key combo to record it, or type it manually. It's applied immediately on save (in rare cases it asks you to restart).

On first run, `saveguardian.config.json` is created next to the `.exe`:

```json
{
  "activeGame": "Project Plague",
  "hotkey": "Ctrl+Alt+S",
  "port": 8787,
  "games": [
    { "name": "Project Plague",
      "source": "C:/.../GameSlots/maingame1",
      "backupRoot": "D:/MySaveBackups/ProjectPlague" }
  ]
}
```

- `source`: the folder to back up as a whole. (The old `sourceRoot` key is still read and migrated automatically.)
- `hotkey`: `Ctrl` / `Alt` / `Shift` / `Win` combos + letters / digits / `F1`–`F12`, e.g. `Ctrl+Alt+F5`.
- `port`: the web UI port; change it if the default is taken.

## 📦 What a backup looks like

```
<your backup destination>/
  20260722-154108/            # one backup (timestamp)
    ...(all files from your configured folder)
    meta.json                 # note, time, type, per-file size & SHA-256
  20260722-160233/
    ...
```

`meta.json` records the note, creation time, backup type (`manual` / `hotkey` / `pre-restore`), and each file's size and checksum.

## 🔒 Safety

- Original saves are **only ever copied read-only**; the tool never moves or deletes your `.sav` files.
- A `pre-restore` safety snapshot is created automatically, and the backup's **integrity is verified** first — corrupt backups are refused.
- Restore uses a “staging dir + atomic swap + rollback on failure” to avoid leaving a half-restored save.
- The backup destination and source folder may not be nested inside each other, preventing recursive copies.
- The local web API enforces a **same-origin check**, so other web pages can't trigger backup / restore / delete cross-site.

## 🧩 Known limitations

- The global hotkey, folder picker, and chime are **Windows** features; on other platforms the web UI still works and you can paste paths manually.
- If a restore reports a file is in use, make sure you've returned to the title screen and try again.

## 🛠️ Build from source (developers)

Requires [Go](https://go.dev/dl/) 1.22+.

```bash
# run tests (with the race detector)
go test -race ./...

# cross-compile the Windows exe (from any platform)
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o SaveGuardian.exe .

# or use the script
./build.sh
```

Frontend assets (`webui/`) are bundled into the `.exe` via Go `embed`, so the final artifact is a single executable.

<details>
<summary><b>Project layout</b></summary>

```
main.go                    entry point: serve, open browser, register hotkey
internal/config/           config & multi-game profile read/write
internal/backup/           backup / restore / checksums / integrity check (with unit tests)
internal/api/              HTTP REST API (same-origin check, op serialization) + SSE
internal/hotkey/           global hotkey (Windows syscall, runtime re-arm; no-op elsewhere)
internal/folderpick/       native folder picker (Windows only)
internal/notify/           chime (synthesized WAV on Windows; no-op elsewhere)
webui/                     embedded single-page UI (HTML/CSS/JS)
docs/superpowers/specs/    design docs
```

</details>

## 🤝 Feedback & contributing

Issues and PRs welcome: https://github.com/nanocm/save-guardian/issues

## 📄 License

Released under the [MIT License](LICENSE).
