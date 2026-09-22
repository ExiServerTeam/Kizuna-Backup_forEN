# Kizuna-Backup LITE

[Kizuna-Eye](https://github.com/ExiServerTeam/Kizuna-Eye_forEN) 用の軽量バックアッププラグイン。

[English](README.md) | **日本語**

## 特徴

- **アーカイブモード**: ディレクトリを `.tar.gz` で圧縮し、SHA-256 で検証
- **同期モード**: rsync over SSH でリモートホストにファイルを転送
- **cronスケジューリング**: cron式または固定間隔でバックアップを実行
- **世代管理**: 古いバックアップを自動削除（ローカル・リモート両方）
- **DRY-RUNモード**: 実際の変更を行わずにバックアップフローをテスト
- **JSON Lines ログ**: 機械的に解析可能な構造化ログ

## 動作要件

- **Kizuna-Eye** v0.6.0 以上
- **Go 1.27.1** 以上
- **CGO_ENABLED=1**
- **rsync**（同期モード使用時）
- **SSH鍵認証**（同期モード使用時）

## インストール

### 1. プラグインをビルド

```bash
git clone https://github.com/ExiServerTeam/Kizuna-Backup-LITE.git
cd Kizuna-Backup-LITE/plugin
GOWORK=off CGO_ENABLED=1 go build -buildmode=plugin -o kizuna_backup_lite.so .
2. Kizuna-Eye に配備
bash
cp kizuna_backup_lite.so /opt/kizuna-eye/bin/plugins/
3. modules.json に登録
json
[
  {
    "name": "kizuna_backup_lite",
    "type": "plugin",
    "enabled": true,
    "config": {
      "plugin_path": "/opt/kizuna-eye/bin/plugins/kizuna_backup_lite.so",
      "mode": "archive",
      "target_dir": "/path/to/backup/target",
      "interval_sec": 3600,
      "schedule_expr": "",
      "run_on_start": false,
      "local_temp_dir": "/tmp/kizuna-backups",
      "log_path": "./logs/kizuna-backup-lite.log",
      "keep_local": false,
      "rotation_keep": 5,
      "dry_run": false
    }
  }
]
4. Kizuna-Eye を再起動
bash
cd /path/to/Kizuna-Eye
./stop.sh && ./start.sh
設定
アーカイブモード（デフォルト）
json
{
  "mode": "archive",
  "target_dir": "/home/user/data",
  "local_temp_dir": "/tmp/kizuna-backups",
  "keep_local": false,
  "rotation_keep": 5
}
同期モード（SSH転送）
json
{
  "mode": "sync",
  "target_dir": "/home/user/data",
  "remote_dest": "user@192.168.0.100",
  "remote_dir": "/backup/kizuna",
  "ssh_port": 22,
  "ssh_key": "/home/user/.ssh/id_ed25519",
  "rotation_keep": 5
}
要件:

SSH鍵認証（自動実行モードではパスワード認証は未対応）

ローカル・リモート両方に rsync がインストールされていること

リモートディレクトリが存在し、書き込み可能であること

スケジュール設定
2つのスケジューリング方式をサポートします。

1. 固定間隔 (interval_sec):

json
{
  "interval_sec": 3600
}
2. cron式 (schedule_expr、指定時は interval_sec より優先):

json
{
  "schedule_expr": "0 3 * * *"
}
cron式の例:

式	意味
0 3 * * *	毎日 午前3時
*/15 * * * *	15分ごと
0 0 * * 0	毎週日曜 午前0時
0 9,18 * * 1-5	平日 午前9時と午後6時
ログ形式
ログは機械解析用に JSON Lines 形式で出力されます。

json
{"ts":"2026-09-22T05:14:15Z","level":"INFO","event":"archive_complete","message":"圧縮完了: 180.0 B","size":180,"sha256":"abc..."}
jq での読み取り例:

bash
# 人間可読なメッセージ
cat log | jq -r '.message'

# archive_complete イベントからサイズを抽出
cat log | jq 'select(.event=="archive_complete") | .size'
ライセンス
MIT License. 詳細は LICENSE を参照してください。

作者
sy815twty-spec（Exi Server Team）

GitHub: https://github.com/sy815twty-spec

Website: https://exi-server.site/
