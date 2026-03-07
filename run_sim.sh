export PATH=`pwd`/mocks/bin:$PATH
export MOCK_MENDER_STATE_DIR=`pwd`/mock-mender-state 
go run cmd/api-server/main.go --server.address :8090 --log.level debug