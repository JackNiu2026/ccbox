# CCBox 文档入口

首次使用 CCBox，从安装与代理配置开始；已有环境遇到问题时，直接查故障恢复。公开安装包与版本验收范围见 [安装指南](guide/installation.md)，产品介绍见[项目主页](../docs/guide/installation.md)。

## 使用与反馈

面向日常使用的[官网使用文档](../docs/guide/index.md)提供分类目录、全文搜索和章节链接。内容源位于 [guide/](guide/index.md)，包括多账号、浏览器、手机、云端、设置与排障；下表保留安装、平台说明与维护资料的直接入口。

| 你要做什么 | 从这里开始 |
|---|---|
| 让自己的 Agent 下载、安装、配置 CCBox，跟踪任务并排查故障 | [Agent 使用指南](guide/agent.md) |
| 选择 macOS、Windows、Linux 安装包，配置代理并打开第一个项目 | [开始使用](getting-started.md) |
| 选择与主机同类的客机系统，了解原生工具、镜像与数据保留 | [选择客机系统](guide/guest-systems.md) |
| 查看快捷键、文件保存、关闭与退出行为 | [日常使用](getting-started.md#日常使用) |
| 在 Windows 上安装、使用和更新 | [Windows 支持](windows.md) |
| 在 Linux 上安装、使用和更新 | [Linux 支持](linux.md) |
| 升级应用、更新环境工具或卸载 | [升级与卸载](getting-started.md#升级与卸载) |
| 查看首个内测版本的能力与验收范围 | [0.1.0-alpha.1 发行说明](releases/0.1.0-alpha.1.md) |
| 排查下载、启动、联网与授权问题 | [常见问题与恢复](troubleshooting.md) |
| 导出故障日志并了解自动清理 | [导出诊断包](troubleshooting.md#导出诊断包) |
| 报告隔离或凭据安全问题 | [安全报告](../SECURITY.md) |

许可材料见 [LICENSE](../LICENSE)、[NOTICE](../NOTICE)和[第三方许可](../THIRD_PARTY_LICENSES)。

## 准备独立下载发布

GitHub 私有仓库内的 [上线准备说明](launch-readiness.md)列出当前测试产物、正式签名配置、官网构建流程和上线验收条件。

先使用对应平台的打包脚本生成 CCBox 正式包、`SHA256SUMS` 和对应源码。Windows 正式包由打包流程验证 Authenticode 签名，macOS 正式包须完成 Developer ID 签名与公证。Linux 发行者在打包时将 `CCBOX_DEB_MAINTAINER` 设为真实的维护者名称和邮箱。

为实际提供的平台准备发行说明，再从仓库根目录运行以下命令。示例只发布 Windows，也可传入 Linux 或 macOS 的正式包输出目录。

```sh
node www/scripts/stage-downloads.mjs --notes release-notes.md _output/releases/0.1.7/windows
npm --prefix www run build
node www/scripts/check-site.mjs
```

脚本校验包、校验清单和源码后，把发行文件放入 `www/public/downloads/<版本>/`，并生成官网使用的下载清单。已有版本目录不会被覆盖。构建前将 `SITE_URL` 设为自有 HTTPS 网站地址，校验通过后部署 `www/dist/`。未准备本地发行包时，官网平台入口继续指向安装指南。
