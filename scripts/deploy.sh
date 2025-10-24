#!/bin/bash
set -e
echo "Deploying codec-svr..."

ssh ubuntu@tu-servidor <<'EOF'
cd /srv/codec-svr
git pull
go build -o codec-svr ./cmd/server
sudo systemctl restart codec-svr
sudo systemctl status codec-svr --no-pager
EOF
