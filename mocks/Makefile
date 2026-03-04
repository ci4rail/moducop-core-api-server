mocks: mender-update docker reboot

mender-update: ## Simulates mender-update for rootfs and application updates
	@go build -o bin/mender-update ./mender-update/main.go

docker: 
	@go build -o bin/docker ./docker/main.go

reboot: ## Simulates a system reboot
	@go build -o bin/reboot ./reboot/main.go

test-mocks:
	robot -d tests/results tests/

.PHONY: mocks mender-update docker reboot test-mocks