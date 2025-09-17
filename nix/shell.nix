{
  mkShell,
  stove8s,

  gopls,
  nixfmt-rfc-style,
  markdownlint-cli,

  kubebuilder,
}:

mkShell {
  inputsFrom = [ stove8s ];

  buildInputs = [
    kubebuilder
    gopls
    nixfmt-rfc-style
    markdownlint-cli
  ];

  shellHook = ''
    export PS1="\033[0;31m[stove8s]\033[0m $PS1"
  '';
}
