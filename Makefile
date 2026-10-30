GO ?= go
GOLANGCI_LINT ?= golangci-lint

.PHONY: build test e2e lint trace image clean

build:
	$(GO) build -o bin/parachute ./cmd/parachute

test:
	$(GO) test ./...

E2E_ARGS :=
ifdef E2E_RUN
E2E_ARGS += -run '$(E2E_RUN)'
endif
ifdef E2E_SKIP
E2E_ARGS += -skip '$(E2E_SKIP)'
endif

e2e:
	cd e2e && $(GO) test -count=1 -timeout 30m $(E2E_ARGS) ./...

lint:
	test -z "$$(gofmt -l .)"
	$(GO) vet ./...
	cd e2e && $(GO) vet ./...
	$(GOLANGCI_LINT) run ./...
	cd e2e && $(GOLANGCI_LINT) run ./...
	! grep -rnE '(^|[[:space:]])//|/\*' --include='*.go' . | grep -v '//go:'

trace:
	$(GO) run ./tools/trace

image:
	docker build -t parachute:dev .

clean:
	rm -rf bin
