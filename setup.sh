#!/usr/bin/env bash

# Install required Go packages
echo "Installing required Go packages..."
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Add Go bin to PATH temporarily if not already there
export PATH="$PATH:$(go env GOPATH)/bin"

# Initialize go module if not already initialized
if [ ! -f "go.mod" ]; then
    echo "Initializing Go module..."
    go mod init github.com/cryptexus/go-tunnel
fi

# Download and tidy dependencies
echo "Getting dependencies..."
go get -u google.golang.org/grpc
go get -u google.golang.org/protobuf
go get -u github.com/spf13/cobra
go get -u golang.org/x/crypto/ssh
go get -u google.golang.org/grpc/credentials/insecure
go mod tidy

# Generate protobuf code
echo "Generating protobuf code..."
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    internal/proto/tunnel.proto

# Verify dependencies again after proto generation
echo "Verifying dependencies..."
go mod tidy

# Build the binaries
echo "Building tunneld daemon..."
go build -o tunneld cmd/tunneld/main.go

echo "Building tunnel CLI..."
go build -o tunnel cmd/tunnel/main.go

# Make sure the socket file doesn't exist
rm -f /tmp/tunnel.sock

echo "Build complete!"
echo
echo "To use the program:"
echo "1. First start the daemon in one terminal:    ./tunneld"
echo "2. Then in another terminal, you can run:"
echo "   - Create a tunnel:     ./tunnel example.com 8080:80"
echo "   - List active tunnels: ./tunnel list"
echo "   - Close a tunnel:      ./tunnel close example.com 80"
echo
echo "Note: Make sure you have SSH keys set up (~/.ssh/id_rsa) for the target machines."
