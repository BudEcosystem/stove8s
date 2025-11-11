{
  stove8s-controller,
  lib,
  dockerTools,
}:
let
  port = 8008;
in
dockerTools.buildLayeredImage {
  name = "budstudio/stove8s-controller";
  tag = "git";

  contents = [
    stove8s-controller
  ];

  config = {
    Cmd = [
      (lib.getExe stove8s-controller)
    ];
    Env = [
      "PORT=${builtins.toString port}"
    ];
    ExposedPorts = {
      "${builtins.toString port}/tcp" = { };
    };
  };
}
