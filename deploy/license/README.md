# CCBox 激活服务

客户端授权按设备发放，每个激活码增加 30 天。提前续期从当前到期时间起算，过期后从兑换时间起算。首次激活和续期需要联网。客户端保存签名授权，在有效期内可离线启动环境。

## 部署和发码

构建授权服务并安装到服务器。使用专用系统账户运行服务及下面的管理命令，确保该账户独占密钥和状态目录。

```sh
go build -trimpath -o ccbox-license ./cmd/ccbox-license
install -d -m 700 /var/lib/ccbox-license
ccbox-license init-key /var/lib/ccbox-license/signing.key
ccbox-license serve /var/lib/ccbox-license/state /var/lib/ccbox-license/signing.key
```

`init-key` 输出构建客户端所需的公钥。将私钥和状态目录（含账本与操作日志）备份到受限位置。恢复时同时恢复签名密钥和当前账本，回滚旧账本可能使已经使用的码再次可用。

将 `https://ccbox.org/v1/redeem` 反向代理到授权服务。宿主机部署默认监听 `127.0.0.1:8791`。与现有反代同网的 Docker 部署可设置 `CCBOX_LICENSE_CONTAINER=1` 并仅在内部容器网监听 8791。公开 `POST /v1/redeem`、`POST /v1/auth/*` 和 `POST /v1/account*`（见下文「线上账户」）。发码和换机命令在服务器本机执行，代理及监控系统不要记录请求体。使用 systemd 时，创建 `ccbox-license` 系统用户和组，确认二进制和状态路径匹配，再安装并启用本目录的服务单元。

```sh
ccbox-license issue /var/lib/ccbox-license/state /var/lib/ccbox-license/signing.key
```

命令只输出一个可发给用户的激活码。服务端保存码的哈希，同一设备重试已兑换的码会收到同一份授权。

## 网页管理后台

设置环境变量 `CCBOX_LICENSE_ADMIN_PASSWORD_FILE` 为专用密码文件的绝对路径。密码使用至少 24 个字符，文件仅对授权服务账户可读。服务未配置此文件时，管理路由全部关闭。管理员账号由 `CCBOX_LICENSE_ADMIN_USER` 设置（1 至 64 个不含空格的字符），未设置时为 `admin`。开启后访问 `http://127.0.0.1:8791/admin/license/`，用管理员账号和密码登录。账号或密码任一不符时只提示“账号或密码不正确”。

后台包含：

| 页面 | 用途 |
|---|---|
| 概览 | 发放、库存、兑换率、有效设备等指标，近 30 天生成与兑换趋势，7 天内到期的设备 |
| 激活码 | 按状态、类型、批次、尾号或设备筛选；生成带备注的批次；输入完整激活码查询兑换状态；停用未使用的码；导出 CSV |
| 批次 | 每批的兑换进度，一键停用整批未使用的码 |
| 用户 | 线上账户列表与统计，按状态、用户名或编号筛选；详情中查看登录设备、授权记录和相关操作；调整期限、解绑设备、发放密码重置码、停用或恢复账户；导出 CSV |
| 设备 | 到期时间与兑换记录；为设备续期 30 天（生成仅该设备可用的续期码）；换机；导出 CSV |
| 操作日志 | 登录、登录失败、生成、停用、续期、换机和用户管理记录 |

激活码只在生成后显示一次，可复制或下载 TXT / CSV。账本只保存激活码哈希和末 4 位尾号，后台无法再次显示原码；已兑换或绑定设备的码不能停用，因为离线凭证已在设备上。操作日志写入状态目录的 `admin-audit.jsonl`，不含激活码和签名凭证。

登录使用服务端会话：Cookie 为 HttpOnly、SameSite=Strict，经 HTTPS 访问时带 Secure；闲置 45 分钟或登录满 8 小时失效，服务重启后需重新登录。同一来源 15 分钟内失败 8 次后暂停登录。所有写操作校验浏览器 `Origin` 与 `Host`，并要求 JSON 请求体；页面使用严格的内容安全策略，不加载任何外部资源；配色与官网一致，内嵌的 Inter 字体按 SIL OFL 1.1 分发（`web/inter-LICENSE.txt`）。

默认允许的来源为 `http://127.0.0.1:8791`、`http://localhost:8791` 和 `https://ccbox.org`。使用其他域名或端口时，设置 `CCBOX_LICENSE_ADMIN_ORIGINS`，多个来源用逗号分隔，例如 `https://admin.example.com`。

生产部署可在官网的 HTTPS 虚拟主机中单独代理整个管理路径（包括不带结尾斜杠的 `/admin/license`，服务会把它重定向到 `/admin/license/`），保留请求的原始 Host。反代位于本机或内网时，后台从 `X-Forwarded-For` 读取访客地址用于登录限速和日志。建议在代理入口限制管理员来源或使用管理 VPN。主页不显示管理入口。

```caddyfile
handle /admin/license* {
    reverse_proxy 127.0.0.1:8791
}
```

示例适用于反代与授权服务同一网络命名空间。容器中的反代应改用可访问的内部地址。请通过 systemd 服务覆盖配置设置密码文件和来源环境变量，再重启服务：

```ini
# systemctl edit ccbox-license
[Service]
Environment=CCBOX_LICENSE_ADMIN_PASSWORD_FILE=/etc/ccbox-license/admin-password
Environment=CCBOX_LICENSE_ADMIN_USER=admin
Environment=CCBOX_LICENSE_ADMIN_ORIGINS=https://ccbox.org
```

旧版本账本可直接使用：早期发放的码在后台显示为“旧记录”，没有尾号和批次，状态与设备信息照常显示。

## 线上账户

用户在桌面应用中用用户名和密码注册线上账户（第一版不做邮箱验证）。账户与激活码保存在同一个账本中，兑换激活码和延长账户期限是一次原子写入。

- 用户名 3–32 个字母、数字、下划线、连字符或点，不区分大小写；密码至少 8 个字符，服务端只保存 argon2id 哈希。
- 登录会话的令牌只以 SHA-256 摘要保存，只能从签发它的设备使用；90 天未使用失效。修改或重置密码会让其他设备退出登录。
- 激活码兑换到账户后，期限跟随账户。设备凭证格式不变（新增账户字段，旧客户端忽略），每张最长 7 天，客户端在后台自动续签；停用账户或解绑设备最迟 7 天后对离线设备生效。
- 设备首次登录账户时，该设备原有的激活码期限会转入账户一次。旧版客户端在已登录账户的设备上兑换激活码，也会同时延长该账户。
- 每个账户默认最多 2 台设备，可用 `CCBOX_LICENSE_MAX_DEVICES`（1–20）调整。用户可在已登录的设备上移除其他设备，管理员可在后台解绑。
- 注册每个来源每小时 5 次；登录每个来源 15 分钟 30 次，同一用户名 15 分钟内失败 10 次后暂停；重置每个来源每小时 10 次。
- 没有邮箱时，忘记密码由客服在后台「用户」详情中发放一次性重置码（24 小时有效，仅显示一次），用户在登录页「忘记密码？」中输入重置码设置新密码。管理员看不到也不能直接设置用户密码。

公开接口均为 JSON `POST`，已登录的请求带 `Authorization: Bearer` 令牌并在请求体中提供设备编号：

| 路径 | 用途 |
|---|---|
| `/v1/auth/register`、`/v1/auth/login` | 注册或登录，返回会话令牌和账户信息 |
| `/v1/auth/logout`、`/v1/auth/reset` | 退出登录；用重置码设置新密码 |
| `/v1/account` | 当前账户、期限和设备 |
| `/v1/account/grant` | 为当前设备签发凭证 |
| `/v1/account/redeem` | 将激活码兑换到账户 |
| `/v1/account/devices/remove`、`/v1/account/password` | 移除其他设备；修改密码 |

```caddyfile
@license path /v1/redeem /v1/auth/* /v1/account /v1/account/*
handle @license {
    reverse_proxy 127.0.0.1:8791
}
```

代理和监控系统不要记录这些请求的请求体。

## 换机

用户换机时，在两台设备的「设置 → 授权」中获取设备编号，或者从发放记录中核对原设备编号。以下命令返回一个仅供目标设备使用的换机码。

```sh
ccbox-license transfer /var/lib/ccbox-license/state /var/lib/ccbox-license/signing.key OLD_DEVICE NEW_DEVICE
```

也可以在网页后台的「设备」页完成换机。换机保留剩余有效期。旧设备保存的离线凭证在原到期时间失效。服务器账户应只向授权持有人发放换机码。

## 构建客户端

设置 `CCBOX_LICENSE_PUBLIC_KEY` 为 `init-key` 输出的公钥，再通过既有 Windows、Linux 或 macOS 构建入口生成桌面包。未注入公钥的构建沿用预览版行为。私钥只用于授权服务，客户端包中只有公钥。

本机验收构建额外设置 `CCBOX_LICENSE_LOCAL_TEST=1`，将客户端兑换地址设为 `http://127.0.0.1:8791/v1/redeem`。生产构建应清除这个变量，并使用生产公钥和默认 HTTPS 端点。

发布前用测试码验证首次激活、同设备重试、提前续期、断网启动、到期拒绝下一次启动和现有项目保留。使用各平台的实际安装包验收。
