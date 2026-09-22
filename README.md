# Kizuna-Backup LITE

A lightweight backup plugin for [Kizuna-Eye](https://github.com/ExiServerTeam/Kizuna-Eye_forEN).

## Features

- **Archive Mode**: Compress directories into `.tar.gz` archives with SHA-256 verification
- **Sync Mode**: Sync files to a remote host via rsync over SSH
- **Cron Scheduling**: Schedule backups with cron expressions or fixed intervals
- **Generation Management**: Automatically rotate old backups (local and remote)
- **DRY-RUN Mode**: Test the backup flow without making actual changes
- **JSON Lines Logging**: Machine-parsable structured logs
- **Self-Declared Config**: The plugin declares its config fields, and the Kizuna-Eye dashboard renders the form automatically

## Requirements

- **Kizuna-Eye** v0.6.0 or higher
- **Go 1.27.1** or higher
- **CGO_ENABLED=1** (when building the plugin)
- **rsync** (for sync mode)
- **SSH key-based authentication** (for sync mode)

## Installation

### 1. Clone the repository

```bash
git clone https://github.com/ExiServerTeam/Kizuna-Backup_forEN.git
cd Kizuna-Backup_forEN/plugin
2. Build the plugin
bash
GOWORK=off CGO_ENABLED=1 go build -buildmode=plugin -o kizuna_backup_lite.so .
3. Deploy to Kizuna-Eye
bash
cp kizuna_backup_lite.so /opt/kizuna-eye/bin/plugins/
4. Register in modules.json
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
5. Restart Kizuna-Eye
bash
cd /path/to/Kizuna-Eye
./stop.sh && ./start.sh
Alternatively, upload the .so file through the Kizuna-Eye dashboard's Module Management tab. The embedded plugin-inspect tool will automatically analyze the plugin and generate the config form.

Configuration
Archive Mode (default)
json
{
  "mode": "archive",
  "target_dir": "/home/user/data",
  "local_temp_dir": "/tmp/kizuna-backups",
  "keep_local": false,
  "rotation_keep": 5
}
The plugin creates a .tar.gz archive of target_dir, computes its SHA-256 hash, and optionally transfers it to a remote host.

Sync Mode (SSH Transfer)
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
Requirements:

SSH key-based authentication (password auth is not supported in automated mode)

rsync installed on both local and remote hosts

Remote directory must exist and be writable

The plugin uses rsync to transfer the archive and verifies the transfer by comparing SHA-256 hashes on both sides.

Schedule Configuration
Two scheduling modes are supported.

1. Fixed Interval (interval_sec):

json
{
  "interval_sec": 3600
}
2. Cron Expression (schedule_expr, takes precedence over interval_sec):

json
{
  "schedule_expr": "0 3 * * *"
}
Cron examples:

Expression	Meaning
0 3 * * *	Every day at 3:00 AM
*/15 * * * *	Every 15 minutes
0 0 * * 0	Every Sunday at midnight
0 9,18 * * 1-5	Weekdays at 9:00 AM and 6:00 PM
Generation Management
Old backups are automatically rotated based on rotation_keep. Set to 0 to disable rotation.

Local rotation: Applied to local_temp_dir if keep_local is true

Remote rotation: Applied to remote_dir if sync mode is used

DRY-RUN Mode
Enable dry_run to test the backup flow without making actual changes.

json
{
  "dry_run": true
}
Log Format
Logs are written in JSON Lines format for machine parsing.

json
{"ts":"2026-09-22T05:14:15Z","level":"INFO","event":"archive_complete","message":"Archive complete: 180.0 B","size":180,"sha256":"abc..."}
Read with jq:

bash
# Human-readable messages
cat log | jq -r '.message'

# Extract sizes from archive_complete events
cat log | jq 'select(.event=="archive_complete") | .size'

# Count events by type
cat log | jq -r '.event' | sort | uniq -c
Full Configuration Reference
Key	Type	Default	Description
mode	string	archive	archive or sync
target_dir	string	(required)	Directory to back up
interval_sec	int	3600	Backup interval in seconds (10–86400)
schedule_expr	string	""	Cron expression (takes precedence over interval_sec)
run_on_start	bool	false	Run a backup immediately on plugin start
local_temp_dir	string	/tmp	Directory for temporary archives
log_path	string	./logs/kizuna-backup-lite.log	Log file path
keep_local	bool	false	Keep the local archive after transfer
rotation_keep	int	5	Number of backups to keep (0 = unlimited)
dry_run	bool	false	Test mode without actual changes
remote_dest	string	""	Remote host (user@host) for sync mode
remote_dir	string	""	Remote directory for sync mode
ssh_port	int	22	SSH port
ssh_key	string	""	Path to SSH private key
License
MIT License. See LICENSE for details.

Author
sy815twty-spec (Exi Server Team)

GitHub: https://github.com/sy815twty-spec

Website: https://exi-server.site/

Related Projects
Kizuna-Eye - A lightweight server monitoring tool

## Related Projects

- [Kizuna-Backup LITE](https://github.com/ExiServerTeam/Kizuna-Backup_forEN) - A lightweight backup plugin for Kizuna-Eye
