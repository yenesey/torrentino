#!/bin/bash
#
# Build and deploy a new torrentino binary: stop the service, replace the
# binary in /opt/torrentino, start the service again. Run as root from any
# directory:
#
#   sudo ./deploy/update-service.sh
#
# The systemd unit template lives next to this script: deploy/torrentino.service.

set -euo pipefail

COLOR_RED="$(printf '\033[1;31m')"
COLOR_GREEN="$(printf '\033[1;32m')"
COLOR_DEFAULT="$(printf '\033[0m')"

if [ "$EUID" -ne 0 ]; then
    echo "${COLOR_RED}ERROR${COLOR_DEFAULT}: Please run this script as root"
    exit 1
fi

# The script lives in deploy/, one level below the Go module root. Resolve
# both paths so it works no matter which directory it is called from.
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname -- "$SCRIPT_DIR")"

export PATH="$PATH:/usr/local/go/bin"

echo "${COLOR_GREEN}Building${COLOR_DEFAULT}: binary"
cd "$REPO_ROOT"
go build -v -ldflags "-s -w" -o "$REPO_ROOT/torrentino"

echo "${COLOR_GREEN}Stopping${COLOR_DEFAULT}: torrentino.service"
systemctl stop torrentino.service

# If replacing the binary fails, bring the previous version back up instead of
# leaving the service stopped.
trap 'echo "${COLOR_RED}ERROR${COLOR_DEFAULT}: update failed, starting the previous binary"; systemctl start torrentino.service' ERR

echo "${COLOR_GREEN}Updating${COLOR_DEFAULT}: binary"
mv -v "$REPO_ROOT/torrentino" /opt/torrentino/torrentino
trap - ERR

echo "${COLOR_GREEN}Starting${COLOR_DEFAULT}: torrentino.service"
systemctl start torrentino.service
