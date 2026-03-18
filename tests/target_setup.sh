#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

# usage: target_factory <ip> <default-image-name>

#set -euo pipefail

if [ "$#" -ne 2 ]; then
    echo "Usage: $0 <ip> <default-image-path>"
    exit 1
fi

dut_ip=$1
default_image_path=$2


pass="cheesebread"
scp_opts="-o StrictHostKeyChecking=no -c aes128-ctr -o IPQoS=throughput"
ssh_opts="-o StrictHostKeyChecking=no"

set -x
sshpass -p "$pass" scp $scp_opts "$default_image_path" "root@$dut_ip:/tmp"
sshpass -p "$pass" ssh $ssh_opts "root@$dut_ip" \
    "mender-update commit; mender-update install /tmp/$(basename "$default_image_path")"
sshpass -p "$pass" ssh $ssh_opts "root@$dut_ip" \
    'echo "RESET" | factory-reset'

sleep 40
sshpass -p "$pass" ssh $ssh_opts "root@$dut_ip" "mender-update commit"
sshpass -p "$pass" ssh $ssh_opts "root@$dut_ip" "rm -f /root/core-api-server; systemctl stop core-api-server.service"
GOOS=linux GOARCH=arm64 go build -o ../bin/core-api-server-arm64 ../cmd/api-server/main.go 
sshpass -p "$pass" scp $scp_opts ../bin/core-api-server-arm64 "root@$dut_ip:/root/core-api-server"
sshpass -p "$pass" scp $scp_opts ../deploy/core-api-server.service "root@$dut_ip:/etc/systemd/system/core-api-server.service"

cpu01ucfw=assets/fw-cpu01uc-default-1.1.0.fwpkg

sshpass -p "$pass" scp $scp_opts "$cpu01ucfw" "root@$dut_ip:/tmp"
sshpass -p "$pass" ssh $ssh_opts "root@$dut_ip" \
    "io4edge-cli -d S101-CPU01UC load-firmware /tmp/$(basename "$cpu01ucfw")"

sshpass -p "$pass" ssh $ssh_opts "root@$dut_ip" \
    "rm -rf /data/core-api-server; systemctl daemon-reload; systemctl enable core-api-server.service; systemctl start core-api-server.service"

sleep 10
