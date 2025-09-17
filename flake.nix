{
  inputs.nixpkgs.url = "github:NixOs/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      lib = nixpkgs.lib;

      forSystem =
        f: system:
        f {
          inherit system;
          pkgs = import nixpkgs { inherit system; };
        };
      supportedSystems = lib.platforms.unix;
      forAllSystems = f: lib.genAttrs supportedSystems (forSystem f);
      forLinuxSystems = f: lib.genAttrs lib.platforms.linux (forSystem f);
    in
    {

      packages =
        lib.recursiveUpdate
          (forAllSystems (
            { system, pkgs }:
            {
              stove8s = pkgs.callPackage ./nix/package.nix { };
              default = self.packages.${system}.stove8s;
            }
          ))
          (
            forLinuxSystems (
              { system, pkgs }:
              {
                oci = pkgs.callPackage ./nix/oci.nix {
                  stove8s = self.packages.${system}.stove8s;
                };
              }
            )
          );

      devShells = forAllSystems (
        { system, pkgs }:
        {
          stove8s = pkgs.callPackage ./nix/shell.nix {
            stove8s = self.packages.${system}.stove8s;
          };
          default = self.devShells.${system}.stove8s;
        }
      );
    };
}
