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
              stove8s-controller = pkgs.callPackage ./nix/packages/controller.nix { };
              default = self.packages.${system}.stove8s-controller;
            }
          ))
          (
            forLinuxSystems (
              { system, pkgs }:
              {
                oci-stove8s-controller = pkgs.callPackage ./nix/oci/controller.nix {
                  stove8s-controller = self.packages.${system}.stove8s-controller;
                };
              }
            )
          );

      devShells = forAllSystems (
        { system, pkgs }:
        {
          stove8s = pkgs.callPackage ./nix/shell.nix {
            stove8s-controller = self.packages.${system}.stove8s-controller;
          };
          default = self.devShells.${system}.stove8s;
        }
      );
    };
}
