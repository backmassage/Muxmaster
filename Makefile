BINARY  := muxmaster
VERSION := 2.0.0-dev
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

.PHONY: build test vet fmt lint docs-naming coverage ci clean install

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd

test:
	go test ./... -v -count=1

vet:
	go vet ./...

fmt:
	gofmt -l -w .

lint:
	golangci-lint run ./...

docs-naming:
	@bash -ceu '\
		errors=0; \
		is_allowed_root_doc() { \
			case "$$1" in README.md|CHANGELOG.md) return 0 ;; *) return 1 ;; esac; \
		}; \
		while IFS= read -r file; do \
			rel="$${file#./}"; \
			dir="$$(dirname "$$rel")"; \
			base="$$(basename "$$rel")"; \
			if [ "$$dir" = "." ]; then \
				if is_allowed_root_doc "$$base"; then \
					continue; \
				fi; \
				echo "invalid root markdown filename: $$rel"; \
				echo "  expected one of: README.md, CHANGELOG.md"; \
				errors=1; \
				continue; \
			fi; \
			if [[ ! "$$base" =~ ^[a-z0-9]+(-[a-z0-9]+)*\.md$$ ]]; then \
				echo "invalid markdown filename: $$rel"; \
				echo "  expected lowercase kebab-case (example: foundation-plan.md)"; \
				errors=1; \
			fi; \
		done < <(rg --files -g "*.md"); \
		if [ "$$errors" -ne 0 ]; then \
			echo; \
			echo "markdown naming check failed"; \
			exit 1; \
		fi; \
		echo "markdown naming check passed"'

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

ci: vet fmt docs-naming build test

clean:
	rm -f $(BINARY) coverage.out coverage.html

install: build
	install -Dm755 $(BINARY) $(HOME)/bin/$(BINARY)
