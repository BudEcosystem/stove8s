{
  lib,
  buildGoModule,
}:

buildGoModule (finalAttrs: {
  pname = "stove8s-controller";
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

  vendorHash = "sha256-CzE9/IX5TAkGssejfX9/oJOnSOOvsb815gEcTFwdOMM=";

  # TODO: fix me
  doCheck = false;

  meta = {
    platforms = lib.platforms.unix;
    license = lib.licenses.agpl3Plus;
    mainProgram = "stove8s";
    maintainers = with lib.maintainers; [ sinanmohd ];
  };
})
