#!/usr/bin/env bash
# One-time (and re-runnable) setup of the Oracle Always Free ARM VM, Ubuntu 24.04.
# Run as a sudo user from a checkout at /opt/careerglobe:  sudo ops/server/setup.sh
# It does not create secrets: copy your .env to /opt/careerglobe/.env (chmod 600) first.
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "run with sudo"; exit 1; }
APP=/opt/careerglobe
[[ -f "$APP/.env" ]] || { echo "$APP/.env is missing (copy it from your password manager)"; exit 1; }

export DEBIAN_FRONTEND=noninteractive
apt-get update -q
apt-get install -yq ca-certificates curl gnupg ufw unattended-upgrades fail2ban \
  postgresql-16 postgresql-16-postgis-3 python3.12 python3.12-venv golang-go age rclone jq

# Automatic security updates, a firewall with only SSH open, SSH keys only.
dpkg-reconfigure -fnoninteractive unattended-upgrades
ufw default deny incoming
ufw default allow outgoing
ufw allow OpenSSH
ufw --force enable
sed -i 's/^#\?PasswordAuthentication .*/PasswordAuthentication no/' /etc/ssh/sshd_config
sed -i 's/^#\?PermitRootLogin .*/PermitRootLogin no/' /etc/ssh/sshd_config
systemctl reload ssh

# PostgreSQL listens on localhost only.
PGCONF=$(ls /etc/postgresql/16/main/postgresql.conf)
sed -i "s/^#\?listen_addresses.*/listen_addresses = 'localhost'/" "$PGCONF"
systemctl restart postgresql

# Logs never fill the disk.
mkdir -p /etc/systemd/journald.conf.d
printf '[Journal]\nSystemMaxUse=500M\n' > /etc/systemd/journald.conf.d/careerglobe.conf
systemctl restart systemd-journald

# The app user and its data directory (resumes live here, outside any web root).
id careerglobe >/dev/null 2>&1 || useradd --system --home "$APP" --shell /usr/sbin/nologin careerglobe
install -d -o careerglobe -g careerglobe -m 700 /var/lib/careerglobe /var/lib/careerglobe/resumes
chown -R careerglobe:careerglobe "$APP"
chmod 600 "$APP/.env"

# Cloudflare Tunnel client.
if ! command -v cloudflared >/dev/null; then
  mkdir -p --mode=0755 /usr/share/keyrings
  curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg -o /usr/share/keyrings/cloudflare-main.gpg
  echo "deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared $(. /etc/os-release; echo $VERSION_CODENAME) main" \
    > /etc/apt/sources.list.d/cloudflared.list
  apt-get update -q && apt-get install -yq cloudflared
fi

# goose for migrations, then the app: Python venv and Go binaries.
sudo -u careerglobe -H bash -c "cd $APP && GOBIN=$APP/api/bin go install github.com/pressly/goose/v3/cmd/goose@v3.28.0"
sudo -u careerglobe -H bash -c "cd $APP && python3.12 -m venv crawler/.venv && crawler/.venv/bin/pip install -q -r crawler/requirements.txt"
sudo -u careerglobe -H bash -c "cd $APP/api && go build -o bin/api ./cmd/api && go build -o bin/nightly ./cmd/nightly"

cp "$APP"/ops/systemd/careerglobe-*.{service,timer} /etc/systemd/system/
systemctl daemon-reload
echo
echo "Done. Next (see ops/server/README.md): create the database, run migrations and db-roles.sql,"
echo "then: systemctl enable --now careerglobe-api careerglobe-nightly.timer careerglobe-push.timer careerglobe-backup.timer"
