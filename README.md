# go-tunnel

A Go-based SSH tunneling tool with real-time monitoring capabilities.

## Features

- Create multiple SSH tunnels simultaneously
- Real-time bandwidth monitoring
- Connection tracking and statistics
- Automatic reconnection on failure
- Watch mode for live updates
- Color-coded status output

## Usage

### Starting the Daemon

First, start the tunnel daemon:

```bash
tunneld
```

### Creating Tunnels

Create a tunnel with automatic port mapping:
```bash
tunnel server1 8080                    # Local 8080 to remote 8080
```

Create a tunnel with custom port mapping:
```bash
tunnel server1 8080:80                # Local 8080 to remote 80
```

Create multiple tunnels at once:
```bash
tunnel server1 8080 9090 3000:3001    # Multiple tunnels
```

### Managing Tunnels

List all active tunnels:
```bash
tunnel list
```

Monitor tunnels in real-time:
```bash
tunnel list --watch
# or
tunnel list -w
```

Close a specific tunnel:
```bash
tunnel close server1 8080
```

Close all active tunnels:
```bash
tunnel closeall
```

## Authentication

The tool uses your SSH configuration and keys from `~/.ssh/`. You can:

- Use default SSH keys (id_ed25519, id_rsa, id_ecdsa)
- Specify a custom key with `SSH_KEY_PATH` environment variable
- Use an encrypted key by setting `SSH_KEY_PASSPHRASE` environment variable

## Monitoring Features

The watch mode (`tunnel list -w`) displays:
- Active connections
- Total data transferred
- Current bandwidth (up/down)
- Uptime and last activity
- Connection counts

## Notes

- The daemon creates a Unix socket at `/tmp/tunnel.sock`
- Automatic reconnection on network issues
- Bandwidth statistics are updated in real-time
- Color-coded output for better visibility

## NixOS Usage

### Quick Start with `nix run`

To try the tool without installation:

```bash
nix run github:cryptexus/go-tunnel
```

### NixOS Configuration

Add to your NixOS configuration:

```nix
{
  inputs.go-tunnel.url = "github:cryptexus/go-tunnel";

  outputs = { self, nixpkgs, go-tunnel, ... }: {
    nixosConfigurations.your-hostname = nixpkgs.lib.nixosSystem {
      # ... your other config ...
      modules = [
        # ... your other modules ...
        {
          environment.systemPackages = [ go-tunnel.packages.${system}.default ];
          
          # Optional: Run tunneld as a service
          systemd.services.tunneld = {
            description = "SSH Tunnel Daemon";
            wantedBy = [ "multi-user.target" ];
            after = [ "network.target" ];
            
            serviceConfig = {
              ExecStart = "${go-tunnel.packages.${system}.default}/bin/tunneld";
              Restart = "always";
              RestartSec = "10";
              User = "your-username"; # Replace with your username
            };
          };
        }
      ];
    };
  };
}
```

Or add to your home-manager configuration:

```nix
{
  inputs.go-tunnel.url = "github:cryptexus/go-tunnel";

  outputs = { self, nixpkgs, home-manager, go-tunnel, ... }: {
    homeConfigurations.your-username = home-manager.lib.homeManagerConfiguration {
      # ... your other config ...
      modules = [
        {
          home.packages = [ go-tunnel.packages.${system}.default ];
          
          # Optional: Run tunneld as a user service
          systemd.user.services.tunneld = {
            Unit = {
              Description = "SSH Tunnel Daemon";
              After = [ "network.target" ];
            };
            
            Service = {
              ExecStart = "${go-tunnel.packages.${system}.default}/bin/tunneld";
              Restart = "always";
              RestartSec = "10";
            };
            
            Install = {
              WantedBy = [ "default.target" ];
            };
          };
        }
      ];
    };
  };
}
