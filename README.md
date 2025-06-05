# Linkpearl 🔮

[![Go Version](https://img.shields.io/badge/go-1.24.3+-blue.svg)](https://golang.org/dl/)
[![Build Status](https://github.com/Veraticus/linkpearl/actions/workflows/test.yml/badge.svg)](https://github.com/Veraticus/linkpearl/actions/workflows/test.yml)
[![Release](https://img.shields.io/github/release/Veraticus/linkpearl.svg)](https://github.com/Veraticus/linkpearl/releases/latest)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/Veraticus/linkpearl)](https://goreportcard.com/report/github.com/Veraticus/linkpearl)
[![GoDoc](https://pkg.go.dev/badge/github.com/Veraticus/linkpearl)](https://pkg.go.dev/github.com/Veraticus/linkpearl)

> Secure, peer-to-peer clipboard synchronization for your devices

Linkpearl creates a private mesh network between your computers, enabling instant clipboard synchronization. Named after Final Fantasy XIV's magical communication crystals, Linkpearl connects your devices in a secure "linkshell" without any cloud services.

## ✨ Features

- **🔒 Secure by Design** - All data encrypted with TLS 1.3, authenticated with shared secrets
- **🌐 True P2P** - Direct connections between your devices, no central server required
- **🚀 Real-time Sync** - Changes propagate instantly across all connected devices
- **🔄 Resilient** - Handles network disruptions, sleeping computers, and connection drops
- **🖥️ Cross-platform** - Native support for macOS and Linux (Windows coming soon)
- **🎯 Simple** - Single binary, minimal configuration, just works

## 📥 Installation

### Pre-built Binaries

Download the latest release for your platform:

```bash
# macOS (Apple Silicon)
curl -L https://github.com/Veraticus/linkpearl/releases/latest/download/linkpearl-darwin-arm64.tar.gz -o linkpearl.tar.gz
tar -xzf linkpearl.tar.gz
chmod +x linkpearl-darwin-arm64

# macOS (Intel)
curl -L https://github.com/Veraticus/linkpearl/releases/latest/download/linkpearl-darwin-amd64.tar.gz -o linkpearl.tar.gz
tar -xzf linkpearl.tar.gz
chmod +x linkpearl-darwin-amd64

# Linux (x64)
curl -L https://github.com/Veraticus/linkpearl/releases/latest/download/linkpearl-linux-amd64.tar.gz -o linkpearl.tar.gz
tar -xzf linkpearl.tar.gz
chmod +x linkpearl-linux-amd64
```

### From Source

Requires Go 1.24.3 or later:

```bash
git clone https://github.com/Veraticus/linkpearl.git
cd linkpearl
make build

# Install to PATH
sudo make install
```

### NixOS / Nix

Linkpearl can be installed on NixOS or any system with Nix package manager using either flakes or traditional Nix expressions.

#### Using Nix Flakes

```bash
# Run directly without installation
nix run github:Veraticus/linkpearl -- --help

# Install to user profile
nix profile install github:Veraticus/linkpearl

# Or in your flake.nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    linkpearl.url = "github:Veraticus/linkpearl";
  };

  outputs = { self, nixpkgs, linkpearl }: {
    # For NixOS system configuration
    nixosConfigurations.mysystem = nixpkgs.lib.nixosSystem {
      modules = [
        linkpearl.nixosModules.default
        {
          services.linkpearl = {
            enable = true;
            secret = "your-shared-secret";
            listen = ":9437";
            join = [ "other-machine:9437" ];
          };
        }
      ];
    };
  };
}
```

#### Using Traditional Nix

```bash
# Build from source
nix-build -E 'with import <nixpkgs> {}; callPackage ./default.nix {}'

# Install using nix-env
nix-env -f ./default.nix -i

# Or add to configuration.nix using fetchFromGitHub
{ pkgs, ... }:
let
  linkpearl = pkgs.callPackage (pkgs.fetchFromGitHub {
    owner = "Veraticus";
    repo = "linkpearl";
    rev = "20b21640831c906ca5129f23975bdf647e0a8d92";
    sha256 = "sha256-067iq6w95qki79c2adsnf6p7m9p94f74fi5i1ysniqwnz8p972sm";
  } + "/default.nix") { };
in
{
  environment.systemPackages = [ linkpearl ];
}

# Or for development from local checkout
{ pkgs, ... }:
let
  linkpearl = pkgs.callPackage /path/to/linkpearl/default.nix { 
    src = /path/to/linkpearl;
  };
in
{
  environment.systemPackages = [ linkpearl ];
}
```

#### Development Shell

```bash
# Enter development environment with all dependencies
nix develop

# Or without flakes
nix-shell -E 'with import <nixpkgs> {}; mkShell { buildInputs = [ go_1_24 gnumake ]; }'
```

#### NixOS Service Configuration

Linkpearl includes a NixOS module for running as a systemd user service:

```nix
# In your configuration.nix or home-manager config
{
  services.linkpearl = {
    enable = true;
    # Option 1: Direct secret (simple but less secure)
    secret = "your-shared-secret";
    
    # Option 2: Secret from file (recommended)
    # secretFile = "/run/secrets/linkpearl-secret";
    
    listen = ":9437";               # Optional: for full nodes
    join = [                        # Optional: peers to connect to
      "desktop.local:9437"
      "server.example.com:9437"
    ];
    nodeId = "laptop";              # Optional: custom node ID
    pollInterval = "500ms";         # Optional: clipboard check interval
    verbose = true;                 # Optional: enable debug logging
  };
}
```

For secret management, consider using [agenix](https://github.com/ryantm/agenix) or [sops-nix](https://github.com/Mic92/sops-nix):

```nix
{
  services.linkpearl = {
    enable = true;
    secretFile = config.age.secrets.linkpearl-secret.path;
    # ... other options
  };
}
```

#### Home Manager Configuration

For user-level client configurations (no listening, only joining), use the home-manager module:

```nix
# In your home.nix or home-manager configuration
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    home-manager.url = "github:nix-community/home-manager";
    linkpearl.url = "github:Veraticus/linkpearl";
  };

  outputs = { self, nixpkgs, home-manager, linkpearl }: {
    homeConfigurations.myuser = home-manager.lib.homeManagerConfiguration {
      modules = [
        linkpearl.homeManagerModules.default
        {
          services.linkpearl = {
            enable = true;
            
            # Choose one:
            secret = "your-shared-secret";
            # secretFile = "${config.home.homeDirectory}/.config/linkpearl/secret";
            
            # Required for client mode
            join = [
              "desktop.local:9437"
              "server.example.com:9437"
            ];
            
            # Optional settings
            nodeId = "laptop-client";
            pollInterval = "500ms";
            verbose = false;
          };
        }
      ];
    };
  };
}
```

The home-manager module:
- Runs as a user systemd service
- Requires `join` addresses (client-only mode)
- Starts after graphical session is available
- Automatically installs the linkpearl package

## 🚀 Quick Start

### Two Computer Setup

On your desktop (server):
```bash
# Using command line secret (simple but less secure)
linkpearl --secret "your-shared-secret" --listen :9437

# Or using a secret file (recommended)
echo "your-shared-secret" > ~/.linkpearl-secret
chmod 600 ~/.linkpearl-secret
linkpearl --secret-file ~/.linkpearl-secret --listen :9437
```

On your laptop (client):
```bash
# Using the same secret
linkpearl --secret "your-shared-secret" --join desktop.local:9437

# Or with secret file
linkpearl --secret-file ~/.linkpearl-secret --join desktop.local:9437
```

That's it! Copy text on one device and paste on the other.

### Multiple Computers

Create a mesh network with multiple devices:

```bash
# Desktop (full node - accepts connections)
linkpearl --secret "mysecret" --listen :9437

# Laptop (full node - accepts and makes connections)
linkpearl --secret "mysecret" --listen :9437 --join desktop.local:9437

# Work machine (client node - only makes connections)
linkpearl --secret "mysecret" --join desktop.local:9437 --join laptop.local:9437
```

## 📖 Usage

### Command Line Flags

```
linkpearl [flags]

Flags:
  --secret string       Shared secret for authentication (required if --secret-file not used)
  --secret-file string  Path to file containing the shared secret (required if --secret not used)
  --listen string       Address to listen on (recommended: :9437)
  --join strings        Addresses to connect to (can be repeated)
  --node-id string      Unique node identifier (default: hostname-timestamp)
  --poll-interval       Clipboard check interval (default: 500ms)
  -v, --verbose         Enable verbose logging
  -h, --help            Show help message
  
Note: Either --secret or --secret-file must be provided, but not both.
```

### Environment Variables

All flags can be set via environment variables:

```bash
export LINKPEARL_SECRET="your-shared-secret"
export LINKPEARL_SECRET_FILE="/path/to/secret.txt"
export LINKPEARL_LISTEN=":9437"
export LINKPEARL_JOIN="server1:9437,server2:9437"
export LINKPEARL_VERBOSE=true

linkpearl  # Uses environment variables
```

### Node Types

Linkpearl supports two node types:

- **Full Node**: Can accept incoming connections and make outbound connections
  - Use when: Device has a stable network address
  - Example: Desktop computer, always-on server
  
- **Client Node**: Only makes outbound connections
  - Use when: Device is behind NAT, uses dynamic IPs, or is mobile
  - Example: Laptop, work computer behind firewall

## 🔧 Configuration Examples

### Home Network Setup

```bash
# Desktop in home office
linkpearl --secret "home-secret" --listen :9437

# Laptop anywhere in house
linkpearl --secret "home-secret" --join desktop.local:9437
```

### Remote Work Setup

```bash
# Home desktop (behind router)
linkpearl --secret "work-secret" --listen :9437

# Work laptop (behind corporate firewall)  
linkpearl --secret "work-secret" --join home.example.com:9437

# Note: Requires port forwarding on home router
```

### Multi-Site Setup

```bash
# Server A (full node)
linkpearl --secret "team-secret" --listen :9437

# Server B (full node) 
linkpearl --secret "team-secret" --listen :9437 --join server-a:9437

# Client devices
linkpearl --secret "team-secret" --join server-a:9437 --join server-b:9437
```

## 🔒 Security

### Threat Model

Linkpearl protects against:
- **Network eavesdropping**: All traffic encrypted with TLS 1.3
- **Unauthorized access**: Shared secret authentication required
- **Replay attacks**: Timestamp validation with 5-minute window
- **Man-in-the-middle**: HMAC verification before TLS handshake

### Best Practices

1. **Use strong secrets**: Generate with `openssl rand -base64 32`
2. **Rotate secrets regularly**: Change monthly for sensitive environments  
3. **Limit network exposure**: Use firewalls to restrict access
4. **Monitor connections**: Use `--verbose` to see all peer connections

### Secure Secret Management

To avoid exposing secrets in process lists or shell history, use the `--secret-file` option:

```bash
# Generate a strong secret
openssl rand -base64 32 > ~/.linkpearl-secret
chmod 600 ~/.linkpearl-secret

# Use the secret file
linkpearl --secret-file ~/.linkpearl-secret --listen :9437

# Or via environment variable
export LINKPEARL_SECRET_FILE=~/.linkpearl-secret
linkpearl --listen :9437
```

This approach is recommended for production deployments and integrates well with:
- Kubernetes Secrets (mounted as files)
- HashiCorp Vault (via agent injection)
- systemd credentials (`LoadCredential=`)
- Docker secrets

### What's NOT Encrypted

- Clipboard data at rest (in system clipboard)
- Command line arguments (secret visible in process list unless using `--secret-file`)

## 🛠️ Troubleshooting

### Common Issues

**"Connection refused" error**
- Check firewall settings
- Verify the listening address is correct
- Ensure the port is not already in use

**Clipboard not syncing**
- Verify same secret on all nodes
- Check network connectivity with ping
- Look for errors with `--verbose` flag

**High CPU usage**
- Increase poll interval: `--poll-interval 2s`
- On Linux, install `clipnotify` for efficient monitoring

### Platform-Specific

**Linux clipboard tools**
```bash
# X11
sudo apt install xsel    # or xclip

# Wayland  
sudo apt install wl-clipboard

# Efficient monitoring
sudo apt install clipnotify
```

**macOS permissions**
- Grant Terminal/iTerm2 accessibility permissions
- System Preferences → Security & Privacy → Privacy → Accessibility

## 🏗️ Architecture

Linkpearl uses a peer-to-peer mesh topology:

```
┌─────────┐         ┌─────────┐
│ Laptop  │◄───────►│ Desktop │
└─────────┘         └─────────┘
     ▲                   ▲
     │                   │
     └──────┬────────────┘
            │
       ┌─────────┐
       │ Server  │
       └─────────┘
```

Key components:
- **Clipboard Monitor**: Platform-specific clipboard access
- **Transport Layer**: Authenticated TLS connections
- **Mesh Network**: Resilient P2P topology with automatic reconnection
- **Sync Engine**: Conflict-free clipboard synchronization

For detailed architecture documentation, see [ARCHITECTURE.md](ARCHITECTURE.md).

## 📊 Performance

- **Latency**: Sub-100ms on local networks
- **Memory**: ~10MB baseline, grows with peer count
- **CPU**: <1% idle, spikes during clipboard changes
- **Network**: Minimal traffic, only clipboard deltas transmitted

## 🤝 Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Development Setup

```bash
# Clone repository
git clone https://github.com/Veraticus/linkpearl.git
cd linkpearl

# Install dependencies
go mod download

# Run tests
make test

# Run with race detector
make test-race

# Build binary
make build
```

## 📜 License

Linkpearl is licensed under the MIT License. See [LICENSE](LICENSE) for details.

## 🙏 Acknowledgments

- Named after the communication system in [Final Fantasy XIV](https://finalfantasyxiv.com)
- Inspired by [Teleport](https://github.com/abyssoft/teleport) and [Barrier](https://github.com/debauchee/barrier)
- Built with [Go](https://golang.org) and love for simple, secure tools

## 📮 Support

- **Issues**: [GitHub Issues](https://github.com/Veraticus/linkpearl/issues)
- **Discussions**: [GitHub Discussions](https://github.com/Veraticus/linkpearl/discussions)
- **Security**: Report vulnerabilities to security@example.com

---

Made with ❤️ for the clipboard warriors who refuse to email themselves text
