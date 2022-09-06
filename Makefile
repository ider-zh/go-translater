include .env
export

test:
	go test -timeout 10s -v  ./...
