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

`init-key` 输出构建客户端所需的公钥。将私钥和状态目录备份到受限位置。恢复时同时恢复签名密钥和当前账本，回滚旧账本可能使已经使用的码再次可用。

将 `https://ccbox.org/v1/redeem` 反向代理到授权服务。宿主机部署默认监听 `127.0.0.1:8791`。与现有反代同网的 Docker 部署可设置 `CCBOX_LICENSE_CONTAINER=1` 并仅在内部容器网监听 8791。仅公开 `POST /v1/redeem`。发码和换机命令在服务器本机执行，代理及监控系统不要记录请求体。使用 systemd 时，创建 `ccbox-license` 系统用户和组，确认二进制和状态路径匹配，再安装并启用本目录的服务单元。

```sh
ccbox-license issue /var/lib/ccbox-license/state /var/lib/ccbox-license/signing.key
```

命令只输出一个可发给用户的激活码。服务端保存码的哈希，同一设备重试已兑换的码会收到同一份授权。

## 网页发码后台

设置环境变量 `CCBOX_LICENSE_ADMIN_PASSWORD_FILE` 为专用密码文件的绝对路径。密码使用至少 24 个字符，文件仅对授权服务账户可读。服务未配置此文件时，管理路由关闭。开启后以用户名 `admin` 登录 `http://127.0.0.1:8791/admin/license/`。

生产部署可在官网的 HTTPS 虚拟主机中单独代理此管理路径，保留请求的原始 Host。凭据通过 HTTPS 传输。建议在代理入口限制管理员来源或使用管理 VPN。主页不显示管理入口。

```caddyfile
@licenseAdmin path /admin/license/
handle @licenseAdmin {
    reverse_proxy 127.0.0.1:8791
}
```

示例适用于反代与授权服务同一网络命名空间。容器中的反代应改用可访问的内部地址。请通过 systemd 服务覆盖配置设置密码文件环境变量，再重启服务。

## 换机

用户换机时，在两台设备的「设置 → 授权」中获取设备编号，或者从发放记录中核对原设备编号。以下命令返回一个仅供目标设备使用的换机码。

```sh
ccbox-license transfer /var/lib/ccbox-license/state /var/lib/ccbox-license/signing.key OLD_DEVICE NEW_DEVICE
```

换机保留剩余有效期。旧设备保存的离线凭证在原到期时间失效。服务器账户应只向授权持有人发放换机码。

## 构建客户端

设置 `CCBOX_LICENSE_PUBLIC_KEY` 为 `init-key` 输出的公钥，再通过既有 Windows、Linux 或 macOS 构建入口生成桌面包。未注入公钥的构建沿用预览版行为。私钥只用于授权服务，客户端包中只有公钥。

本机验收构建额外设置 `CCBOX_LICENSE_LOCAL_TEST=1`，将客户端兑换地址设为 `http://127.0.0.1:8791/v1/redeem`。生产构建应清除这个变量，并使用生产公钥和默认 HTTPS 端点。

发布前用测试码验证首次激活、同设备重试、提前续期、断网启动、到期拒绝下一次启动和现有项目保留。使用各平台的实际安装包验收。
