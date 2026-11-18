# CCBox 上线准备

源码和构建工件保存在私有仓库。对外官网部署构建后的静态目录，安装包、对应源码和校验文件使用官网自己的下载路径。

## GitHub 构建和官网

1. 在仓库的 Actions 变量中设置 `SITE_URL`，值为 CCBox 的 HTTPS 网站根地址。网站统计默认关闭。需要统计时填写自己的 `GA_MEASUREMENT_ID`。
2. 运行 **CCBox website checks** 可下载 `ccbox-website` 静态工件。缺少正式安装包时，官网的平台入口指向对应的安装指南。
3. 将选定的 `main` 提交打上 `ccbox-v<VERSION>` 标签。CCBox 0.1.7 使用 `ccbox-v0.1.7`。在 `main` 手动运行 **Build CCBox release candidates**，传入该标签所指向的完整提交 SHA 和目标平台。该流程只生成私有工件。同批重跑使用相同提交。若发布前必须改源码并保留应用版本号，在修订后的提交上新建 `ccbox-v0.1.7-build.1` 等顺序标签，并从该提交重建本批全部平台。
4. 在 `main` 运行 **Assemble CCBox website with verified release downloads**，传入同一个完整提交 SHA、本次上架的平台清单（如 `windows macos linux android`），以及成功的正式构建运行 ID。多个平台或运行 ID 用空格分隔。该流程从此提交构建官网，逐项核对平台、版本、构建提交、文件名和 SHA-256，缺包、多包或混用提交都会中止。通过后生成 `ccbox-website-release`。
5. 将工件解压后的内容部署到所选站点根目录。上线后逐项访问下载链接，核对文件名、状态码和实际下载文件的 SHA-256。公开下载不应要求 GitHub 登录。

## 必须由发布者配置

| 用途 | 需要的配置 |
| --- | --- |
| 官网 | 自有域名、DNS、HTTPS 和可供部署的静态主机或对象存储。仓库无需公开。 |
| Windows | 可信 Authenticode 签名。将证书与密码配置到 `cage-release` 环境的 `CAGE_WINDOWS_PFX_BASE64` 和 `CAGE_WINDOWS_PFX_PASSWORD`。使用 eSigner 时，按 Windows 工作流设置对应的环境变量和 Secrets。 |
| macOS | Developer ID Application 证书和 Apple 公证凭据。在 `cage-release` 设置 `CAGE_MACOS_P12_BASE64`、`CAGE_MACOS_P12_PASSWORD`、`CAGE_MACOS_SIGNING_IDENTITY`、`CAGE_MACOS_NOTARY_KEY_BASE64`、`CAGE_MACOS_NOTARY_KEY_ID`、`CAGE_MACOS_NOTARY_ISSUER_ID`。需要发布相应原生账户能力时，还须提供匹配的描述文件。 |
| Linux | 设置仓库变量 `CCBOX_DEB_MAINTAINER`，例如 `CCBox <support@your-domain.example>`，使用真实可收信地址。 |
| Android | 在 `cage-release` 设置 `CCBOX_ANDROID_KEYSTORE_BASE64` 和 `CCBOX_ANDROID_KEYSTORE_PASSWORD` Secrets。设置 `CCBOX_ANDROID_KEY_ALIAS`、`CCBOX_ANDROID_VERSION_CODE`、`CCBOX_ANDROID_CERT_SHA256` 变量。后续更新使用同一个发布密钥和递增的版本码。 |
| iPhone | Apple Developer 团队、团队拥有的 Bundle ID、签名证书及对应分发描述文件。标准版和 Basic 版的当前仓库工件仅用于模拟器。真机开发包见[移动端构建说明](../mobile/README.md)。App Store 和 TestFlight 还需要 App Store Connect 配置。 |
| 手机中继 | 0.1.8 不提供官方中继：默认中继为空，手机配对需用户在「设置 → 远程连接」填写自建中继。部署 CCBox 官方中继后，将 `internal/mobilelink/link.go` 的 `OfficialRelay` 设为其 HTTPS 地址。手机应用暂不随本版发布。 [中继部署说明](../deploy/relay/README.md) |
| 支持与品牌 | 确认对外安全反馈邮箱、品牌名使用权和发行主体。MIT 版权及第三方许可继续随包提供。 |

对外显示名为 `CCBox`，下载与项目主页为 `https://ccbox.org/`。Windows 安装器使用专属安装标记和卸载注册项，macOS 使用 `org.ccbox.desktop`，Linux 软件包和命令使用 `ccbox`。新桌面包默认使用 `~/.ccbox`，可用 `CCBOX_CONFIG` 明确指定单独的配置文件。旧预览用户应先备份，使用独立目录安装，确认旧资料归属后再选择需要迁移的内容。Android 应用标识为 `org.ccbox.android`，iOS 标准版和 Basic 分别使用 `org.ccbox.ios` 和 `org.ccbox.ios.basic`。移动端预览用户需单独安装新应用并重新配对。

证书、密码和私钥应通过 GitHub Secrets 配置，勿写入源码、Issue 或聊天。

## 0.1.7 验收记录

- Windows x64 测试 EXE 已通过 Server 2025 安装、覆盖安装和卸载。Server 2022 的测试停在后台关闭确认，虚拟机启动也需实机验证。
- macOS Apple Silicon 测试 DMG 可挂载，内部应用通过本地签名核验。正式 DMG 待 Developer ID 签名和公证。
- Linux x64 DEB 已通过 Debian 12 和 Ubuntu 24.04 兼容性测试。现有 DEB 的维护邮箱为占位地址。
- Android arm64 测试 APK 已通过签名、图标和对齐检查，发布密钥与真机更新验收待完成。
- iOS 标准版和 Basic 版已生成模拟器包。两个真机 IPA 待签名、导出和设备验收。
- 桌面轻量版的虚拟机依赖检查仍未通过。

公开官网应只列出已经验收且实际可下载的版本。对于缺少正式包的平台，继续显示安装指南。
