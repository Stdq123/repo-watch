# 参与贡献

感谢你帮助改进 Repo Watch。请使用 [GitHub 主仓库](https://github.com/Stdq123/repo-watch)的 Issue 和 Pull Request；较大的功能或行为变更建议先通过 Issue 讨论。Gitea 和 Codeberg 是源码同步副本，更新不是实时自动同步的。

## 开发环境

- Windows、Go 1.27.3 或更高版本、WebView2 Runtime。
- 克隆仓库后执行 `go test ./...`；用 `go run .` 开发，或运行 `./build.ps1` 构建 Windows GUI 程序。
- 提交前运行 `gofmt -w` 格式化修改过的 Go 文件，再运行 `go test ./...` 和 `go vet ./...`。
- 请为行为变更提供适当的测试，描述验证方法与平台。UI 或托盘变更请说明手动验证情况。

## 提交变更

1. Fork 主仓库并创建功能分支，保持改动范围清晰。
2. 在 Pull Request 中描述问题、解决方案、测试结果及可能的影响；界面变更可附截图。
3. 确保有权按本项目的 MIT 许可证提交你的贡献，并保留第三方材料的许可与署名。
4. 不要提交访问令牌、真实私有仓库信息、用户配置、构建产物或其他敏感数据。

漏洞请勿公开提交 Issue，改用 [安全报告流程](SECURITY.md)。参与讨论请遵守 [行为准则](CODE_OF_CONDUCT.md)。
