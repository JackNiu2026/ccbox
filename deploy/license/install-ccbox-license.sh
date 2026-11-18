#!/usr/bin/env bash
# Install or upgrade the CCBox license service (activation codes, online
# accounts and the admin console) on a systemd Linux server.
#
#   sudo bash install-ccbox-license.sh ./ccbox-license-linux-amd64
#
# Safe to run again for upgrades: an existing signing key, ledger and admin
# password are kept. Prints the public key for CCBOX_LICENSE_PUBLIC_KEY.
set -euo pipefail

binary=${1:-}
[[ $(id -u) == 0 ]] || { echo 'Run as root (sudo).' >&2; exit 1; }
[[ -n $binary && -f $binary ]] || { echo "Usage: sudo bash $0 PATH/TO/ccbox-license-linux-ARCH" >&2; exit 1; }
command -v systemctl >/dev/null || { echo 'This installer needs systemd.' >&2; exit 1; }
case "$(uname -m)" in
  x86_64) want=amd64 ;; aarch64|arm64) want=arm64 ;; *) want=unknown ;;
esac
[[ $binary == *"$want"* ]] || echo "Note: this server is $(uname -m); make sure the binary matches (linux-$want)." >&2

user=ccbox-license
state_root=/var/lib/ccbox-license
key=$state_root/signing.key
state=$state_root/state
existing=0

echo '==> Existing deployment'
# Upgrade an existing systemd service in place, keeping its key and ledger.
unit_exec=$(systemctl show -p ExecStart --value ccbox-license 2>/dev/null || true)
if [[ $unit_exec == *ccbox-license*serve* ]]; then
  read -r -a parts <<< "$(sed -n 's/.*argv\[\]=\([^;]*\);.*/\1/p' <<< "$unit_exec")"
  if [[ ${#parts[@]} -ge 4 && ${parts[1]} == serve ]]; then
    state=${parts[2]}; key=${parts[3]}
    unit_user=$(systemctl show -p User --value ccbox-license)
    [[ -n $unit_user ]] && user=$unit_user
    existing=1
    echo "Upgrading the existing service in place: state $state, key $key, user $user."
    echo 'Its service file and admin password are kept; only the program is replaced.'
  fi
elif command -v ss >/dev/null && ss -ltn 2>/dev/null | grep -q ':8791 '; then
  echo 'Port 8791 is already used by something other than a ccbox-license systemd service' >&2
  echo '(for example a Docker container). Nothing was changed. Send this report to support:' >&2
  ss -ltnp 2>/dev/null | grep ':8791 ' >&2 || true
  command -v docker >/dev/null && docker ps --format '{{.Names}} {{.Image}} {{.Ports}}' 2>/dev/null >&2 || true
  find / -xdev -name signing.key -path '*license*' 2>/dev/null >&2 || true
  exit 1
fi
state_root=$(dirname "$key")
public_file=$state_root/public-key.txt
config=/etc/ccbox-license
password_file=$config/admin-password
origin=${CCBOX_ADMIN_ORIGIN:-https://ccbox.org}

if [[ $existing == 0 ]]; then
  echo '==> Service account and directories'
  id "$user" >/dev/null 2>&1 || useradd --system --home-dir "$state_root" --shell /usr/sbin/nologin "$user"
  install -d -m 700 -o "$user" -g "$user" "$state_root"
  [[ -d $state ]] || install -d -m 700 -o "$user" -g "$user" "$state"
  install -d -m 750 -o root -g "$user" "$config"
fi

echo '==> Binary'
if systemctl is-active --quiet ccbox-license 2>/dev/null; then systemctl stop ccbox-license; fi
install -m 755 "$binary" /usr/local/bin/ccbox-license

echo '==> Signing key'
if [[ -f $key ]]; then
  echo "Keeping the existing signing key ($key)."
else
  runuser -u "$user" -- /usr/local/bin/ccbox-license init-key "$key" > "$public_file.tmp"
  mv "$public_file.tmp" "$public_file"
  chown "$user:$user" "$public_file"
  echo 'Created a new signing key. Back up the whole state directory somewhere private.'
fi
if [[ ! -s $public_file ]]; then
  # An older install kept no copy of the public key: derive it from the seed.
  command -v openssl >/dev/null || { echo 'openssl is required to read the existing public key.' >&2; exit 1; }
  seed_hex=$(tr -d '\n\r' < "$key" | tr '_-' '/+' | awk '{ while (length($0) % 4) $0 = $0 "="; print }' | base64 -d | od -An -v -tx1 | tr -d ' \n')
  [[ ${#seed_hex} == 64 ]] || { echo 'Existing signing key has an unexpected format.' >&2; exit 1; }
  printf '302e020100300506032b657004220420%s' "$seed_hex" | sed 's/../\\x&/g' | xargs -0 printf '%b' |
    openssl pkey -inform DER -pubout -outform DER | tail -c 32 | base64 | tr '+/' '-_' | tr -d '=\n' > "$public_file"
  chown "$user:$user" "$public_file"
fi
public_key=$(tr -d '\n\r' < "$public_file")
[[ $public_key =~ ^[A-Za-z0-9_-]{43}$ ]] || { echo 'Could not determine the public key.' >&2; exit 1; }

new_password=''
if [[ $existing == 1 ]]; then
  systemctl restart ccbox-license
else
echo '==> Admin console password'
if [[ ! -s $password_file ]]; then
  new_password=$(head -c 48 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 32)
  umask 077
  printf '%s\n' "$new_password" > "$password_file"
fi
chown "$user:$user" "$password_file"
chmod 600 "$password_file"

echo '==> systemd service'
cat > /etc/systemd/system/ccbox-license.service <<'UNIT'
[Unit]
Description=CCBox activation and accounts
After=network.target

[Service]
User=SERVICE_USER
Group=SERVICE_USER
StateDirectory=ccbox-license
StateDirectoryMode=0700
ExecStart=/usr/local/bin/ccbox-license serve STATE_PATH KEY_PATH
Restart=on-failure
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
UMask=0077

[Install]
WantedBy=multi-user.target
UNIT
sed -i -e "s#STATE_PATH#$state#" -e "s#KEY_PATH#$key#" -e "s#SERVICE_USER#$user#g" /etc/systemd/system/ccbox-license.service
install -d /etc/systemd/system/ccbox-license.service.d
cat > /etc/systemd/system/ccbox-license.service.d/admin.conf <<CONF
[Service]
Environment=CCBOX_LICENSE_ADMIN_PASSWORD_FILE=$password_file
Environment=CCBOX_LICENSE_ADMIN_USER=admin
Environment=CCBOX_LICENSE_ADMIN_ORIGINS=$origin
CONF
systemctl daemon-reload
systemctl enable --now ccbox-license >/dev/null
systemctl restart ccbox-license
fi

echo '==> Health check'
for _ in $(seq 1 20); do
  if curl -fsS http://127.0.0.1:8791/healthz >/dev/null 2>&1; then break; fi
  sleep 0.5
done
curl -fsS http://127.0.0.1:8791/healthz >/dev/null || { journalctl -u ccbox-license -n 30 --no-pager; echo 'Service did not start.' >&2; exit 1; }
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{"username":"x","password":"x","device":"x"}' http://127.0.0.1:8791/v1/auth/login)
[[ $code == 401 ]] || { echo "Account API answered $code instead of 401." >&2; exit 1; }
echo 'Service is running locally on 127.0.0.1:8791.'

cat <<DONE

============================================================
CCBox license service installed.

Public key (GitHub repository variable CCBOX_LICENSE_PUBLIC_KEY):
  $public_key

Admin console: $origin/admin/license/   user: admin
DONE
if [[ -n $new_password ]]; then
  echo "  password (shown once, also in $password_file):"
  echo "  $new_password"
else
  echo "  password: unchanged (existing admin settings were kept)"
fi
echo
echo '==> Reverse proxy'
paths='/v1/redeem /v1/auth/* /v1/account /v1/account/* /admin/license*'
caddyfile=/etc/caddy/Caddyfile
proxy_done=0
if [[ -f $caddyfile ]] && command -v caddy >/dev/null && grep -q '/v1/redeem' "$caddyfile"; then
  if grep -q '/v1/auth/' "$caddyfile"; then
    echo 'Caddy already forwards the account paths.'
    proxy_done=1
  else
    backup="$caddyfile.before-ccbox-accounts.$(date +%Y%m%d%H%M%S)"
    cp -p "$caddyfile" "$backup"
    awk -v extra='/v1/auth/* /v1/account /v1/account/*' '
      !done && /\/v1\/redeem/ {
        if ($0 ~ /^[[:space:]]*@[^[:space:]]+[[:space:]]+path[[:space:]]/) { print $0 " " extra; done = 1; next }
        match($0, /^[[:space:]]*/); indent = substr($0, 1, RLENGTH)
        print indent "@ccbox_license_api path /v1/redeem " extra
        sub(/\/v1\/redeem/, "@ccbox_license_api"); print; done = 1; next
      }
      { print }' "$backup" > "$caddyfile"
    if caddy validate --config "$caddyfile" --adapter caddyfile >/dev/null 2>&1 && systemctl reload caddy; then
      echo "Caddy now forwards the account paths (backup: $backup)."
      proxy_done=1
    else
      cp -p "$backup" "$caddyfile"
      echo "Could not update Caddy automatically; the original Caddyfile was restored." >&2
    fi
  fi
fi
if [[ $proxy_done == 1 ]]; then
  sleep 2
  public=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{}' https://ccbox.org/v1/auth/login || true)
  echo "Public check https://ccbox.org/v1/auth/login -> $public (401 expected)"
else
  cat <<MANUAL
Forward these paths on https://ccbox.org to 127.0.0.1:8791:
  $paths
Caddy, inside the ccbox.org site block:
    @ccbox_license path $paths
    handle @ccbox_license {
        reverse_proxy 127.0.0.1:8791
    }
nginx, inside the ccbox.org server block:
    location ~ ^/(v1/redeem|v1/auth/|v1/account(/|\$)|admin/license) {
        proxy_pass http://127.0.0.1:8791;
        proxy_set_header Host \$host;
        proxy_set_header X-Forwarded-For \$remote_addr;
    }
MANUAL
fi
echo '============================================================'
