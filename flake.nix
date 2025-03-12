{
  description = "go-tunnel: A Go-based SSH tunneling tool";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages = rec {
          tunnel-cli = pkgs.buildGoModule {
            pname = "tunnel";
            version = "0.1.0";
            src = ./.;

            vendorHash = "sha256-1p/Hcqig5YgILDtdSdc0EozsK3prgnnpAo8MTbjwWo0=";
            proxyVendor = true;

            # Add git to build inputs for module fetching
            nativeBuildInputs = with pkgs; [
              protoc-gen-go
              protoc-gen-go-grpc
              protobuf
            ];

            preBuild = ''
              # Generate protobuf code
              protoc --go_out=. --go_opt=paths=source_relative \
                --go-grpc_out=. --go-grpc_opt=paths=source_relative \
                internal/proto/tunnel.proto
            '';

            # Build both binaries with version info
            postBuild = ''
              go build -ldflags "-X github.com/maximeaubaret/go-tunnel/internal/version.Version=$pname-$version -X github.com/maximeaubaret/go-tunnel/internal/version.Commit=$(git rev-parse --short HEAD) -X github.com/maximeaubaret/go-tunnel/internal/version.Date=$(date -u +%Y-%m-%d)" -o $GOPATH/bin/tunnel ./cmd/tunnel
              go build -ldflags "-X github.com/maximeaubaret/go-tunnel/internal/version.Version=$pname-$version -X github.com/maximeaubaret/go-tunnel/internal/version.Commit=$(git rev-parse --short HEAD) -X github.com/maximeaubaret/go-tunnel/internal/version.Date=$(date -u +%Y-%m-%d)" -o $GOPATH/bin/tunneld ./cmd/tunneld
            '';

            # Install both binaries
            installPhase = ''
              mkdir -p $out/bin
              cp $GOPATH/bin/tunnel $out/bin/
              cp $GOPATH/bin/tunneld $out/bin/
            '';
          };

          default = tunnel-cli;
        };

        apps = rec {
          tunnel = {
            type = "app";
            program = "${self.packages.${system}.tunnel-cli}/bin/tunnel";
          };
          tunneld = {
            type = "app";
            program = "${self.packages.${system}.tunnel-cli}/bin/tunneld";
          };
          default = tunnel;
        };

        devShells.default = pkgs.mkShell {
            buildInputs = with pkgs; [
              go_1_23
              gopls
              go-tools
              protobuf
            ];

          shellHook = ''
            echo "go-tunnel development shell"
            echo "Go version: $(go version)"
          '';
        };
      }
    );
}

