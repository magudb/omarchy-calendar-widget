# Calendar Widget Makefile

# Build variables
BINARY_NAME=calendar-widget
BUILD_DIR=build
INSTALL_PATH=/usr/local/bin

# Go variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Git variables
VERSION=$(shell git describe --tags --always --dirty)
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

.PHONY: build clean test install uninstall deps tidy manifest help

# Default target
all: build

# Build the binary
build:
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Built $(BINARY_NAME) in $(BUILD_DIR)/"
	chmod +x $(BUILD_DIR)/$(BINARY_NAME)
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "Copied $(BINARY_NAME) to /usr/local/bin/"

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	@echo "Cleaned build directory"

# Run tests
test:
	$(GOTEST) -v ./...

# Install to system
install: build
	@echo "Installing $(BINARY_NAME) to $(INSTALL_PATH)"
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_PATH)/
	sudo chmod +x $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "Installation complete"

# Uninstall from system
uninstall:
	@echo "Removing $(BINARY_NAME) from $(INSTALL_PATH)"
	sudo rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "Uninstallation complete"

# Download dependencies
deps:
	$(GOGET) ./...

# Tidy up go.mod
tidy:
	$(GOMOD) tidy

# Sync the root manifest.json (for `omarchy plugin add <repo-url>`)
# with the canonical quickshell/manifest.json.
manifest:
	cp quickshell/manifest.json manifest.json
	@echo "Synced manifest.json from quickshell/manifest.json"

# Setup development environment
setup: deps
	@echo "Setting up development environment..."
	@mkdir -p $(BUILD_DIR)
	@echo "Development environment ready"

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	@echo "Multi-platform build complete"

# Run the widget in development mode
run:
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) . && $(BUILD_DIR)/$(BINARY_NAME)

# Setup Azure AD app (requires manual steps)
setup-azure:
	@echo "Azure AD Setup Instructions:"
	@echo "1. Go to https://portal.azure.com"
	@echo "2. Navigate to Azure Active Directory -> App registrations"
	@echo "3. Click 'New registration'"
	@echo "4. Name: 'Calendar Widget'"
	@echo "5. Redirect URI: http://localhost:8080/auth/callback (Web)"
	@echo "6. Copy Client ID and Tenant ID"
	@echo "7. Run: ./$(BUILD_DIR)/$(BINARY_NAME) setup"

# Display help
help:
	@echo "Available targets:"
	@echo "  build       - Build the binary"
	@echo "  clean       - Clean build artifacts"
	@echo "  test        - Run tests"
	@echo "  install     - Install to system (/usr/local/bin)"
	@echo "  uninstall   - Remove from system"
	@echo "  deps        - Download dependencies"
	@echo "  tidy        - Tidy go.mod file"
	@echo "  manifest    - Sync root manifest.json with quickshell/manifest.json"
	@echo "  setup       - Setup development environment"
	@echo "  build-all   - Build for multiple platforms"
	@echo "  run         - Build and run the widget"
	@echo "  setup-azure - Show Azure AD setup instructions"
	@echo "  help        - Show this help message"