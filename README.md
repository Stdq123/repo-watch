<p align="center"><img src="assets/logo-kit/horizontal.svg" alt="Repo Watch" width="420"></p>

# Repo Watch

Repo Watch 是一个使用 Windows 原生控件构建的桌面仓库监控面板。它集中显示 GitHub、Codeberg 和 Gitea 仓库的动态，支持公开和私有仓库。

## 功能

- 显示 Stars、Forks、关注数、语言、许可证、创建时间、最新发行版和仓库更新时间（以平台实际返回的信息为准）；同时监控开放 Issue 数的变化。
- 为仓库添加多个自定义标签，并按标签筛选、搜索及排序仓库。
- 后台每 10 分钟刷新，也可手动刷新；首次读取建立基线，之后检测到变化时发送桌面通知。
- 关闭窗口后隐藏到托盘并继续运行；支持窗口置顶、单实例和托盘退出。
- 私有仓库令牌按仓库保存在当前 Windows 用户的凭据管理器中，仅通过 HTTPS 发送，且不会随重定向发送。

## 安装和使用

当前仓库尚未提供正式发行包。请在 Windows 上从源码构建，或使用未来发布的官方 Release。

运行 `RepoWatch.exe`，点击「添加仓库」并粘贴仓库首页链接，或选择平台并输入 owner 和仓库名。自建 Gitea 需填写实例根地址（包含可能的路径前缀）。私有仓库可添加时输入个人访问令牌，或选中仓库并通过「编辑仓库」设置、替换、清除令牌。删除仓库也会删除该仓库的本机令牌。请勿将令牌提交到代码仓库。

添加仓库时可填写标签，多个标签用英文或中文逗号分隔。选中仓库后通过右侧「编辑仓库」编辑或清空标签；使用左侧标签列表可筛选仓库，也可以搜索仓库名、平台、标签或描述。标签保存在本机监控数据中。

「在终端打开」会在关联的本地目录中启动 PowerShell。首次使用时选择仓库目录，也可通过「编辑仓库」重新关联；此操作不会克隆或修改仓库。本地目录保存在同一份监控数据中。

GitHub 细粒度令牌需选择目标私有仓库并授予 **Metadata: read** 和 **Contents: read**（读取发行版）；组织仓库可能还需要组织授权。Gitea/Codeberg 令牌需具有目标仓库的读取权限。出现 401、403 或 404 时，请检查地址、令牌、权限和平台限流。

关闭窗口后，可从托盘菜单恢复窗口或彻底退出；再次运行程序也可以唤回窗口。退出进程、注销或关机后不会继续监控，程序不会自动开机启动。通知显示还取决于 Windows 通知设置。

仓库列表和最近快照保存在 `%APPDATA%\RepoWatch\monitor.json`，可能包含私有仓库名称和描述。原生版直接读取该文件；如果数据只保存在尚未迁移的旧版 WebView 中，请先运行旧版完成迁移，再从托盘退出旧版并启动原生版。应用不提供历史趋势图。GitHub 的开放 Issue 数可能包含 Pull Request；Gitea 和 Codeberg 的时间含义取决于平台返回的数据。

## 从源码构建

需要 Windows 和 Go 1.27.3 或更高版本。原生界面无需 WebView2 Runtime 或 Node.js。PowerShell：

```powershell
git clone https://github.com/Stdq123/repo-watch.git
cd repo-watch
go test ./...
.\build.ps1
.\RepoWatch.exe
```

构建图形程序不会额外打开终端窗口。若程序仍在后台运行，请先从托盘退出再构建。当前项目以 Windows 为主要运行平台，令牌持久化仅在 Windows 实现；其他平台的运行体验未作为发布目标验证。

## 仓库与参与

- 主仓库：[GitHub](https://github.com/Stdq123/repo-watch)
- 自动同步副本：[Gitea](https://gitea.com/puremixai/repo-watch) · [Codeberg](https://codeberg.org/puremixai/repo-watch)。GitHub Actions 在主分支更新后及每日定时同步 `main` 和标签；仓库议题、发行包和其他分支不在同步范围内。
- Bug 和功能建议请在主仓库提交 [Issue](https://github.com/Stdq123/repo-watch/issues)；贡献方式见 [CONTRIBUTING.md](CONTRIBUTING.md)。
- 安全问题请按 [SECURITY.md](SECURITY.md) 私下报告。社区行为准则见 [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)。

代码使用 [MIT License](LICENSE)。标志中使用的 DejaVu 字体相关许可见 [字体许可](assets/logo-kit/LICENSE-DejaVu.txt)。

界面图标使用 [Lucide 0.468.0](https://github.com/lucide-icons/lucide/tree/0.468.0)（[ISC 许可](assets/icons/lucide/LICENSE)，部分图标源自 [Feather](assets/icons/lucide/FEATHER-LICENSE)）及 [Simple Icons 13.21.0](https://github.com/simple-icons/simple-icons/tree/13.21.0)（[许可](assets/icons/simple-icons/LICENSE.md)、[声明](assets/icons/simple-icons/DISCLAIMER.md)）。图标资源随程序打包，运行时不联网；品牌标识的商标权属于各自权利人。

英文和数字使用 [Inter 4.1](https://github.com/rsms/inter/tree/v4.1)，遵循 [SIL Open Font License 1.1](assets/fonts/inter/LICENSE.txt)。字体随程序打包并仅在当前进程中加载，不会安装到系统；Inter 不包含中文字形，中文使用 Windows 自带的 Microsoft YaHei UI。加载失败时界面使用系统字体。
