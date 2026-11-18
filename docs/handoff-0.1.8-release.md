# CCBox 0.1.8 发布交接说明

给接手执行发布的工程师或 AI 助手（如 Codex）。按顺序完成第 1–6 步，每步都有验收标准，达不到不要进入下一步。

## 当前状态（2026-11-18）

| 项目 | 状态 |
|---|---|
| 代码 | 已合并到 `main`（合并提交 `a943956`，PR #6）。`VERSION` 为 `0.1.8`，三平台 CI 全部通过。 |
| 在线账户 | 已实现用户名和密码注册、登录（第一版不做邮箱验证），管理后台已有用户管理。已用真实服务、真实后台和真实客户端做过端到端验证。 |
| 线上服务器 | `ccbox.org`（`189.24.67.205`，Caddy）仍运行**旧版**授权服务：`POST https://ccbox.org/v1/auth/login` 返回 404。新版尚未部署。 |
| 官网 | `https://ccbox.org/` 目前提供手工上传的 0.1.7 下载（`/downloads/0.1.7/…`，含 Android 调试包）。 |
| 签名 | 没有 Windows 和 Apple 证书。本次 Windows、macOS 以**未签名预览版**发布，Linux 为正式版。 |
| 手机端 | 本次不发布，默认中继为空。 |
| 尚未完成 | 部署授权服务 → 设置公钥变量 → 打 tag → 三平台打包 → 组装官网 → 部署官网。 |

## 安全要求

- root 密码曾在聊天中以明文出现，**第 1 步开始前先修改**（`passwd`）。条件允许时改用 SSH 密钥登录。
- 不要把任何密码、私钥（`signing.key`）、后台密码文件内容写进仓库、日志、PR、Issue 或聊天。
- 公钥（43 个字符）不是秘密，会打进客户端，可以写进仓库变量。
- 授权服务的状态目录包含签名私钥和账本。改动前先备份；**不得删除或重新生成已有的 `signing.key`**，否则所有已激活用户都会失效。

---

## 第 1 步：在服务器上部署新版授权服务

授权服务（`cmd/ccbox-license`）负责兑换激活码、在线账户（`/v1/auth/*`、`/v1/account*`）和管理后台（`/admin/license/`）。0.1.8 客户端注册和登录都依赖它。

### 1.1 构建程序

在任意装有 Go 的机器上，于仓库根目录、`main` 最新提交执行：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ccbox-license-linux-amd64 ./cmd/ccbox-license
```

服务器是 ARM 时把 `amd64` 换成 `arm64`。可在服务器上用 `uname -m` 确认：`x86_64` 对应 amd64，`aarch64` 对应 arm64。

### 1.2 先弄清楚旧服务怎么运行

登录服务器后执行，记录结果：

```bash
systemctl status ccbox-license --no-pager
```

```bash
ss -ltnp | grep 8791
```

```bash
docker ps --format '{{.Names}} {{.Image}} {{.Ports}}' 2>/dev/null
```

```bash
grep -n -B2 -A6 'redeem\|8791\|admin/license' /etc/caddy/Caddyfile
```

- **情况 A**：旧服务是 systemd 服务 `ccbox-license`，或者 8791 端口没有被占用。直接用 1.3 的脚本。
- **情况 B**：8791 由 Docker 容器或其他进程占用。脚本会停下并打印报告，不做任何修改。此时按 1.4 处理。

### 1.3 情况 A：运行安装脚本

先备份状态目录（路径以 systemd 单元里 `ExecStart` 的参数为准，默认 `/var/lib/ccbox-license`）：

```bash
tar -czf /root/ccbox-license-backup-$(date +%Y%m%d%H%M%S).tgz /var/lib/ccbox-license /etc/ccbox-license 2>/dev/null; ls -la /root/ccbox-license-backup-*
```

把程序和 `deploy/license/install-ccbox-license.sh` 传到服务器的 `/root/`，然后执行：

```bash
bash /root/install-ccbox-license.sh /root/ccbox-license-linux-amd64
```

脚本的行为：

- **已有 systemd 服务**：从现有单元读出状态目录、密钥路径和运行账户，只替换 `/usr/local/bin/ccbox-license` 并重启。服务文件、管理后台密码和签名密钥都不动。
- **首次安装**：创建系统账户 `ccbox-license`、`/var/lib/ccbox-license`、签名密钥、`/etc/ccbox-license/admin-password`，以及 systemd 单元和 `admin.conf`。
- **自检**：检查 `/healthz`；`/v1/auth/login` 必须返回 401。
- **改 Caddy**：如果 Caddyfile 里已经转发 `/v1/redeem`，会备份 Caddyfile，把 `/v1/auth/*`、`/v1/account`、`/v1/account/*` 加到同一转发，再执行 `caddy validate` 并 reload。失败时自动还原。
- **输出**：打印公钥、后台地址、新生成的后台密码（只在首次生成时显示），以及外网检查结果。

### 1.4 情况 B：Docker 或其他方式运行

不要用脚本。目标是：用新版程序替换旧版，**继续使用原来的状态目录和 `signing.key`**，并让 Caddy 转发新路径。

1. 找到旧服务使用的状态目录和密钥文件（容器挂载卷：`docker inspect <容器>`，看 `Mounts` 和启动参数）。先备份它们。
2. 选一种方式：
   - **方式 1：用新程序重建容器。** 启动命令保持 `ccbox-license serve <状态目录> <密钥文件>`。如果反代通过容器网络访问，设置环境变量 `CCBOX_LICENSE_CONTAINER=1`。
   - **方式 2：改用 systemd。** 停掉并禁用旧容器，把状态目录和密钥复制到 `/var/lib/ccbox-license/`（属主 `ccbox-license`，权限 700/600），然后按情况 A 运行脚本。
3. 管理后台需要环境变量 `CCBOX_LICENSE_ADMIN_PASSWORD_FILE`（至少 24 个字符的密码文件，只有服务账户可读）和 `CCBOX_LICENSE_ADMIN_ORIGINS=https://ccbox.org`。详见 `deploy/license/README.md`。
4. Caddy 需要把以下路径转发给授权服务（与旧的 `/v1/redeem` 放在同一站点块）：

```caddyfile
@ccbox_license path /v1/redeem /v1/auth/* /v1/account /v1/account/* /admin/license*
handle @ccbox_license {
    reverse_proxy 127.0.0.1:8791
}
```

### 1.5 第 1 步验收

以下每一项都要满足：

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST -H 'Content-Type: application/json' -d '{}' https://ccbox.org/v1/auth/login
```

- 输出 `401`（不是 404，也不是 502）。
- `https://ccbox.org/admin/license/` 能打开，用 `admin` 和后台密码能登录。
- 后台「激活码」页能生成 1 枚测试码。生成后在「激活码」页停用它，避免流入市场。
- 旧数据仍在：后台「概览」的累计发放数、「设备」页的已激活设备与升级前一致。
- 记下脚本打印的公钥（43 个字符，只含 `A–Z a–z 0–9 - _`）。如果用的是方式 1，没有脚本输出，在服务器上执行下面的命令，用私钥算出公钥：

```bash
seed=$(tr -d '\n\r' < /var/lib/ccbox-license/signing.key | tr '_-' '/+' | awk '{while(length($0)%4)$0=$0"=";print}' | base64 -d | od -An -v -tx1 | tr -d ' \n'); printf '302e020100300506032b657004220420%s' "$seed" | sed 's/../\\x&/g' | xargs -0 printf '%b' | openssl pkey -inform DER -pubout -outform DER | tail -c 32 | base64 | tr '+/' '-_' | tr -d '=\n'; echo
```

（把密钥路径换成实际路径。）

---

## 第 2 步：设置仓库变量

GitHub 仓库 → Settings → Secrets and variables → Actions → **Variables**：

| 变量 | 值 |
|---|---|
| `CCBOX_LICENSE_PUBLIC_KEY` | 第 1 步得到的公钥 |
| `SITE_URL` | `https://ccbox.org/`（组装官网时要求严格等于这个地址） |
| `CCBOX_DEB_MAINTAINER` | 可选。未设置时默认 `CCBox <niuchen160@gmail.com>` |

用 `gh` 设置：

```bash
gh variable set CCBOX_LICENSE_PUBLIC_KEY --repo JackNiu2026/ccbox --body '<公钥>'
```

```bash
gh variable set SITE_URL --repo JackNiu2026/ccbox --body 'https://ccbox.org/'
```

验收：`gh variable list --repo JackNiu2026/ccbox` 中有上述变量，公钥与服务器一致。**公钥必须和服务器上的私钥配对**，否则 0.1.8 客户端无法激活。

---

## 第 3 步：在 main 上打 tag

```bash
git fetch origin main --tags
```

```bash
git checkout --detach origin/main && cat VERSION && git log --oneline -1
```

确认 `VERSION` 为 `0.1.8` 后：

```bash
git tag -a ccbox-v0.1.8 -m "CCBox 0.1.8" && git push origin ccbox-v0.1.8
```

```bash
SHA=$(git rev-parse ccbox-v0.1.8^{commit}) && echo $SHA && node scripts/release-version.mjs source $SHA
```

最后一条命令必须无报错。下文的 `<SHA>` 都指这个完整的 40 位提交号。

---

## 第 4 步：构建三平台安装包

工作流 **Build CCBox release candidates**（`.github/workflows/release-platforms.yml`）必须在 `main` 上触发。**不要选 `all`**，因为它包含本次不发布的 Android。三个平台分别触发：

```bash
gh workflow run release-platforms.yml --repo JackNiu2026/ccbox --ref main -f ref=<SHA> -f platform=windows -f unsigned_preview=true
```

```bash
gh workflow run release-platforms.yml --repo JackNiu2026/ccbox --ref main -f ref=<SHA> -f platform=macos -f unsigned_preview=true
```

```bash
gh workflow run release-platforms.yml --repo JackNiu2026/ccbox --ref main -f ref=<SHA> -f platform=linux -f unsigned_preview=true
```

注意：

- 三次运行属于同一并发组，会依次排队执行。全部完成大约需要 1–1.5 小时。
- 构建任务使用 GitHub Environment `cage-release`。如果该环境设置了审批人，需要在运行页面点 **Review deployments → Approve**。
- `validate` 步骤会检查：tag 与 `VERSION` 一致；提交在 main 上；公钥变量格式正确；不再使用遗留中继。失败时按报错修正，不要绕过。
- `unsigned_preview=true` 只影响 Windows 和 macOS，Linux 始终是正式包。
- 偶发失败的处理：
  - `linux / ubuntu-guest` 因 Snap Store 网络错误（connection reset/refused、nonce 超时）失败：等 20–30 分钟后 `gh run rerun <run-id> --failed`。
  - Windows NSIS 测试超时：看日志里的诊断输出，确认是性能问题再重跑。
  - 其他失败要读日志定位，不要跳过、禁用或删除测试。

验收：三次运行都为 success。记下三个运行 ID：

```bash
gh run list --repo JackNiu2026/ccbox --workflow release-platforms.yml --limit 5
```

---

## 第 5 步：组装官网

```bash
gh workflow run website-release.yml --repo JackNiu2026/ccbox --ref main -f ref=<SHA> -f platforms='windows macos linux' -f release_runs='<windows运行ID> <macos运行ID> <linux运行ID>'
```

该流程核对每个包的平台、版本、构建提交、文件名和 SHA-256，并构建网站。Windows 和 macOS 会显示为「预览版」。产物是 artifact `ccbox-website-release`（即 `www/dist/`）。

```bash
gh run download <本次运行ID> --repo JackNiu2026/ccbox --name ccbox-website-release --dir ./ccbox-website-0.1.8
```

验收：

- 目录中存在 `downloads/0.1.8/`，内含 Windows `.exe`、macOS `.dmg`、Linux `.deb`、各自的源码包、`SHA256SUMS` 和 `release-notes.txt`。
- 在该目录内执行 `sha256sum -c downloads/0.1.8/SHA256SUMS`（Mac 上用 `shasum -a 256 -c`），全部 OK。

---

## 第 6 步：部署官网并验收

1. 在服务器上找到网站根目录：`grep -n 'root\|file_server' /etc/caddy/Caddyfile`。
2. 备份当前网站目录：

```bash
tar -czf /root/ccbox-site-backup-$(date +%Y%m%d%H%M%S).tgz -C <网站根目录> .
```

3. 上传新站点。**不要删除旧的 `downloads/0.1.7/`**，以免老用户的下载链接失效，所以不要用 `rsync --delete`：

```bash
rsync -av ./ccbox-website-0.1.8/ root@189.24.67.205:<网站根目录>/
```

4. 如果 Caddy 对静态文件有缓存设置，执行 `systemctl reload caddy`。

上线验收：

- `https://ccbox.org/` 下载区显示 0.1.8：Windows、macOS 标注「预览版」，Linux 为正式版。三个链接都能下载，SHA-256 与 `SHA256SUMS` 一致。Android 入口指向安装指南。
- 在真实电脑上安装测试：
  - **Windows**：SmartScreen 提示时点「更多信息 → 仍要运行」。
  - **macOS**：先双击一次，再到「系统设置 → 隐私与安全性 → 仍要打开」。
  - **Linux**：`sudo apt install ./CCBox-0.1.8-linux-amd64.deb`。
- 在客户端完成一次完整流程：
  1. 注册新账户。
  2. 在后台为该账户生成 1 枚激活码。
  3. 在客户端「设置 → 授权」兑换，状态变为有效，到期时间为 30 天后。
  4. 在后台「用户」页能看到该账户及其设备。
  5. 测完后在后台停用该测试账户。

---

## 相关文档与代码

| 内容 | 位置 |
|---|---|
| 授权服务、后台与在线账户 | `deploy/license/README.md`、`cmd/ccbox-license/`、`internal/license/` |
| 服务器安装脚本 | `deploy/license/install-ccbox-license.sh` |
| 发布流程总览、预览版说明 | `docs/launch-readiness.md` |
| 用户侧账户说明 | `docs/guide/accounts.md`、`docs/guide/installation.md` |
| 构建工作流 | `.github/workflows/release-platforms.yml`、`windows-build.yml`、`macos-build.yml`、`linux-build.yml`、`website-release.yml` |

## 已知限制

- 预览版没有应用内自动更新。取得证书后改为正式签名发布，用户需要重新下载安装。
- 停用账户或解绑设备后，客户端会立即被登出，但已下发的离线凭证最长 7 天后才失效，这是设计如此。
- 忘记密码不通过邮箱找回：由客服在后台「用户 → 详情」发放一次性重置码（24 小时有效）。
- 每个账户默认最多 2 台设备，可用服务端环境变量 `CCBOX_LICENSE_MAX_DEVICES`（1–20）调整。
