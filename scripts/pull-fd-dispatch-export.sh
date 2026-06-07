#!/usr/bin/env bash
# Export 78 FD dispatch (TG 46036) calls from production and pull locally.
set -euo pipefail

LIMIT="${1:-600}"
LOCAL_DIR="${LOCAL_DIR:-/tmp/tlr-debug/tlr-fd-dispatch-export}"
REMOTE_DIR="/tmp/tlr-debug/tlr-fd-dispatch-export"
PROD_HOST="${PROD_HOST:-thinline@10.10.10.188}"
PROD_INI="${PROD_INI:-/opt/thinline-radio/thinline-radio.ini}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "Building export-lfd-calls for linux/amd64..."
(cd "$REPO_ROOT/server" && GOOS=linux GOARCH=amd64 go build -o /tmp/export-lfd-calls ./cmd/export-lfd-calls)

echo "Copying exporter to $PROD_HOST..."
scp /tmp/export-lfd-calls "${PROD_HOST}:/tmp/export-lfd-calls"

echo "Exporting $LIMIT calls (talkgroup-ref 46036) on production..."
ssh "$PROD_HOST" "chmod +x /tmp/export-lfd-calls && /tmp/export-lfd-calls -ini '$PROD_INI' -out '$REMOTE_DIR' -limit $LIMIT -talkgroup-ref 46036"

echo "Pulling export to $LOCAL_DIR..."
mkdir -p "$LOCAL_DIR"
rsync -avz --progress "${PROD_HOST}:${REMOTE_DIR}/" "$LOCAL_DIR/"

echo "Done. $(wc -l < "$LOCAL_DIR"/calls_*.csv | tr -d ' ') csv rows, $(ls "$LOCAL_DIR/audio" | wc -l | tr -d ' ') audio files"
