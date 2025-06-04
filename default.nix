{ lib
, buildGoModule
, fetchFromGitHub
, xsel
, xclip
, wl-clipboard
, clipnotify
, stdenv
}:

buildGoModule rec {
  pname = "linkpearl";
  version = "0.1.0";

  src = ./.;

  vendorHash = null; # Will be updated after first build

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