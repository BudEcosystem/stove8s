{
  mkShell,
  stove8s,

  gopls,
  nixfmt-rfc-style,
  markdownlint-cli,
  helm-ls,

  kubebuilder,
  jq,
  kubernetes-controller-tools,

  kubectl,
  k3d,
}:

mkShell {
  inputsFrom = [ stove8s ];

  buildInputs = [
    jq
    kubebuilder
    gopls
    nixfmt-rfc-style
    markdownlint-cli
    kubernetes-controller-tools
    kubectl
    k3d
    helm-ls
  ];

  shellHook = ''
    export PS1="\033[0;31m[stove8s]\033[0m $PS1"
  '';
}
