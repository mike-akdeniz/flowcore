#!/bin/bash
# Turns a blank Ubuntu 24.04 LTS instance into a CaseWork host (client decision
# 55). Lightsail runs it once, as root, at first boot, as the instance's launch
# script; it runs the same by hand as `sudo bash setup.sh` on a fresh VM.
#
# It leaves everything running except CaseWork itself: the binary arrives from
# the deploy pipeline, and the unit starts as soon as one is there. Lightsail
# caps a launch script at 16 KB, so comments here are short and the reasons live
# in client/docs/decisions.md, entries 50 to 60.
# Lightsail runs a launch script with sh, which is dash on Ubuntu and has no
# pipefail, whatever the first line says. Run again under bash if this is not it.
if [ -z "${BASH_VERSION:-}" ]; then
	exec bash "$0" "$@"
fi

set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

# The public half of the deploy key. The infrastructure code replaces the
# placeholder; by hand, `sed` does.
deploy_public_key='__DEPLOY_PUBLIC_KEY__'

domain=casework.happensbefore.com

# llama.cpp's prebuilt Linux x64 release, and Gemma 3 270M at the commit and
# file the tests were run against. Both are checked against their hashes.
llama_release=b11146
llama_sha256=c150306eb16b5ab696f76a8bdf810c35fd98a24e82158742e6fa28f420ff8410
model_commit=e7647be17ae1108f2f605ed061ca0608b171afff
model_sha256=0ef57d2c838458a1952664260dcba38e5bdda37494f3af732f06e4add24068e3

if [[ "$deploy_public_key" == __* ]]; then
	echo "setup.sh: deploy_public_key was not filled in" >&2
	exit 1
fi

timedatectl set-timezone UTC

# --- packages -----------------------------------------------------------------

apt-get update
apt-get install -y ca-certificates curl gpg openssl

# Caddy from its own repository: Ubuntu's package is several releases behind.
curl -fsSL https://dl.cloudsmith.io/public/caddy/stable/gpg.key |
	gpg --dearmor -o /usr/share/keyrings/caddy-stable.gpg
echo "deb [signed-by=/usr/share/keyrings/caddy-stable.gpg] https://dl.cloudsmith.io/public/caddy/stable/deb/debian any-version main" \
	>/etc/apt/sources.list.d/caddy-stable.list

apt-get update
apt-get install -y caddy libgomp1 postgresql ufw unattended-upgrades

# --- who can log in -----------------------------------------------------------

# The pipeline's user. It owns the CaseWork directory so a deploy is a copy, and
# may restart the one service, nothing else.
useradd --create-home --shell /bin/bash deploy
install -d -m 700 -o deploy -g deploy /home/deploy/.ssh
echo "$deploy_public_key" >/home/deploy/.ssh/authorized_keys
chown deploy:deploy /home/deploy/.ssh/authorized_keys
chmod 600 /home/deploy/.ssh/authorized_keys

echo 'deploy ALL=(root) NOPASSWD: /usr/bin/systemctl restart casework' >/etc/sudoers.d/deploy
chmod 440 /etc/sudoers.d/deploy
visudo -cf /etc/sudoers.d/deploy

install -d -o deploy -g deploy /opt/casework

# Named 01 so it is read before cloud-init's own 50 file; the first value wins.
cat >/etc/ssh/sshd_config.d/01-casework.conf <<'EOF'
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin no
EOF
# sshd starts on demand on Ubuntu 24.04, so its runtime directory may not exist
# yet, and the syntax check needs it.
install -d /run/sshd
sshd -t
systemctl try-reload-or-restart ssh

ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force enable

# --- postgres -----------------------------------------------------------------

database_password=$(openssl rand -hex 24)

sudo -u postgres psql -v ON_ERROR_STOP=1 <<EOF
create role casework login password '$database_password';
create database casework owner casework;
EOF

# CaseWork's whole configuration. 24 idle hours and Secure cookies are decision
# 56; it listens on localhost, and Caddy is the only way in.
umask 077
cat >/etc/casework.env <<EOF
CLIENT_DATABASE_URL=postgres://casework:$database_password@127.0.0.1:5432/casework?sslmode=disable
CLIENT_ADDR=127.0.0.1:8080
CLIENT_LOCAL_MODEL_URL=http://127.0.0.1:8081
CLIENT_SESSION_TTL=24h
CLIENT_SECURE_COOKIES=true
EOF
umask 022

# --- the model server ---------------------------------------------------------

install -d /opt/llama
cd /opt/llama

llama_archive="llama-$llama_release-bin-ubuntu-x64.tar.gz"
curl -fsSL -o "$llama_archive" "https://github.com/ggml-org/llama.cpp/releases/download/$llama_release/$llama_archive"
echo "$llama_sha256  $llama_archive" | sha256sum -c -
tar -xzf "$llama_archive" --strip-components=1
rm "$llama_archive"

curl -fsSL -o model.gguf \
	"https://huggingface.co/ggml-org/gemma-3-270m-it-GGUF/resolve/$model_commit/gemma-3-270m-it-Q8_0.gguf"
echo "$model_sha256  model.gguf" | sha256sum -c -

cd /

# --- services -----------------------------------------------------------------

# One slot and about 4,000 tokens of context: the dispatcher's one worker never
# asks for two calls at once, and the server's default would reserve 600 MB.
cat >/etc/systemd/system/llama-server.service <<'EOF'
[Unit]
Description=llama-server, the model that decides agent steps
After=network.target

[Service]
DynamicUser=yes
ExecStart=/opt/llama/llama-server -m /opt/llama/model.gguf --alias gemma-3-270m --host 127.0.0.1 --port 8081 --parallel 1 --ctx-size 4000
Restart=always

[Install]
WantedBy=multi-user.target
EOF

# Skipped, not failed, until the pipeline has copied a binary in.
cat >/etc/systemd/system/casework.service <<'EOF'
[Unit]
Description=CaseWork
After=network.target postgresql.service llama-server.service
Wants=postgresql.service llama-server.service
ConditionPathExists=/opt/casework/casework

[Service]
DynamicUser=yes
EnvironmentFile=/etc/casework.env
ExecStart=/opt/casework/casework
Restart=always

[Install]
WantedBy=multi-user.target
EOF

# Full client addresses, kept 60 days (decision 57). Caddy redacts the Cookie
# header in its log by default.
install -d -o caddy -g caddy /var/log/caddy

cat >/etc/caddy/Caddyfile <<EOF
$domain {
	log {
		output file /var/log/caddy/access.log {
			roll_size 100MiB
			roll_keep 60
			roll_keep_for 1440h
		}
	}
	reverse_proxy 127.0.0.1:8080
}
EOF

systemctl daemon-reload
systemctl enable --now llama-server casework
systemctl restart caddy

# --- upkeep -------------------------------------------------------------------

# Security updates for Ubuntu's packages are on by default; Caddy's repository is
# added to them, and a reboot at 07:00 UTC only when an update needs one
# (decision 60).
cat >/etc/apt/apt.conf.d/52casework <<'EOF'
Unattended-Upgrade::Origins-Pattern { "origin=cloudsmith/caddy/stable"; };
Unattended-Upgrade::Automatic-Reboot "true";
Unattended-Upgrade::Automatic-Reboot-Time "07:00";
EOF

cat >/etc/apt/apt.conf.d/20auto-upgrades <<'EOF'
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
EOF

install -d /etc/systemd/journald.conf.d
cat >/etc/systemd/journald.conf.d/size.conf <<'EOF'
[Journal]
SystemMaxUse=200M
EOF
systemctl restart systemd-journald

echo "setup.sh: done"
