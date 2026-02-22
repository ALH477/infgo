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
        goVersion = pkgs.go_1_26;
      in {
        # ── Dev shell (recommended for development) ─────────────────────────────
        devShells.default = pkgs.mkShell {
          name = "infgo-dev";

          buildInputs = with pkgs; [
            goVersion
            gopls
            gotools
            golangci-lint
            delve
          ];

          shellHook = ''
            echo ""
            echo "    Infgo dev shell"
            echo "  Go $(go version | awk '{print $3}')"
            echo ""
            echo "  Commands:"
            echo "    go run .              — run in place"
            echo "    go build -o infgo .  — build binary"
            echo ""
          '';
        };
      }
    );
}
