.PHONY: all test test-race cover bench vet fmt lint tidy clean

GO ?= go
GOLANGCI_LINT ?= golangci-lint
COVER_PROFILE ?= coverage.out

all: fmt vet lint test

test:
	$(GO) test -v ./...

test-race:
	$(GO) test -race -v ./...

cover:
	$(GO) test -race -coverprofile=$(COVER_PROFILE) -covermode=atomic ./...
	$(GO) tool cover -func=$(COVER_PROFILE)

bench:
	$(GO) test -bench=. -benchmem -run=^$$ ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

lint:
	$(GOLANGCI_LINT) run ./...

tidy:
	$(GO) mod tidy

clean:
	$(GO) clean -testcache
	rm -f $(COVER_PROFILE)
