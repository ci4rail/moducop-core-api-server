while true; do
    go run ../cmd/api-server/main.go --server.address :8090 --log.level debug
    echo "API server exited. Restarting in 1 second..."
    sleep 1
done
