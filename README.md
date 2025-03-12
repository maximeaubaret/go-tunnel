# Go SSH Tunnel Manager

A simple and efficient SSH tunnel manager written in Go, featuring a daemon service and CLI interface.

## Overview

The program consists of two main components:
1. `tunneld` - A daemon that manages SSH tunnels
2. `tunnel` - A CLI tool to interact with the daemon

## Quick Start

The easiest way to set up the program is to use the provided setup script:

```bash
# Make the setup script executable
chmod +x setup.sh

# Run the setup script
./setup.sh
```

## Manual Setup

If you prefer to set up manually, you'll need:

1. Go 1.16 or later
2. Protocol Buffer compiler (protoc)

### Installation Steps

1. Install the Protocol Buffer compiler:
   ```bash
   # Ubuntu/Debian
   sudo apt install protobuf-compiler

   # Fedora
   sudo dnf install protobuf-compiler

   # macOS
   brew install protobuf
   ```

2. Clone the repository:
   ```bash
   git clone https://github.com/cryptexus/go-tunnel.git
   cd go-tunnel
   ```

3. Install Go dependencies and build:
   ```bash
   go mod download
   go mod tidy
   go build -o tunneld cmd/tunneld/main.go
   go build -o tunnel cmd/tunnel/main.go
   ```

## SSH Key Configuration

The program supports multiple SSH key types and locations:

1. Default locations (tried in order):
   - `~/.ssh/id_ed25519` (preferred)
   - `~/.ssh/id_rsa`
   - `~/.ssh/id_ecdsa`

2. Custom key location:
   ```bash
   export SSH_KEY_PATH="/path/to/your/key"
   ```

3. For encrypted keys:
   ```bash
   export SSH_KEY_PASSPHRASE="your-passphrase"
   ```

## Usage

1. Start the daemon:
   ```bash
   ./tunneld
   ```

2. In another terminal, use the CLI:
   ```bash
   # Create a tunnel (local:remote ports)
   ./tunnel example.com 8080:80

   # Create a tunnel (same port locally and remotely)
   ./tunnel example.com 80

   # List active tunnels
   ./tunnel list

   # Close a tunnel
   ./tunnel close example.com 80
   ```

## Common Use Cases

1. Access a remote web server locally:
   ```bash
   ./tunnel webserver.example.com 8080:80
   # Now access the remote web server at localhost:8080
   ```

2. Connect to a remote database:
   ```bash
   ./tunnel dbserver.example.com 5432
   # Now connect to the database at localhost:5432
   ```

## Troubleshooting

1. If the daemon won't start:
   - Check if `/tmp/tunnel.sock` exists and remove it
   - Ensure you have proper permissions
   - Check SSH key permissions (should be 600)

2. If tunnels fail to create:
   - Verify SSH key exists and is valid
   - Check if the remote host is accessible
   - Ensure the remote port is available

3. SSH key issues:
   - Run `tunneld` with verbose logging to see which keys are being tried
   - Check key permissions (should be 600)
   - For encrypted keys, ensure SSH_KEY_PASSPHRASE is set correctly

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
