#!/bin/bash
# /samba/share/Kizuna-Backup/Kizuna-Backup-LITE/plugin/build.sh
set -e
cd "$(dirname "${BASH_SOURCE[0]}")"

GOWORK=off CGO_ENABLED=1 go build -buildmode=plugin -o kizuna_backup_lite.so .

# 配備先にコピー
DEPLOY_DIR="/opt/kizuna-eye/bin/plugins"
if [ -d "$DEPLOY_DIR" ]; then
    cp kizuna_backup_lite.so "$DEPLOY_DIR/"
    echo "✅ 配備完了: $DEPLOY_DIR/kizuna_backup_lite.so"
else
    echo "⚠️ 配備先が見つかりません: $DEPLOY_DIR"
fi

ls -la kizuna_backup_lite.so