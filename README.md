<p align="center"><img src="assets/logo-kit/horizontal.svg" alt="Repo Watch" width="420"></p>

# Repo Watch

Repo Watch 是一个 Windows 桌面仓库监控面板，使用 [MyGo](https://github.com/egoist/mygo) 构建。它集中显示 GitHub、Codeberg 和 Gitea 仓库的动态，支持公开和私有仓库。

## 功能

- 显示 Stars、Forks、开放 Issue 数、最新发行版和仓库更新时间。
- 后台每 10 分钟刷新，也可手动刷新；首次读取建立基线，之后检测到变化时发送桌面通知。
- 关闭窗口后隐藏到托盘并继续运行；支持窗口置顶、单实例和托盘退出。
- 私有仓库令牌按仓库保存在当前 Windows 用户的凭据管理器中，仅通过 HTTPS 发送，且不会随重定向发送。

## 安装和使用

当前仓库尚未提供正式发行包。请在 Windows 上从源码构建，或使用未来发布的官方 Release。需要 [Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)。

运行 `RepoWatch.exe`，点击「＋」并粘贴仓库首页链接，或选择平台并输入 owner 和仓库名。自建 Gitea 需填写实例根地址（包含可能的路径前缀）。私有仓库可添加时输入个人访问令牌，或点击仓库卡片的钥匙按钮设置、替换、清除令牌。删除仓库也会删除该仓库的本机令牌。请勿将令牌提交到代码仓库。

GitHub 细粒度令牌需选择目标私有仓库并授予 **Metadata: read** 和 **Contents: read**（读取发行版）；组织仓库可能还需要组织授权。Gitea/Codeberg 令牌需具有目标仓库的读取权限。出现 401、403 或 404 时，请检查地址、令牌、权限和平台限流。

关闭窗口后，可从托盘菜单恢复窗口或彻底退出；再次运行程序也可以唤回窗口。退出进程、注销或关机后不会继续监控，程序不会自动开机启动。通知显示还取决于 Windows 通知设置。

仓库列表和最近快照保存在 `%APPDATA%\RepoWatch\monitor.json`，可能包含私有仓库名称和描述；首次运行会迁移旧版 WebView 的仓库数据并保留旧数据。应用不提供历史趋势图。GitHub 的开放 Issue 数可能包含 Pull Request；Gitea 和 Codeberg 的时间含义取决于平台返回的数据。

## 从源码构建

需要 Windows、Go 1.27.3 或更高版本以及 WebView2 Runtime。前端通过 Go embed 打包，无需 Node.js。PowerShell：

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
