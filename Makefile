.PHONY: build lint test

build:
	go build -o boltdb-cli .

lint:
	golangci-lint run ./...

test:
	go test ./...
