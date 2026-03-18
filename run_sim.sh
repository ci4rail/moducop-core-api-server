# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

export PATH=`pwd`/mocks/bin:$PATH
export MOCK_MENDER_STATE_DIR=`pwd`/mock-mender-state 
export MOCK_MENDER_KILL_PARENT=yes
go run cmd/api-server/main.go --server.address :8090 --log.level debug
