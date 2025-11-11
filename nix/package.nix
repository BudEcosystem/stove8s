{
  lib,
  buildGoModule,
  subPackage ? "controller",
}:

buildGoModule (finalAttrs: {
  pname = "stove8s";
  version = "git";

  src = lib.cleanSourceWith {
    filter =
      name: type:
      lib.cleanSourceFilter name type
      && !(builtins.elem (baseNameOf name) [
        "nix"
        "flake.nix"
      ]);
    src = ../.;
  };
  vendorHash = "sha256-D3sMx3wRO4Qjk+vKLLxdOlUWngJ0y/zWG9Ne6GDrTvQ=";
  subPackages = [ "./cmd/${subPackage}" ];

  # TODO: fix me
  doCheck = false;

  meta = {
    platforms = lib.platforms.unix;
    license = lib.licenses.agpl3Plus;
    mainProgram = subPackage;
    maintainers = with lib.maintainers; [ sinanmohd ];
  };
})
