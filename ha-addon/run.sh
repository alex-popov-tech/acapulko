#!/bin/sh
set -e

OPTIONS_FILE="/data/options.json"

echo "Looking for options file..."
if [ -f "$OPTIONS_FILE" ]; then
    echo "Found $OPTIONS_FILE, contents:"
    cat "$OPTIONS_FILE"
    echo ""

    export PORT=$(jq -r '.PORT' "$OPTIONS_FILE")
    export HA_BASE_URL=$(jq -r '.HA_BASE_URL' "$OPTIONS_FILE")
    export HA_TOKEN=$(jq -r '.HA_TOKEN' "$OPTIONS_FILE")
    export HA_ENTITY=$(jq -r '.HA_ENTITY' "$OPTIONS_FILE")
    export HA_POLL_INTERVAL=$(jq -r '.HA_POLL_INTERVAL' "$OPTIONS_FILE")
    export DTEK_BASE_URL=$(jq -r '.DTEK_BASE_URL' "$OPTIONS_FILE")
    export DTEK_REGION=$(jq -r '.DTEK_REGION' "$OPTIONS_FILE")
    export DTEK_CITY=$(jq -r '.DTEK_CITY' "$OPTIONS_FILE")
    export DTEK_STREET=$(jq -r '.DTEK_STREET' "$OPTIONS_FILE")
    export DTEK_BUILDING=$(jq -r '.DTEK_BUILDING' "$OPTIONS_FILE")
    export DTEK_POLL_INTERVAL=$(jq -r '.DTEK_POLL_INTERVAL' "$OPTIONS_FILE")
    export HISTORY_FILE_PATH=$(jq -r '.HISTORY_FILE_PATH' "$OPTIONS_FILE")
    export HISTORY_WINDOW=$(jq -r '.HISTORY_WINDOW' "$OPTIONS_FILE")
    export SENTRY_DSN=$(jq -r '.SENTRY_DSN' "$OPTIONS_FILE")
    export SENTRY_ENV=$(jq -r '.SENTRY_ENV' "$OPTIONS_FILE")
else
    echo "WARNING: $OPTIONS_FILE not found!"
    echo "Checking what files exist in /data/:"
    ls -la /data/ 2>/dev/null || echo "/data/ does not exist"
fi

echo "Starting acapulko..."
exec /acapulko
