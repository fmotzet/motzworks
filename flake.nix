{
  description = "motzworks — agentless network inventory scanner";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f (import nixpkgs { inherit system; }));
    in
    {
      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          name = "motzworks-dev";

          packages = with pkgs; [
            go               # 1.26.x — matches go.mod
            gopls            # language server
            gotools          # goimports etc.
            go-tools         # staticcheck
            golangci-lint    # linter
            delve            # debugger
            nodejs           # React/TS dashboard (Phase 2)
            postgresql_16    # psql client for local dev DB
            openssl          # generating MOTZWORKS_AUTH_SECRET (see docs/DEPLOY.md)
            git
          ];

          shellHook = ''
            echo "motzworks dev shell — $(go version | cut -d' ' -f3)"
            echo "  docker compose up -d   # start Postgres"
            echo "  go run ./cmd/motzworks migrate up"
          '';
        };
      });
    };
}
