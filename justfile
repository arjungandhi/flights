# Build the flights binary
build:
    go build ./cmd/flights

# Run all tests
test:
    go test ./...

# Run the server against a throwaway demo database
demo:
    FLIGHTS_DIR=$(mktemp -d) go run ./cmd/flights --demo --addr :8080

# Install the binary
install:
    go install ./cmd/flights

# Build with Nix
nix-build:
    nix build

# Format code
fmt:
    go fmt ./...

# Vet code
vet:
    go vet ./...

# Clean build artifacts
clean:
    go clean
