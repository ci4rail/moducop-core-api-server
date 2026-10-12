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
# Install after factory-reset, which clears /data. Keep the executable separate
# from /data/core-api-server, the server's disposable deployment state.
set -e
GOOS=linux GOARCH=arm64 go build -o ../bin/core-api-server-arm64 ../cmd/api-server/main.go
# Keep the image server stopped across OTA reboots using a persistent mask.
sshpass -p "$pass" ssh $ssh_opts "root@$dut_ip" \
    "systemctl mask --now moducop-core-api-server.service"
sshpass -p "$pass" ssh $ssh_opts "root@$dut_ip" \
    "systemctl stop core-api-server.service || true; mkdir -p /data/core-api-server-test /etc/systemd/system"
sshpass -p "$pass" scp $scp_opts ../bin/core-api-server-arm64 "root@$dut_ip:/data/core-api-server-test/core-api-server"
# /etc is a persistent overlay on the target, so the service survives OTA too.
# Replace a possible vendor-unit symlink instead of copying through it. The
# same unit name overrides the image's service rather than running alongside it.
sshpass -p "$pass" ssh $ssh_opts "root@$dut_ip" \
    "rm -f /etc/systemd/system/core-api-server.service"
sshpass -p "$pass" scp $scp_opts core-api-server.service "root@$dut_ip:/etc/systemd/system/core-api-server.service"
sshpass -p "$pass" ssh $ssh_opts "root@$dut_ip" \
    "chmod 755 /data/core-api-server-test/core-api-server"

sleep 60
cpu01ucfw=assets/fw-cpu01uc-default-1.1.0.fwpkg

set -e
sshpass -p "$pass" scp $scp_opts "$cpu01ucfw" "root@$dut_ip:/tmp"
sshpass -p "$pass" ssh $ssh_opts "root@$dut_ip" \
    "io4edge-cli -d S101-CPU01UC load-firmware /tmp/$(basename "$cpu01ucfw")"
sshpass -p "$pass" ssh $ssh_opts "root@$dut_ip" \
    "rm -rf /data/core-api-server && systemctl daemon-reload && systemctl enable --force core-api-server.service && systemctl start core-api-server.service"

if ! python3 - "$dut_ip" <<'READY'
import sys
import time
import urllib.error
import urllib.request

url = f"http://{sys.argv[1]}:8090/api/v1/software/core-os"
for attempt in range(30):
    try:
        with urllib.request.urlopen(url, timeout=2) as response:
            if response.status == 200:
                sys.exit(0)
    except (urllib.error.URLError, TimeoutError):
        pass
    time.sleep(1)
sys.exit("Target API did not become ready")
READY
then
    sshpass -p "$pass" ssh $ssh_opts "root@$dut_ip" \
        "systemctl status core-api-server.service --no-pager; journalctl -u core-api-server.service -b --no-pager -n 80"
    exit 1
fi
