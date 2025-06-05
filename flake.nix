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
          
          vendorHash = "sha256-Y6/dBYyG252dmyVhcmN+25lqD4E0e9Vm5x8aFYC7J/I=";
          
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
        
        # Home-manager module for running as a user service
        homeManagerModules.default = { config, lib, pkgs, ... }:
          with lib;
          let
            cfg = config.services.linkpearl;
          in
          {
            options.services.linkpearl = {
              enable = mkEnableOption "Linkpearl clipboard synchronization service";
              
              secret = mkOption {
                type = types.nullOr types.str;
                default = null;
                description = "Shared secret for authentication (required if secretFile is not set)";
              };
              
              secretFile = mkOption {
                type = types.nullOr types.path;
                default = null;
                description = "Path to file containing the shared secret (required if secret is not set)";
              };
              
              join = mkOption {
                type = types.listOf types.str;
                default = [];
                example = [ "desktop.local:9437" "server:9437" ];
                description = "Addresses to connect to (required for client mode)";
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
              assertions = [
                {
                  assertion = (cfg.secret != null) || (cfg.secretFile != null);
                  message = "linkpearl: either 'secret' or 'secretFile' must be set";
                }
                {
                  assertion = !((cfg.secret != null) && (cfg.secretFile != null));
                  message = "linkpearl: 'secret' and 'secretFile' are mutually exclusive";
                }
                {
                  assertion = cfg.join != [];
                  message = "linkpearl: 'join' must be set for client mode in home-manager";
                }
              ];
              
              systemd.user.services.linkpearl = {
                Unit = {
                  Description = "Linkpearl clipboard synchronization";
                  After = [ "graphical-session.target" ];
                };
                
                Service = {
                  Environment = lib.mkMerge [
                    (mkIf (cfg.secret != null) [ "LINKPEARL_SECRET=${cfg.secret}" ])
                    (mkIf (cfg.secretFile != null) [ "LINKPEARL_SECRET_FILE=${toString cfg.secretFile}" ])
                    (mkIf (cfg.join != []) [ "LINKPEARL_JOIN=${concatStringsSep "," cfg.join}" ])
                    (mkIf (cfg.nodeId != null) [ "LINKPEARL_NODE_ID=${cfg.nodeId}" ])
                    (mkIf cfg.verbose [ "LINKPEARL_VERBOSE=true" ])
                  ];
                  
                  ExecStart = "${cfg.package}/bin/linkpearl --poll-interval ${cfg.pollInterval}";
                  Restart = "on-failure";
                  RestartSec = 5;
                };
                
                Install = {
                  WantedBy = [ "default.target" ];
                };
              };
              
              home.packages = [ cfg.package ];
            };
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
                type = types.nullOr types.str;
                default = null;
                description = "Shared secret for authentication (required if secretFile is not set)";
              };
              
              secretFile = mkOption {
                type = types.nullOr types.path;
                default = null;
                description = "Path to file containing the shared secret (required if secret is not set)";
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
              assertions = [
                {
                  assertion = (cfg.secret != null) || (cfg.secretFile != null);
                  message = "linkpearl: either 'secret' or 'secretFile' must be set";
                }
                {
                  assertion = !((cfg.secret != null) && (cfg.secretFile != null));
                  message = "linkpearl: 'secret' and 'secretFile' are mutually exclusive";
                }
              ];
              
              systemd.user.services.linkpearl = {
                description = "Linkpearl clipboard synchronization";
                after = [ "network.target" ];
                wantedBy = [ "default.target" ];
                
                environment = {
                } // (optionalAttrs (cfg.secret != null) {
                  LINKPEARL_SECRET = cfg.secret;
                }) // (optionalAttrs (cfg.secretFile != null) {
                  LINKPEARL_SECRET_FILE = toString cfg.secretFile;
                }) // (optionalAttrs (cfg.listen != null) {
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
