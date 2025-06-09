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
    rev = "d76b02bfe256fba065234d3aa840548f09c61bba";
    sha256 = "sha256-1llza9vz76a71zv9z8agf00v1736wnpi965l05cai3rbgzvyrcpq";
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