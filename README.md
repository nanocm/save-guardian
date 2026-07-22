# SaveGuardian · 存档守护

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows-0078D6.svg)](https://github.com/nanocm/save-guardian/releases)
[![Made with Go](https://img.shields.io/badge/made%20with-Go-00ADD8.svg)](https://go.dev)
[![Release](https://img.shields.io/github/v/release/nanocm/save-guardian?include_prereleases&sort=semver)](https://github.com/nanocm/save-guardian/releases)
[![Stars](https://img.shields.io/github/stars/nanocm/save-guardian?style=social)](https://github.com/nanocm/save-guardian)

> 项目地址：https://github.com/nanocm/save-guardian

一个通用的**手动存档备份 / 恢复**小工具，专为魂类（Souls-like）等“死亡自动存档 + 死亡惩罚”的游戏设计。

在关键节点手动备份整个存档槽位，遇到不利结果时一键恢复到之前的状态，从而规避死亡惩罚。

- ✅ **全局热键**一键备份（默认 `Ctrl+Alt+S`），成功有柔和提示音
- ✅ **网页图形界面**：查看、恢复、删除备份，**列表实时自动刷新**
- ✅ **内联编辑备注**：直接在记录上改，回车保存
- ✅ **批量选择删除**：点击整条记录即可选中，一键删除多个
- ✅ 以**整个文件夹**为单位备份（通用于各种存档结构）
- ✅ 恢复时**只需退回游戏标题界面**，无需完全退出游戏
- ✅ **备份目标目录可自选**（原生文件夹选择器）
- ✅ **多游戏档案**：一个工具管理多款游戏
- ✅ 恢复前**自动安全快照**，误操作也能找回
- ✅ 单个 `.exe`，**免安装、双击即用**

---

## 给玩家：如何使用（无需任何技术背景）

1. 到 [Releases](../../releases) 下载 `SaveGuardian.exe`，放到任意文件夹，双击运行。
2. 程序会自动打开浏览器界面（如果没弹出，手动访问 `http://127.0.0.1:8787`）。
3. 点击 **「管理档案」→ 新增档案**：
   - **档案名称**：随便起，例如 `Project Plague`。
   - **要备份的文件夹**：直接填你想整体备份的那个文件夹（工具会把它连同里面所有文件一起备份）。例如某个存档槽位目录：
     `C:\Users\你的用户名\AppData\Local\Project_Plague\Saved\<数字>\GameSlots\maingame1`
   - **备份目标目录**：备份存到哪里，例如 `D:\MySaveBackups\ProjectPlague`。
   - 路径可以点「选择…」用系统文件夹选择器挑，也可以直接粘贴。
   - 不同游戏的存档结构各不相同，所以工具**不自动识别槽位**——你指哪个文件夹，就整体备份哪个文件夹。需要备份多个槽位时，为每个槽位各建一个档案即可。
4. 在顶部选择要操作的 **游戏档案**。
5. 想备份时：
   - 在界面填个备注（可选），点 **「立即备份」**；或
   - 直接按 **热键 `Ctrl+Alt+S`** 快速备份当前档案的文件夹。
6. 想恢复时：
   - **先在游戏里退回到标题界面**（不用完全退出游戏）。
   - 在备份列表里找到目标备份，点 **「恢复」** 并确认。
   - 恢复前工具会自动为“当前文件夹”做一次安全快照（标记为“安全快照”），万一恢复错了还能再切回去。

> 提示：恢复会用备份内容覆盖你配置的那个文件夹。原始文件只做复制，绝不会被移动或删除。

---

## 备份文件长什么样

```
<你的备份目标目录>/
  20260722-154108/            # 一次备份（时间戳）
    ...（你配置的文件夹里的全部文件）
    meta.json                 # 备注、时间、类型、校验和
  20260722-160233/
    ...
```

`meta.json` 里记录了备注、创建时间、备份类型（手动 / 热键 / 安全快照）以及每个文件的大小与 SHA-256 校验和。

---

## 配置文件

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

- `source`：要整体备份的文件夹。（旧版本用的 `sourceRoot` 键仍可读取，会自动迁移。）

- `hotkey`：支持 `Ctrl` / `Alt` / `Shift` / `Win` 组合 + 字母 / 数字 / `F1`~`F12` 等，例如 `Ctrl+Alt+F5`。
- `port`：网页界面端口，若被占用可改成别的。

---

## 给开发者：从源码构建

需要 [Go](https://go.dev/dl/) 1.21+。

```bash
# 运行测试
go test ./...

# 生成 Windows exe（在任意平台交叉编译）
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o SaveGuardian.exe .

# 或使用脚本
./build.sh
```

前端资源（`webui/`）通过 Go `embed` 打包进 `.exe`，最终产物就是单个可执行文件。

### 项目结构

```
main.go                    程序入口：起服务、开浏览器、注册热键
internal/config/           配置与多游戏档案的读写
internal/backup/           备份 / 恢复 / 校验和 核心逻辑（含单元测试）
internal/api/              HTTP REST 接口 + SSE 实时推送
internal/hotkey/           全局热键（Windows 用 syscall，其它平台空实现）
internal/folderpick/       系统文件夹选择器（Windows 专用）
internal/notify/           提示音（Windows 合成 WAV 播放，其它平台空实现）
webui/                     内嵌的单页图形界面（HTML/CSS/JS）
docs/superpowers/specs/    设计文档
```

---

## 安全说明

- 原始存档只做**只读复制**，工具不会移动或删除你的 `.sav`。
- 恢复前自动创建 `pre-restore` 安全快照。
- 恢复只影响所选文件夹，采用“暂存目录 + 原子替换”，尽量避免中途中断产生半个存档。
- 备份目标目录与源目录不允许互相嵌套，防止递归复制。

## 已知限制

- 全局热键、文件夹选择器与提示音为 **Windows** 功能；其它平台仍可用网页界面并手动粘贴路径。
- 若恢复时提示文件被占用，请确认已退回游戏标题界面后重试。

## 反馈与贡献

欢迎提 Issue 或 PR：https://github.com/nanocm/save-guardian/issues

## 许可证

本项目基于 [MIT License](LICENSE) 开源。
