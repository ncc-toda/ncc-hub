{
  description = "campus works showcase";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        runtimeDeps = with pkgs; [ ffmpeg-headless exiftool ];
        # PocketBase v0.40 は Go 1.27+ を要求（nixpkgs 既定の go はまだ 1.26）
        buildGoModule' = pkgs.buildGoModule.override { go = pkgs.go_1_27; };
      in {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go_1_27 gopls golangci-lint
            nodejs_22
            ffmpeg exiftool cloudflared sqlite just prettier
          ];
          shellHook = ''
            export WORKS_DEV=1
            echo "works dev shell: 'just dev' で server + web を起動"
          '';
        };

        packages = rec {
          server = buildGoModule' {
            pname = "works-server";
            version = "0.1.0";
            src = ./server;
            vendorHash = "sha256-KnDWngI1FPXHxZ6rHItZry3QX2TYWX06mHdC4/jWrrI=";
            subPackages = [ "cmd/server" ];
            nativeBuildInputs = [ pkgs.makeWrapper ];
            postInstall = ''
              mv $out/bin/server $out/bin/works-server
              wrapProgram $out/bin/works-server \
                --prefix PATH : ${pkgs.lib.makeBinPath runtimeDeps}
            '';
          };

          web = pkgs.buildNpmPackage {
            pname = "works-web";
            version = "0.1.0";
            src = ./web;
            npmDepsHash = "sha256-RWV/4NwxGEEsAv9j/9C9PD30HcnAULvHjjKlrmFXz9s=";
            installPhase = ''
              runHook preInstall
              mkdir -p $out
              cp -r dist/. $out/
              runHook postInstall
            '';
          };

          default = pkgs.runCommand "works" { } ''
            mkdir -p $out/bin $out/share/works
            ln -s ${server}/bin/works-server $out/bin/works-server
            ln -s ${web} $out/share/works/pb_public
          '';
        };

        apps.default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/works-server";
        };
      })
    // {
      nixosModules.default = import ./deploy/nixos-module.nix;
    };
}
