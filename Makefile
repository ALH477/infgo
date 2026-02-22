# Makefile for infgo
#
# Targets:
#   make build     — compile both binaries into ./bin/
#   make proto     — regenerate metrics/metrics.pb.go from proto/metrics.proto
#   make run       — run the TUI without logging
#   make run-log   — run the TUI with logging to /tmp/session.infgo
#   make analyze   — analyze the most recent /tmp/session.infgo log
#   make lint      — run golangci-lint
#   make tidy      — go mod tidy
#   make clean     — remove build artefacts

.PHONY: build proto run run-log analyze lint tidy clean

BINARY_DIR  := ./bin
INFGO      := $(BINARY_DIR)/infgo
ANALYZE     := $(BINARY_DIR)/analyze
LOG_FILE    := /tmp/session.infgo

# ── Build ─────────────────────────────────────────────────────────────────────

build: $(INFGO) $(ANALYZE)

$(INFGO): go.mod $(shell find . -name '*.go' -not -path './cmd/*')
	@mkdir -p $(BINARY_DIR)
	go build -ldflags="-s -w" -o $@ .

$(ANALYZE): go.mod $(shell find ./cmd/analyze -name '*.go') $(shell find ./metrics -name '*.go') $(shell find ./logger -name '*.go')
	@mkdir -p $(BINARY_DIR)
	go build -ldflags="-s -w" -o $@ ./cmd/analyze

# ── Protobuf code generation ──────────────────────────────────────────────────
# Requires: protoc + protoc-gen-go
#   brew install protobuf
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#
# NOTE: The hand-authored metrics/metrics.go is the primary encoding
# implementation.  Regenerating will produce metrics/metrics.pb.go alongside
# it, which you can optionally use instead (update imports accordingly).

proto:
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go_opt=Mproto/metrics.proto=github.com/charmbracelet/infgo/metrics \
		proto/metrics.proto
	@echo "Generated: metrics/metrics.pb.go"
	@echo "NOTE: metrics/metrics.go is the active hand-authored implementation."
	@echo "      You may delete metrics.go and use the generated file instead."

# ── Developer workflow ────────────────────────────────────────────────────────

run: $(INFGO)
	$(INFGO)

run-log: $(INFGO)
	$(INFGO) -log $(LOG_FILE)

analyze: $(ANALYZE) $(LOG_FILE)
	$(ANALYZE) $(LOG_FILE)

# ── Code quality ──────────────────────────────────────────────────────────────

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

# ── Cleanup ───────────────────────────────────────────────────────────────────

clean:
	rm -rf $(BINARY_DIR)
	rm -f $(LOG_FILE) /tmp/*.infgo /tmp/*_report.png
