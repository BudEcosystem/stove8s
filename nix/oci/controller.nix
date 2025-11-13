{
  stove8s-controller,
  lib,
  dockerTools,
}:
dockerTools.buildLayeredImage {
  name = "docker.io/budstudio/stove8s-controller";
  tag = "git";

  contents = [
    stove8s-controller
    dockerTools.caCertificates
  ];

  config = {
    Cmd = [
      (lib.getExe stove8s-controller)
    ];
  };
}
