{
  description = "Linkpearl - Secure, peer-to-peer clipboard synchronization";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      # Define modules outside of eachDefaultSystem since they're system-agnostic
      nixosModule = { config, lib, pkgs, ... }:
        with lib;
        let
          cfg = config.services.linkpearl;
        in
        {
          options.services.linkpearl = {
            enable = mkEnableOption (lib.mdDoc "Linkpearl clipboard synchronization service");
            
            secret = mkOption {
              type = types.nullOr types.str;
              default = null;
              description = lib.mdDoc "Shared secret for authentication (required if secretFile is not set)";
            };
            
            secretFile = mkOption {
              type = types.nullOr types.path;
              default = null;
              description = lib.mdDoc "Path to file containing the shared secret (required if secret is not set)";
            };
            
            listen = mkOption {
              type = types.nullOr types.str;
              default = null;
              example = ":9437";
              description = lib.mdDoc "Address to listen on";
            };
            
            join = mkOption {
              type = types.listOf types.str;
              default = [];
              example = [ "desktop.local:9437" "server:9437" ];
              description = lib.mdDoc "Addresses to connect to";
            };
            
            nodeId = mkOption {
              type = types.nullOr types.str;
              default = null;
              description = lib.mdDoc "Unique node identifier";
            };
            
            pollInterval = mkOption {
              type = types.str;
              default = "500ms";
              description = lib.mdDoc "Clipboard check interval";
            };
            
            verbose = mkOption {
              type = types.bool;
              default = false;
              description = lib.mdDoc "Enable verbose logging";
            };
            
            package = mkOption {
              type = types.package;
              default = self.packages.${pkgs.system}.default;
              defaultText = literalExpression "pkgs.linkpearl";
              description = lib.mdDoc "The linkpearl package to use";
            };
            
            clipboardPackage = mkOption {
              type = types.nullOr types.package;
              default = null;
              example = literalExpression "pkgs.xsel";
              description = lib.mdDoc ''
                Clipboard tool package to add to PATH.
                Common options include pkgs.xsel, pkgs.xclip, or pkgs.wl-clipboard.
                Set to null to use system clipboard tools.
              '';
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
              
              path = lib.optional (cfg.clipboardPackage != null) cfg.clipboardPackage;
              
              serviceConfig = {
                ExecStart = "${cfg.package}/bin/linkpearl --poll-interval ${cfg.pollInterval}";
                Restart = "on-failure";
                RestartSec = 5;
                
                # Hardening
                PrivateTmp = true;
                ProtectKernelTunables = true;
                ProtectKernelModules = true;
                ProtectControlGroups = true;
                RestrictAddressFamilies = [ "AF_UNIX" "AF_INET" "AF_INET6" ];
                RestrictNamespaces = true;
                RestrictRealtime = true;
                RestrictSUIDSGID = true;
                MemoryDenyWriteExecute = true;
                LockPersonality = true;
                NoNewPrivileges = true;
                ProtectProc = "invisible";
                ProcSubset = "pid";
                ProtectHostname = true;
                ProtectClock = true;
                ProtectKernelLogs = true;
                SystemCallArchitectures = "native";
                
                # Ensure clipboard access works
                PassEnvironment = [ "DISPLAY" "WAYLAND_DISPLAY" "XDG_RUNTIME_DIR" ];
              };
            };
          };
        };

      homeManagerModule = { config, lib, pkgs, ... }:
        with lib;
        let
          cfg = config.services.linkpearl;
        in
        {
          options.services.linkpearl = {
            enable = mkEnableOption (lib.mdDoc "Linkpearl clipboard synchronization service");
            
            secret = mkOption {
              type = types.nullOr types.str;
              default = null;
              description = lib.mdDoc "Shared secret for authentication (required if secretFile is not set)";
            };
            
            secretFile = mkOption {
              type = types.nullOr types.path;
              default = null;
              description = lib.mdDoc "Path to file containing the shared secret (required if secret is not set)";
            };
            
            join = mkOption {
              type = types.listOf types.str;
              default = [];
              example = [ "desktop.local:9437" "server:9437" ];
              description = lib.mdDoc "Addresses to connect to (required for client mode)";
            };
            
            nodeId = mkOption {
              type = types.nullOr types.str;
              default = null;
              description = lib.mdDoc "Unique node identifier";
            };
            
            pollInterval = mkOption {
              type = types.str;
              default = "500ms";
              description = lib.mdDoc "Clipboard check interval";
            };
            
            verbose = mkOption {
              type = types.bool;
              default = false;
              description = lib.mdDoc "Enable verbose logging";
            };
            
            package = mkOption {
              type = types.package;
              default = self.packages.${pkgs.system}.default;
              defaultText = literalExpression "pkgs.linkpearl";
              description = lib.mdDoc "The linkpearl package to use";
            };
            
            clipboardPackage = mkOption {
              type = types.nullOr types.package;
              default = null;
              example = literalExpression "pkgs.xsel";
              description = lib.mdDoc ''
                Clipboard tool package to add to PATH.
                Common options include pkgs.xsel, pkgs.xclip, or pkgs.wl-clipboard.
                Set to null to use system clipboard tools.
              '';
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
            
            home.packages = [ cfg.package ] ++ lib.optional (cfg.clipboardPackage != null) cfg.clipboardPackage;
          };
        };
    in
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
        packages = {
          default = linkpearl;
          linkpearl = linkpearl;
        };
        
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
      }
    ) // {
      # System-agnostic outputs go here, outside eachDefaultSystem
      nixosModules = {
        default = nixosModule;
        linkpearl = nixosModule; # For backwards compatibility
      };

      homeManagerModules = {
        default = homeManagerModule;
        linkpearl = homeManagerModule; # For backwards compatibility
      };

      # Overlay for easier integration
      overlays.default = final: prev: {
        linkpearl = self.packages.${final.system}.default;
      };
    };
}
