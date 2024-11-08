#!/bin/bash

COLOR_RED="$(printf '\033[1;31m')"
COLOR_GREEN="$(printf '\033[1;32m')"
COLOR_DEFAULT="$(printf '\033[0m')"

if [ "$EUID" -ne 0 ]; then
    echo "${COLOR_RED}ERROR${COLOR_DEFAULT}: Please run this script as root"
    exit 1
fi
export PATH="$PATH:/usr/local/go/bin"

echo "${COLOR_GREEN}Building${COLOR_DEFAULT}: binary"
go build -v -ldflags "-s -w"

echo "${COLOR_GREEN}Stopping${COLOR_DEFAULT}: torrentino.service"
systemctl stop torrentino.service

echo "${COLOR_GREEN}Updating${COLOR_DEFAULT}: binary"
mv -v ./torrentino /opt/torrentino/torrentino

echo "${COLOR_GREEN}Starting${COLOR_DEFAULT}: torrentino.service"
systemctl start torrentino.service
