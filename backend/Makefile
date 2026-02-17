# OpenClaw Dashboard Build System
# Builds lightweight Go binaries for dashboard and all modules

BINARY_DIR = bin
GO = go
GOFLAGS = -ldflags="-s -w" -trimpath
MODULES = system-health session-manager live-feed log-viewer file-manager \
          cost-analyzer rate-limiter memory-viewer service-control cron-manager

.PHONY: all clean dashboard modules $(MODULES) tidy

all: tidy dashboard modules

tidy:
	$(GO) mod tidy

dashboard:
	@echo "Building dashboard server..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(BINARY_DIR)/openclaw-dashboard ./cmd/dashboard/

modules: $(MODULES)

$(MODULES):
	@echo "Building module: $@..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(BINARY_DIR)/module-$(@:module-%=%) ./modules/$@/

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BINARY_DIR)

# Build a specific module
module-%:
	@echo "Building module: $*..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(BINARY_DIR)/module-$* ./modules/$*/

# Show binary sizes
sizes: all
	@echo ""
	@echo "Binary sizes:"
	@ls -lh $(BINARY_DIR)/ | awk '{print $$5, $$9}' | tail -n +2

# Install to system
install: all
	@echo "Installing to /usr/local/bin/openclaw-dashboard..."
	cp $(BINARY_DIR)/openclaw-dashboard /usr/local/bin/
	@mkdir -p /usr/local/lib/openclaw/modules/
	cp $(BINARY_DIR)/module-* /usr/local/lib/openclaw/modules/

# Run dashboard (development)
run: all
	@echo "Starting OpenClaw Dashboard..."
	cd .. && ./backend/$(BINARY_DIR)/openclaw-dashboard --config config/openclaw.config.json
