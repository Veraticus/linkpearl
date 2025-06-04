{
  description = "Linkpearl - Secure, peer-to-peer clipboard synchronization";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        
        linkpearl = pkgs.buildGoModule rec {
          pname = "linkpearl";
          version = "0.1.0";
          
          src = ./.;
          
          vendorHash = "sha256-1wi7pf01a6hzwxkdayrlh47nm6fvgrip4q95kffrvnw6ih2xvbv3";
          
          ldflags = [
            "-s"
            "-w"
            "-X main.version=${version}"
          ];
          
          meta = with pkgs.lib; {
            description = "Secure, peer-to-peer clipboard synchronization for your devices";
            homepage = "https://github.com/Veraticus/linkpearl";
            license = licenses.mit;
            maintainers = [ ];
            platforms = platforms.linux ++ platforms.darwin;
          };
        };
      in
      {
        packages.default = linkpearl;
        packages.linkpearl = linkpearl;
        
        apps.default = flake-utils.lib.mkApp {
          drv = linkpearl;
        };
        
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_24
            gnumake
            git
            
            # Clipboard dependencies for different platforms
            (if stdenv.isLinux then xsel else null)
            (if stdenv.isLinux then xclip else null)
            (if stdenv.isLinux then wl-clipboard else null)
            (if stdenv.isLinux then clipnotify else null)
          ];
          
          shellHook = ''
            echo "Linkpearl development environment"
            echo "Run 'make build' to build the project"
            echo "Run 'make test' to run tests"
          '';
        };
        
        # NixOS module for running as a systemd service
        nixosModules.default = { config, lib, pkgs, ... }:
          with lib;
          let
            cfg = config.services.linkpearl;
          in
          {
            options.services.linkpearl = {
              enable = mkEnableOption "Linkpearl clipboard synchronization service";
              
              secret = mkOption {
                type = types.str;
                description = "Shared secret for authentication";
              };
              
              listen = mkOption {
                type = types.nullOr types.str;
                default = null;
                example = ":8080";
                description = "Address to listen on";
              };
              
              join = mkOption {
                type = types.listOf types.str;
                default = [];
                example = [ "desktop.local:8080" "server:8080" ];
                description = "Addresses to connect to";
              };
              
              nodeId = mkOption {
                type = types.nullOr types.str;
                default = null;
                description = "Unique node identifier";
              };
              
              pollInterval = mkOption {
                type = types.str;
                default = "500ms";
                description = "Clipboard check interval";
              };
              
              verbose = mkOption {
                type = types.bool;
                default = false;
                description = "Enable verbose logging";
              };
              
              package = mkOption {
                type = types.package;
                default = self.packages.${pkgs.system}.linkpearl;
                defaultText = literalExpression "pkgs.linkpearl";
                description = "The linkpearl package to use";
              };
            };
            
            config = mkIf cfg.enable {
              systemd.user.services.linkpearl = {
                description = "Linkpearl clipboard synchronization";
                after = [ "network.target" ];
                wantedBy = [ "default.target" ];
                
                environment = {
                  LINKPEARL_SECRET = cfg.secret;
                } // (optionalAttrs (cfg.listen != null) {
                  LINKPEARL_LISTEN = cfg.listen;
                }) // (optionalAttrs (cfg.join != []) {
                  LINKPEARL_JOIN = concatStringsSep "," cfg.join;
                }) // (optionalAttrs (cfg.nodeId != null) {
                  LINKPEARL_NODE_ID = cfg.nodeId;
                }) // (optionalAttrs cfg.verbose {
                  LINKPEARL_VERBOSE = "true";
                });
                
                serviceConfig = {
                  ExecStart = "${cfg.package}/bin/linkpearl --poll-interval ${cfg.pollInterval}";
                  Restart = "on-failure";
                  RestartSec = 5;
                };
              };
            };
          };
      });
}
