.PHONY: build install

build:
	GOOS=darwin GOARCH=arm64 go build -o bin/obi ./cmd/obi

install:
	GOOS=darwin GOARCH=arm64 go install ./cmd/obi
