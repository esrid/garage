.PHONY: build run test vet lint vuln

build:
	go build -o bin/app ./cmd

run:
	go run ./cmd

test:
	go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...
