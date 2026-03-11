mocks: mender-update docker reboot preparefs io4edge-cli

mender-update: ## Simulates mender-update for rootfs and application updates
	@go build -o bin/mender-update ./mender-update/main.go

docker: 
	@go build -o bin/docker ./docker/main.go

reboot: ## Simulates a system reboot
	@go build -o bin/reboot ./reboot/main.go

preparefs: ## Prepares mock filesystem state
	@go build -o bin/preparefs ./preparefs/main.go

io4edge-cli: ## Simulates io4edge-cli firmware and hardware commands
	@go build -o bin/io4edge-cli ./io4edge-cli/main.go

test-mocks:
	robot -d tests/results tests/

.PHONY: mocks mender-update docker reboot preparefs io4edge-cli test-mocks
