.PHONY: all build test cover fmt lint vet check clean

# `make` with no target builds the binary, exactly what a contributor
# expects from a Go project.
all: build

# build produces the chronicle binary at the repo root. The output
# location matches the .gitignore so the binary stays out of version
# control.
build:
	go build -o chronicle ./cmd/chronicle

# test runs every test in the module with the race detector enabled.
# The race detector slows tests down a little but catches data-race
# bugs the regular runner would miss. Worth the cost for the small
# test suite we have.
test:
	go test -race ./...

# cover prints test coverage per package and writes the full HTML
# report to coverage.html. Open that file in a browser to see which
# lines are exercised. Useful when you want to know whether a new
# function actually has a test.
cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -20
	go tool cover -html=coverage.out -o coverage.html
	@echo "Open coverage.html in a browser for line-by-line details."

# fmt rewrites every Go file in the canonical style. We run this in
# CI to catch drift, and contributors should run it before they
# commit. Most editors run gofmt on save, so this target rarely
# changes anything in practice.
fmt:
	gofmt -w .

# vet runs Go's built-in static checks. It catches things like
# Printf format mismatches, unreachable code, and shadowed
# variables. Cheap to run and worth the safety net.
vet:
	go vet ./...

# GOBIN is where `go install` puts compiled tools. We compute it
# once and reuse it in the lint target, so contributors who do not
# have ~/go/bin on their PATH can still run `make lint`.
GOBIN := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif

# lint runs golangci-lint with the project's .golangci.yml config.
# It is a meta-linter that runs many smaller linters at once. We
# install it on demand if it is missing, so a fresh contributor can
# run `make lint` without any setup.
lint:
	@test -x "$(GOBIN)/golangci-lint" || ( \
	  echo "Installing golangci-lint..."; \
	  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	)
	$(GOBIN)/golangci-lint run

# check is the all-in-one gate. CI runs this, and contributors
# should run it before opening a pull request. Failing here is the
# same as failing CI.
check: fmt vet lint test

# clean removes build artifacts. Coverage files and the local
# binary all live at the repo root and we sweep them with one
# command.
clean:
	rm -f chronicle coverage.out coverage.html
