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
    dockerTools.caCertificates
  ];

  config = {
    Cmd = [
      (lib.getExe stove8s-daemonset)
    ];
    ExposedPorts = {
      "${builtins.toString port}/tcp" = { };
    };
  };
}
