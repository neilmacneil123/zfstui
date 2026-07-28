BINARY=zfstui
VERSION?=0.1.0-dev
LDFLAGS=-s -w -X github.com/zfstui/zfstui/internal/version.Version=$(VERSION)

.PHONY: build test install clean

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/zfstui

test:
	go test ./...

install: build
	install -m 755 bin/$(BINARY) /usr/local/bin/$(BINARY)

clean:
	rm -rf bin
