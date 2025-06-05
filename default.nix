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
    rev = "6e63504417960e571830d35843b0ec14225fb891";
    sha256 = "sha256-0zkjnhcf6bzbyf70fab2widlqxj08skkym4hvr8qfxf6c8ndy79b";
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