.PHONY: fmt vet test lint build

fmt:
	gofmt -l -w .

vet:
	go vet ./...

test:
	go test ./... -cover

lint:
	golangci-lint run ./...

build:
	go build ./...
