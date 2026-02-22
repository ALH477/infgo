{
  description = "Infgo — real-time system resource monitor (Bubble Tea + gopsutil)";

  inputs = {
    nixpkgs.url     = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };

        # ── Go version ──────────────────────────────────────────────────────
        goVersion = pkgs.go_1_26;

        # ── Application derivation ─────────────────────────────────────────
        infgo = pkgs.buildGoModule {
          pname   = "infgo";
          version = "0.1.0";

          src = ./.;

          # Run `gomod2nix` or replace with the actual hash after first build:
          #   nix build 2>&1 | grep "got:" | awk '{print $2}'
          vendorHash = pkgs.lib.fakeHash;

          # gopsutil needs CGO on some platforms for cpu/mem stats
          CGO_ENABLED = 0;

          ldflags = [
            "-s" "-w"
            "-X main.version=0.1.0"
          ];

          meta = with pkgs.lib; {
            description = "Real-time CPU & RAM monitor built with Bubble Tea";
            homepage    = "https://github.com/yourorg/infgo";
            license     = licenses.mit;
            maintainers = [];
            mainProgram = "infgo";
          };
        };

      in {
        # ── Packages ────────────────────────────────────────────────────────
        packages = {
          infgo  = infgo;
          default = infgo;
        };

        # ── Apps (nix run) ───────────────────────────────────────────────────
        apps.default = flake-utils.lib.mkApp { drv = infgo; };

        # ── Dev shell ────────────────────────────────────────────────────────
        devShells.default = pkgs.mkShell {
          name = "infgo-dev";

          buildInputs = with pkgs; [
            goVersion           # Go toolchain
            gopls               # Language server
            gotools             # goimports, godoc, etc.
            golangci-lint       # Linter suite
            delve               # Debugger
            air                 # Live-reload for TUI dev
          ];

          shellHook = ''
            echo ""
            echo "    Infgo dev shell"
            echo "  Go $(go version | awk '{print $3}')"
            echo ""
            echo "  Commands:"
            echo "    go run .              — run in place"
            echo "    go build -o infgo .  — build binary"
            echo "    air                   — live-reload (install .air.toml first)"
            echo "    nix build             — reproducible build"
            echo ""
          '';

          # Ensure CGO is explicitly off so pure Go paths are taken
          CGO_ENABLED = "0";
        };
      }
    );
}
