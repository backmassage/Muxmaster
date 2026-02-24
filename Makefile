BINARY  := muxmaster
VERSION := 2.0.0-dev
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

.PHONY: build test vet fmt lint coverage ci clean install hooks

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/muxmaster

test:
	go test ./... -v -count=1

vet:
	go vet ./...

fmt:
	gofmt -l -w .

lint:
	golangci-lint run ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

ci: vet fmt build test

clean:
	rm -f $(BINARY) coverage.out coverage.html

install: build
	install -Dm755 $(BINARY) $(HOME)/bin/$(BINARY)

hooks:
	@test -d .git || { echo "error: not a git repository"; exit 1; }
	cp scripts/commit-msg .git/hooks/commit-msg
	chmod +x .git/hooks/commit-msg
