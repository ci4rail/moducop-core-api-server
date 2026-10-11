# SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
#
# SPDX-License-Identifier: Apache-2.0

mock-test:
	$(MAKE) -C ../mocks mocks
	robot -d results -X -i mender -i io4edge-mock -i hardware testcases/

real-test:
	./target_setup.sh 192.168.24.11 assets/Moducop-CPU01_Standard-Image-Non-Secure-Boot_v3.0.0-alpha.1+10.scarthgap-secure-boot-mender-signing.88c25bc.20261011.1847.mender
	robot -d results -X -v DUT_IP:192.168.24.11 -i mender -i io4edge-real -i hardware testcases/

.PHONY: mock-test real-test