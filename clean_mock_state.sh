# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

export MOCK_MENDER_STATE_DIR=`pwd`/mock-mender-state && rm -rf $MOCK_MENDER_STATE_DIR && mocks/bin/preparefs
