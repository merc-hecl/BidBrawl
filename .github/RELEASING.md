# 发布 BidBrawl

版本唯一来源是根目录 `mygo.json` 的 `version`。工作流为 [Build and release](workflows/desktop.yml)，只在推送 `v数字.数字.数字` 正式版本标签时发布；标签必须与版本一致。不需要 PAT 或额外仓库密钥，只有发布 job 使用 GitHub 自带的 `GITHUB_TOKEN` 获取 `contents: write`。

## 发布步骤

1. 修改 `mygo.json` 中的版本。首次可使用当前的 `0.1.0`，以后每次发布递增版本。
2. 提交版本与功能改动，确认工作区干净。提交遵循 Conventional Commits。
3. 推送分支并等待 `Build and release` 通过。普通分支、PR 和手动运行均只验证和上传 Actions 附件，不创建 Release。
4. 在已验证的提交上创建标签，再推送该标签。例如首次发布：

   ```sh
   git tag -a v0.1.0 -m "BidBrawl v0.1.0"
   git push origin v0.1.0
   ```

   第一条只创建本地标签；第二条触发远程构建和自动发布。发布配置必须已在标签指向的提交内。无需先在网页创建 Release。

5. 在 Actions 中确认五个平台全部成功，并核对 Release 的十个安装包 / CLI 压缩包及 `SHA256SUMS`。GitHub 自动附带的 Source code ZIP / tar.gz 是源码，不是可执行程序。

## 产物与检查

| 平台 | 桌面与 CLI 安装包 | 独立 CLI |
| --- | --- | --- |
| Windows x64 | Setup EXE | ZIP |
| Linux x64 / ARM64 | DEB | tar.gz |
| macOS Intel / Apple Silicon | PKG | tar.gz |

各平台在原生 GitHub runner 上执行 Go 测试、`go vet`、现有桌面构建器；WebUI 测试和发布脚本测试先执行。独立 CLI 不需要桌面运行时，Unix 压缩包保留可执行权限。安装包和 CLI 压缩包携带项目及第三方许可。二进制构建无需 Node.js；Node.js 和锁定的 changelogen 仅用于开发测试和发布工具。

当前 Windows 安装包没有 Authenticode 签名；macOS 只有 MyGo 的本地签名，没有 Developer ID 签名和公证，系统可能阻止安装。没有配置自动更新源。发布构建检查不等同于各平台真实安装验收或真实 Agent 对战验收。

## Release 说明

参考 `dsh-plugin-stickers` 的 changelogen 配置：英文分类标题、Conventional Commits 条目、scope、提交 / PR 链接、Breaking Changes、Contributors。首次读取完整历史；后续只读取当前标签可达的上一正式版本以来的提交。未合并分支、预发布标签和当前版本之后的提交不会混入。贡献者包含作者和 `Co-authored-by`，去重、排除机器人、不输出邮箱。另附各平台下载表、校验文件和安装限制。

本地预览已存在的标签（只写文件，不修改版本或 Git 引用）：

```sh
npm ci --prefix .github/release --ignore-scripts
npm test --prefix .github/release
node .github/release/notes.mjs v0.1.0 build/release-notes.md
```

## 失败恢复

全部构建成功并校验产物集合后，工作流才创建草稿、上传文件并公开 Release。上传失败时保留草稿，重跑失败 job 可补齐并发布。已公开的版本不会被自动覆盖；修改代码后应增加版本并使用新标签。仅在网页修改 Release 或推送分支不会触发发布。

仓库可见性不由工作流修改。将仓库改为 Public 是单独的 GitHub 设置操作，会公开已有 Git 历史；本配置不执行该操作。
