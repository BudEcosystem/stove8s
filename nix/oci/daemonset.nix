{
  stove8s-daemonset,
  lib,
  dockerTools,
}:
let
  port = 8008;
in
dockerTools.buildLayeredImage {
  name = "docker.io/budstudio/stove8s-daemonset";
  tag = "git";

  contents = [
    stove8s-daemonset
  ];

  config = {
    Cmd = [
      (lib.getExe stove8s-daemonset)
    ];
    Env = [
      "PORT=${builtins.toString port}"
    ];
    ExposedPorts = {
      "${builtins.toString port}/tcp" = { };
    };
  };
}
