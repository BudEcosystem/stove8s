{
  lib,
  buildGoModule,
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
  vendorHash = lib.fakeHash;

  meta = {
    platforms = lib.platforms.unix;
    license = lib.licenses.agpl3Plus;
    mainProgram = "stove8s";
    maintainers = with lib.maintainers; [ sinanmohd ];
  };
})
