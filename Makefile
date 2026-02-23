BINARY  := muxmaster
VERSION := 2.0.0-dev
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

.PHONY: build test vet ci clean install

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/muxmaster

test:
	go test ./... -v -count=1

vet:
	go vet ./...

ci: vet build test

clean:
	rm -f $(BINARY)

install: build
	install -Dm755 $(BINARY) $(HOME)/bin/$(BINARY)
