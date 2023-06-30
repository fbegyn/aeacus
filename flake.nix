{
  description = "aeacus";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    devshell = {
      url = "github:numtide/devshell";
      inputs = {
        flake-utils.follows = "flake-utils";
        nixpkgs.follows = "nixpkgs";
      };
    };
  };

  outputs = { self, nixpkgs, devshell, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; overlays = [ devshell.overlays.default ]; };
      in
      {
        devShells = rec {
          default = aeacus;
          aeacus = pkgs.devshell.mkShell {
            name = "aeacus";
            packages = [
              pkgs.vault-bin
              pkgs.bitwarden
              pkgs.nixpkgs-fmt
              pkgs.go_1_20
            ];
            env = [
              {
                name = "DATA";
                eval = "$PWD/tmp/vault";
              }
              {
                name = "VAULT_TOKEN";
                eval = "foo-bar-root-token";
              }
            ];
            commands = [
              {
                name = "create:dev";
                category = "database";
                help = "create vault tmp data dir";
                command = ''
                  mkdir $PWD/tmp/vault
                '';
              }
              {
                name = "vault:start";
                category = "database";
                help = "Start vault dev instance";
                command = ''
                  vault server -dev -dev-root-token-id foo-bar-root-token
                '';
              }
            ];
          };
        };
      }
    );
}
