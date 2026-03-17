mock-test:
	robot -d results -X -i io4edge-mock testcases/

real-test:
	./target_setup.sh 192.168.24.11 assets/Moducop-CPU01_Standard-Image_v2.7.0.40ee657.20260218.1208.mender
	robot -d results -X -v DUT_IP:192.168.24.11 -i io4edge-real testcases/

.PHONY: test