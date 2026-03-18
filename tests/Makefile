# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

mock-test:
	robot -d results -X -i mender -i io4edge-mock -i hardware testcases/

real-test:
	./target_setup.sh 192.168.24.11 assets/Moducop-CPU01_Standard-Image_v2.7.0.40ee657.20260218.1208.mender
	robot -d results -X -v DUT_IP:192.168.24.11 -i mender -i io4edge-real -i hardware testcases/

.PHONY: mock-test real-test