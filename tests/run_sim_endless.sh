# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

existing_pids="$(lsof -t -iTCP:8090 -sTCP:LISTEN 2>/dev/null)"
if [ -n "$existing_pids" ]; then
    echo "Stopping processes listening on port 8090: $existing_pids"
    kill -9 $existing_pids
fi

while true; do
    go run ../cmd/api-server/main.go --server.address :8090 --log.level debug
    echo "API server exited. Restarting in 1 second..."
    sleep 1
done
