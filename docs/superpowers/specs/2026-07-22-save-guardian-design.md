# SaveGuardian — 手动存档备份系统设计

> **实现说明（as-built）**：本文档最初按"槽位（slot）自动识别"方案编写。实际实现中**放弃了槽位概念**——不同游戏存档结构各异，工具不自动探测槽位，而是**以用户指定的整个文件夹为备份单位**（"你指哪个文件夹，就整体备份哪个"）。下文已更新为与代码一致的最终设计；被放弃的槽位相关内容不再保留。以 README 与源码为准。

## 背景与目标

魂类游戏在死亡等事件时自动存档并施加死亡惩罚，玩家无法在终端阻止。本工具提供一个**手动存档备份与恢复系统**，让玩家在关键节点手动备份整个存档文件夹，遇到不利结果时恢复到备份状态，从而规避死亡惩罚。

工具做成**通用**的，可用于多个游戏，通过配置多个游戏档案实现。

## 核心需求

- 全局**热键**触发对当前选中游戏的快速备份。
- **Web 图形界面**管理备份：列表、备注、恢复、删除、批量删除，并通过 SSE 实时刷新。
- 备份时可写**备注**。
- 备份以**用户指定的整个文件夹为单位**（如整个存档槽位目录 `maingame1/`），而非单个 `.sav` 文件；工具不自动识别槽位。
- 恢复条件：玩家**退到游戏标题界面**即可，无需完全退出游戏。
- 备份目标目录**可手动指定**（文件夹选择器）。
- 支持**多游戏档案**：每个档案有独立的源文件夹与备份目标目录。
- 面向纯玩家分发：**单个 Windows `.exe`，双击即用，无需安装任何运行时**。

## 技术栈

- 后端：**Go**，用 `embed` 将前端资源打包进单一 `.exe`。
- 前端：**内嵌 HTML + 原生 JS + 现代 CSS**，魂系深色风格，单页应用。
- 全局热键 / 文件夹选择器：Windows 平台通过 `syscall` 调用系统 API，用构建标签隔离平台相关代码。
- 交叉编译：Linux 上 `GOOS=windows GOARCH=amd64` 产出 `.exe`。

## 目录与数据结构

配置文件 `saveguardian.config.json`（放在 exe 同目录；若不可写则退回当前工作目录）：

```json
{
  "activeGame": "Project Plague",
  "hotkey": "Ctrl+Alt+S",
  "port": 8787,
  "games": [
    {
      "name": "Project Plague",
      "source": "C:/Users/<用户名>/AppData/Local/Project_Plague/Saved/<数字>/GameSlots/maingame1",
      "backupRoot": "D:/MySaveBackups/ProjectPlague"
    }
  ]
}
```

- `source`：要整体备份的那个文件夹（备份单位）。旧版本的 `sourceRoot` 键在加载时会自动迁移到 `source`。

备份库结构（每次备份就是备份目标目录下的一个时间戳文件夹）：

```
<backupRoot>/
  <timestamp>/                # 如 20260722-153012；同秒冲突时追加 -1、-2…
    <source 文件夹的全部内容副本>
    meta.json
```

`meta.json` 字段（由 `internal/backup` 写入）：

```json
{
  "note": "打Boss前",
  "createdAt": "2026-07-22T15:30:12+08:00",
  "game": "Project Plague",
  "type": "manual",
  "files": [{ "path": "maingame1.sav", "size": 34682, "sha256": "..." }],
  "timestamp": "20260722-153012",
  "totalSize": 34682
}
```

- `type`：`manual`（界面）/ `hotkey`（热键）/ `pre-restore`（恢复前自动安全快照）。
- `files[].path` 为相对 `source` 的斜杠分隔路径，含每个文件的大小与 SHA-256 校验和。

## 组件划分

- **config 包**：读写 `saveguardian.config.json`（原子写），游戏档案增删改查，默认值，`sourceRoot`→`source` 迁移。
- **backup 包**：完整目录复制、生成时间戳、写 `meta.json`、计算校验和、列出/删除/改备注/恢复。**恢复逻辑也在本包内**（未单独拆 restore 包）：恢复前先对当前文件夹做安全快照（`type: pre-restore`），再用备份覆盖源文件夹，仅影响该文件夹。
- **api 包**：REST 端点 + SSE 实时推送（hub 广播），并暴露 `HotkeyBackup` 供热键回调使用。静态前端由 `main.go` 用 `http.FileServer` 服务。
- **hotkey 包**（平台隔离）：`Parse` 平台无关；`Listen` 在 Windows 用 `syscall` 注册全局热键，非 Windows 提供空实现，便于跨平台测试。
- **folderpick 包**（平台隔离）：Windows 打开系统文件夹选择器；非 Windows 空实现。
- **notify 包**（平台隔离）：Windows 合成 WAV 播放提示音（成功/失败不同音）；非 Windows 空实现。
- **webui**：`embed` 的静态前端资源，最终打进单一 `.exe`。

## HTTP API

- `GET    /api/config` — 返回配置与游戏档案列表。
- `POST   /api/games` — 新增/更新游戏档案。
- `DELETE /api/games?name=` — 删除档案。
- `POST   /api/active` — 设置当前活动游戏 `{name}`。
- `GET    /api/backups?game=` — 列出该游戏的备份（含备注、时间、大小、文件数），最新在前。
- `DELETE /api/backups?game=&timestamp=` — 删除某备份。
- `PATCH  /api/backups?game=` — 编辑备注 `{timestamp, note}`。
- `POST   /api/backups/batch-delete` — 批量删除 `{game, timestamps[]}`，返回失败列表。
- `POST   /api/backup` — 对 `{game, note, type}` 创建备份。
- `POST   /api/restore` — 恢复 `{game, timestamp}`，先做 pre-restore 快照，返回安全快照 meta。
- `POST   /api/pick-folder` — 打开系统文件夹选择器，返回所选路径（Windows）。
- `GET    /api/events` — SSE 事件流；任何备份/恢复/删除后广播 `update`（热键广播 `hotkey`），前端据此实时刷新。

## 数据流

1. 启动 exe → 读 `saveguardian.config.json` → 起本地 HTTP 服务 → 自动打开浏览器到界面 → 在 goroutine 中注册全局热键。
2. 备份：热键或界面触发 → 复制活动游戏的 `source` 文件夹到 `<backupRoot>/<timestamp>/` → 写 `meta.json` → SSE 广播 + 通知反馈。
3. 恢复：界面选择备份 → 提示"请先退到标题界面" → 对当前 `source` 文件夹做 pre-restore 快照 → 用备份内容覆盖该文件夹 → 反馈。

## 安全策略

- 原始存档只读复制，**绝不移动或删除**玩家 `.sav`。
- 恢复前**自动安全快照**（`type: pre-restore`），任何误恢复都可回退。
- 恢复只覆盖所选 `source` 文件夹，采用"暂存目录 + 原子替换 + 失败回滚"，避免中途中断产生半个存档；恢复时跳过备份里的 `meta.json`，不污染源文件夹。
- 备份同样"先写 `.tmp` 目录再原子重命名"，避免半个备份。
- 路径校验：时间戳名拒绝路径穿越（`..`、斜杠等）；`backupRoot` 与 `source` 不得互相嵌套以防递归复制。

## 错误处理

- 源文件夹不存在或不是目录：界面明确报错，不创建空备份。
- 备份目标不可写/磁盘满：报错并保留原状态。
- 恢复时目标被游戏占用（Windows 文件锁）：提示"请确认已退到标题界面"并允许重试。
- 备份元数据缺失/损坏：列表仍显示该条（`type: unknown`），不影响其它备份。

## 测试策略

- 平台无关的 config / backup（含恢复）核心逻辑用 Go 单元测试，在 Linux 上用临时目录模拟 source/backupRoot 验证：整目录复制完整性、meta 生成、校验和、newest-first 排序、pre-restore 快照、删除、改备注、路径越界拒绝、嵌套目录拒绝、`sourceRoot` 迁移。
- hotkey / folderpick / notify 用构建标签隔离，非 Windows 用空实现，不阻塞跨平台测试。
- 手动验收：交叉编译出 `.exe`，在 Windows 上验证热键、文件夹选择器、浏览器界面与真实恢复流程。

## 分发

- GitHub Release 通过 tag 触发 CI（`.github/workflows/release.yml`）：跑测试 → 交叉编译 → 发布预编译 `SaveGuardian.exe`。
- README 面向纯玩家：下载 → 双击 → 浏览器打开 → 添加游戏档案（选要备份的文件夹与备份目录）→ 用热键或界面备份/恢复。

## 明确不做（YAGNI）

- 不做云同步、不做账号系统、不做自动定时备份（本期）。
- 不解析 `.sav` 内部结构，仅按文件整体复制。
- **不自动识别槽位**：不同游戏存档结构各异，改由用户显式指定要备份的文件夹；需备份多个槽位时为每个各建一个档案。
