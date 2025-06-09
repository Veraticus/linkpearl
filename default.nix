{ lib
, buildGoModule
, fetchFromGitHub
, xsel
, xclip
, wl-clipboard
, clipnotify
, stdenv
, src ? null
}:

buildGoModule rec {
  pname = "linkpearl";
  version = "0.1.0";

  src = if src != null then src else fetchFromGitHub {
    owner = "Veraticus";
    repo = "linkpearl";
    rev = "55ab9a29679b1e9bb14bd54e073a0b80617aff23";
    sha256 = "sha256-04l5260gcfdl0ir3wdgl67knlbjcvbjkjlpnawdmymmn9hnvws1v";
  };

  vendorHash = "sha256-1gj68wlfm34xlyr5r2v2m70pi5697mqvxr8f7a95myslc96jmmlc";

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];

  nativeBuildInputs = lib.optionals stdenv.isLinux [
    xsel
    xclip
    wl-clipboard
    clipnotify
  ];

  meta = with lib; {
    description = "Secure, peer-to-peer clipboard synchronization for your devices";
    homepage = "https://github.com/Veraticus/linkpearl";
    license = licenses.mit;
    maintainers = [ ];
    platforms = platforms.linux ++ platforms.darwin;
  };
}