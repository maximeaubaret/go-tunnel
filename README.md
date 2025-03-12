
# go-tunnel

A Go-based SSH tunneling tool.

## Building with Nix

### Prerequisites

1. Install Nix with flakes enabled:
```bash
sh <(curl -L https://nixos.org/nix/install) --daemon
```

2. Enable flakes by editing either `~/.config/nix/nix.conf` or `/etc/nix/nix.conf`:
```
experimental-features = nix-command flakes
```

### Development

1. Enter the development shell:
```bash
nix develop
```
2. Build the binaries:
```bash
go build ./cmd/tunnel
go build ./cmd/tunneld
```

### Building with Nix

To build and install the binaries:

```bash
# Build only
nix build

# Install to your profile
nix profile install

# Run without installing
nix run
```

