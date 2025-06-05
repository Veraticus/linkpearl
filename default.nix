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
    rev = "50060cd8a1a8b8622e9fe0b1a1ab47a422951254";
    sha256 = "sha256-016vpqna5191l8s3xyc2jbh5vvydsjhzyyyb30c7svvh0yl9rm5z";
  };

  vendorHash = "sha256-0zlnqk2ymdpb24ihlvm96s30dif0s16n0s1rkz1x5rn3ahmdqi6i";

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