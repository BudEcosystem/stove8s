{
  stove8s,
  lib,
  dockerTools,
}:
let
  port = 8008;
in
dockerTools.buildLayeredImage {
  name = "budstudio/stove8s";
  tag = "git";

  contents = [
    stove8s
  ];

  config = {
    Cmd = [
      (lib.getExe stove8s)
    ];
    Env = [
      "PORT=${builtins.toString port}"
    ];
    ExposedPorts = {
      "${builtins.toString port}/tcp" = { };
    };
  };
}
