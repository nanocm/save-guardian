<div align="center">

# SaveGuardian · 存档守护

**魂类游戏的手动存档备份 / 恢复工具 —— 关键节点一键备份，翻车后一键回档。**

简体中文 · [English](README.en.md)

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows-0078D6.svg)](https://github.com/nanocm/save-guardian/releases)
[![Made with Go](https://img.shields.io/badge/made%20with-Go-00ADD8.svg)](https://go.dev)
[![Release](https://img.shields.io/github/v/release/nanocm/save-guardian?include_prereleases&sort=semver)](https://github.com/nanocm/save-guardian/releases)
[![Stars](https://img.shields.io/github/stars/nanocm/save-guardian?style=social)](https://github.com/nanocm/save-guardian)

</div>

---

魂类（Souls-like）游戏在死亡时自动存档并施加惩罚，玩家无法在终端阻止。**SaveGuardian** 让你在关键节点手动备份**整个存档文件夹**，遇到不利结果时一键恢复到之前的状态，从而规避死亡惩罚。

- 🎮 通用于各种游戏：以**整个文件夹**为单位备份，不依赖任何游戏的存档格式
- 🖱️ 单个 `.exe`，**免安装、双击即用**，无需任何运行时
- 🌐 打开即用的**网页图形界面**，列表实时刷新

## ✨ 功能一览

| | 功能 |
|---|---|
| ⌨️ | **全局热键**一键备份（默认 `Ctrl+Alt+S`），成功有柔和提示音；**热键可在界面里随时修改** |
| 🖼️ | **网页界面**：查看 / 恢复 / 删除 / 批量删除备份，SSE **实时自动刷新** |
| ↩️ | **一键撤销上次恢复**：回档回错了也能一步退回 |
| 🛡️ | **恢复前自动安全快照** + **SHA-256 完整性校验**（损坏的备份拒绝恢复，可手动「校验」） |
| 🔎 | **搜索与类型筛选**、备份总数与占用统计、相对时间显示 |
| ✏️ | **内联编辑备注**：直接在记录上改，回车保存 |
| 📁 | **备份目标目录可自选**（原生文件夹选择器） |
| 🎯 | **多游戏档案**：一个工具管理多款游戏 |

> 恢复时**只需退回游戏标题界面**，无需完全退出游戏。

## 🚀 玩家快速上手（无需任何技术背景）

1. 到 [Releases](../../releases) 下载 `SaveGuardian.exe`，放到任意文件夹，**双击运行**。
2. 程序自动打开浏览器界面（若没弹出，手动访问 `http://127.0.0.1:8787`）。
3. 点 **「管理档案」→ 新增档案**：
   - **档案名称**：随便起，例如 `Project Plague`。
   - **要备份的文件夹**：填你想整体备份的那个文件夹（会连同里面所有文件一起备份）。例如某个存档槽位目录：
     `C:\Users\你的用户名\AppData\Local\Project_Plague\Saved\<数字>\GameSlots\maingame1`
   - **备份目标目录**：备份存到哪里，例如 `D:\MySaveBackups\ProjectPlague`。
   - 路径可点「选择…」用系统文件夹选择器挑，也可直接粘贴。
   - 工具**不自动识别槽位**——你指哪个文件夹，就整体备份哪个。需要备份多个槽位时，为每个各建一个档案。
4. 顶部选择要操作的 **游戏档案**。
5. **备份**：填个备注（可选）点 **「立即备份」**；或直接按 **热键 `Ctrl+Alt+S`**。
6. **恢复**：先在游戏里**退回标题界面** → 在列表里找到目标备份点 **「恢复」** 并确认。恢复前会自动为当前存档创建「安全快照」，回档回错了点 **「撤销上次恢复」** 即可退回。

> 提示：恢复会用备份内容覆盖你配置的那个文件夹。原始文件只做复制，**绝不会被移动或删除**。

## ⚙️ 设置与配置

点界面右上角 **「设置」** 可**修改全局热键**：聚焦输入框直接按下组合键即可录制，保存后立即尝试生效（个别情况下会提示重启后生效）。

首次运行会在 `.exe` 同目录生成 `saveguardian.config.json`：

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

- `source`：要整体备份的文件夹。（旧版本的 `sourceRoot` 键仍可读取，会自动迁移。）
- `hotkey`：支持 `Ctrl` / `Alt` / `Shift` / `Win` 组合 + 字母 / 数字 / `F1`~`F12`，例如 `Ctrl+Alt+F5`。
- `port`：网页界面端口，若被占用可改成别的。

## 📦 备份长什么样

```
<你的备份目标目录>/
  20260722-154108/            # 一次备份（时间戳）
    ...（你配置的文件夹里的全部文件）
    meta.json                 # 备注、时间、类型、每个文件的大小与 SHA-256
  20260722-160233/
    ...
```

`meta.json` 记录备注、创建时间、备份类型（`manual` 手动 / `hotkey` 热键 / `pre-restore` 安全快照）以及每个文件的大小与校验和。

## 🔒 安全说明

- 原始存档只做**只读复制**，工具不会移动或删除你的 `.sav`。
- 恢复前自动创建 `pre-restore` 安全快照，并**校验备份完整性**，损坏的备份会拒绝恢复。
- 恢复采用「暂存目录 + 原子替换 + 失败回滚」，尽量避免中途中断产生半个存档。
- 备份目标目录与源目录不允许互相嵌套，防止递归复制。
- 本地网页接口带**同源校验**，防止其它网页跨站触发备份 / 恢复 / 删除。

## 🧩 已知限制

- 全局热键、文件夹选择器与提示音为 **Windows** 功能；其它平台仍可用网页界面并手动粘贴路径。
- 若恢复时提示文件被占用，请确认已退回游戏标题界面后重试。

## 🛠️ 从源码构建（开发者）

需要 [Go](https://go.dev/dl/) 1.22+。

```bash
# 运行测试（含竞态检测）
go test -race ./...

# 交叉编译出 Windows exe（任意平台）
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o SaveGuardian.exe .

# 或使用脚本
./build.sh
```

前端资源（`webui/`）通过 Go `embed` 打包进 `.exe`，最终产物就是单个可执行文件。

<details>
<summary><b>项目结构</b></summary>

```
main.go                    程序入口：起服务、开浏览器、注册热键
internal/config/           配置与多游戏档案的读写
internal/backup/           备份 / 恢复 / 校验和 / 完整性校验（含单元测试）
internal/api/              HTTP REST 接口（含同源校验、操作串行化）+ SSE 实时推送
internal/hotkey/           全局热键（Windows syscall，支持运行时改键；其它平台空实现）
internal/folderpick/       系统文件夹选择器（Windows 专用）
internal/notify/           提示音（Windows 合成 WAV 播放，其它平台空实现）
webui/                     内嵌的单页图形界面（HTML/CSS/JS）
docs/superpowers/specs/    设计文档
```

</details>

## 🤝 反馈与贡献

欢迎提 Issue 或 PR：https://github.com/nanocm/save-guardian/issues

## 📄 许可证

本项目基于 [MIT License](LICENSE) 开源。
