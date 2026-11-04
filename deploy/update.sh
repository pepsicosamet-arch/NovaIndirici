#!/bin/sh
# NOVA web sürümünü GitHub'daki son koda göre kurar / günceller.
# Kullanım: sh /opt/nova/deploy/update.sh [--force]
set -e
cd /opt/nova
docker network inspect nova >/dev/null 2>&1 || docker network create nova >/dev/null
git fetch -q origin main
if [ "$1" = "--force" ] || [ "$(git rev-parse HEAD)" != "$(git rev-parse origin/main)" ]; then
  git reset -q --hard origin/main
  echo "$(date '+%F %T') yeni sürüm kuruluyor: $(git log -1 --format=%s)"
  docker build -q -t nova-web .
  docker rm -f nova >/dev/null 2>&1 || true
  docker run -d --name nova --network nova --restart unless-stopped \
    -e NOVA_MAX_JOBS=3 -e NOVA_PER_USER=2 -e NOVA_MAX_ITEMS=10 -e NOVA_MAX_FILESIZE=2G \
    nova-web >/dev/null
  docker image prune -f >/dev/null 2>&1 || true
fi
if ! docker inspect caddy >/dev/null 2>&1; then
  docker run -d --name caddy --network nova --restart unless-stopped \
    -p 80:80 -p 443:443 -v caddy_data:/data -v caddy_config:/config \
    -v /opt/nova/deploy/Caddyfile:/etc/caddy/Caddyfile:ro caddy:2 >/dev/null
else
  docker exec caddy caddy reload --config /etc/caddy/Caddyfile >/dev/null 2>&1 || true
fi
