# SaveGuardian — 手动存档备份系统设计

## 背景与目标

魂类游戏在死亡等事件时自动存档并施加死亡惩罚，玩家无法在终端阻止。本工具提供一个**手动存档备份与恢复系统**，让玩家在关键节点手动备份整个存档槽位，遇到不利结果时恢复到备份状态，从而规避死亡惩罚。

工具做成**通用**的，可用于多个游戏，通过配置多个游戏档案实现。

## 核心需求

- 全局**热键**触发对当前选中游戏+槽位的快速备份。
- **Web 图形界面**管理备份：列表、备注、恢复、删除。
- 备份时可写**备注**。
- 备份以**槽位目录为单位**（如整个 `maingame1/`），而非单个 `.sav` 文件。
- 恢复条件：玩家**退到游戏标题界面**即可，无需完全退出游戏。
- 备份目标目录**可手动指定**（文件夹选择器）。
- 支持**多游戏档案**：每个档案有独立的源存档目录与备份目标目录。
- 面向纯玩家分发：**单个 Windows `.exe`，双击即用，无需安装任何运行时**。

## 技术栈

- 后端：**Go**，用 `embed` 将前端资源打包进单一 `.exe`。
- 前端：**内嵌 HTML + 原生 JS + 现代 CSS**，魂系深色风格，单页应用。
- 全局热键 / 文件夹选择器：Windows 平台通过 `syscall` 调用系统 API，用构建标签隔离平台相关代码。
- 交叉编译：Linux 上 `GOOS=windows GOARCH=amd64` 产出 `.exe`。

## 目录与数据结构

配置文件 `config.json`（放在 exe 同目录）：

```json
{
  "activeGame": "Project Plague",
  "hotkey": "Ctrl+Alt+S",
  "port": 8787,
  "games": [
    {
      "name": "Project Plague",
      "sourceRoot": "C:/Users/cms/AppData/Local/Project_Plague/Saved/1008354121/GameSlots",
      "backupRoot": "D:/MySaveBackups/ProjectPlague"
    }
  ]
}
```

备份库结构：

```
<backupRoot>/
  <slotName>/                 # 如 maingame1
    <timestamp>/              # 如 20260722-153012
      <完整槽位目录副本>
      meta.json
```

`meta.json` 字段：

```json
{
  "note": "打Boss前",
  "createdAt": "2026-07-22T15:30:12+08:00",
  "game": "Project Plague",
  "slot": "maingame1",
  "type": "manual",
  "files": [{ "path": "maingame1.sav", "size": 34682, "sha256": "..." }]
}
```

## 组件划分

- **config 包**：读写 `config.json`，游戏档案增删改查，默认值。
- **backup 包**：扫描槽位、完整目录复制、生成时间戳、写 `meta.json`、计算校验和、列出/删除备份。
- **restore 包**：恢复前对当前槽位做安全快照（`type: pre-restore`），再用备份覆盖槽位目录，仅影响目标槽位。
- **httpapi 包**：REST 端点，服务内嵌前端。
- **hotkey 包**（平台隔离）：Windows 注册全局热键；非 Windows 提供空实现，便于跨平台测试。
- **folderpick 包**（平台隔离）：Windows 打开系统文件夹选择器；非 Windows 空实现。
- **webui**：`embed` 的静态前端资源。

## HTTP API

- `GET  /api/config` — 返回配置与游戏档案列表。
- `POST /api/games` — 新增/更新游戏档案。
- `DELETE /api/games?name=` — 删除档案。
- `POST /api/active` — 设置当前活动游戏。
- `GET  /api/slots?game=` — 列出该游戏源目录下的槽位。
- `GET  /api/backups?game=&slot=` — 列出某槽位的备份（含备注、时间、大小）。
- `POST /api/backup` — 对 `{game, slot, note}` 创建备份。
- `POST /api/restore` — 恢复 `{game, slot, timestamp}`，先做 pre-restore 快照。
- `DELETE /api/backup?game=&slot=&timestamp=` — 删除某备份。
- `PATCH /api/backup` — 编辑备注。
- `POST /api/pick-folder` — 打开系统文件夹选择器，返回所选路径（Windows）。

## 数据流

1. 启动 exe → 读 `config.json` → 起本地 HTTP 服务 → 自动打开浏览器到界面 → 注册全局热键。
2. 备份：热键或界面触发 → 复制活动游戏当前槽位目录到 `<backupRoot>/<slot>/<timestamp>/` → 写 `meta.json` → 界面/通知反馈。
3. 恢复：界面选择备份 → 提示“请先退到标题界面” → 对当前槽位做 pre-restore 快照 → 用备份内容覆盖槽位目录 → 反馈。

## 安全策略

- 原始存档只读复制，**绝不移动或删除**玩家 `.sav`。
- 恢复前**自动安全快照**，任何误恢复都可回退。
- 恢复只覆盖目标槽位目录，不触碰其它槽位。
- 复制采用“先写临时目录再原子重命名”，避免中途中断产生半个备份。
- 路径校验：拒绝越界路径，backupRoot 与 sourceRoot 不得互相嵌套以防递归复制。

## 错误处理

- 源目录/槽位不存在：界面明确报错，不创建空备份。
- 备份目标不可写/磁盘满：报错并保留原状态。
- 恢复时目标被游戏占用（Windows 文件锁）：提示“请确认已退到标题界面”并允许重试。
- 校验和不匹配（备份损坏）：恢复前警告用户。

## 测试策略

- 平台无关的 config / backup / restore 核心逻辑用 Go 单元测试，在 Linux 上用临时目录模拟 sourceRoot/backupRoot 验证：复制完整性、meta 生成、校验和、pre-restore 快照、删除、路径越界拒绝。
- hotkey / folderpick 用构建标签隔离，测试用空实现，不阻塞跨平台测试。
- 手动验收：交叉编译出 `.exe`，在 Windows 上验证热键、文件夹选择器、浏览器界面与真实恢复流程。

## 分发

- GitHub Release 提供预编译 `SaveGuardian.exe`。
- README 面向纯玩家：下载 → 双击 → 浏览器打开 → 添加游戏档案（选源目录与备份目录）→ 用热键或界面备份/恢复。

## 明确不做（YAGNI）

- 不做云同步、不做账号系统、不做自动定时备份（本期）。
- 不解析 `.sav` 内部结构，仅按文件整体复制。
