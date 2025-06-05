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
    rev = "02b61281aaff1476984e8a2a440ba45f2b5d49aa";
    sha256 = "sha256-0cl2iivjvhiapz85m0lvac8xj6xl9hib6i8mz4ws9yg0lfr37a1d";
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