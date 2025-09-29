{
  mkShell,
  stove8s,

  gopls,
  nixfmt-rfc-style,
  markdownlint-cli,

  kubebuilder,
  jq,
  kubernetes-controller-tools,
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
  ];

  shellHook = ''
    export PS1="\033[0;31m[stove8s]\033[0m $PS1"
  '';
}
