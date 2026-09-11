# サーバーが NixOS の場合の systemd サービス定義（任意）
# flake の nixosModules.default として公開される。
#
# 使い方（サーバー側の flake.nix）:
#   imports = [ works.nixosModules.default ];
#   services.works = {
#     enable = true;
#     package = works.packages.${system}.default;
#   };
{ config, lib, pkgs, ... }:

let
  cfg = config.services.works;
in
{
  options.services.works = {
    enable = lib.mkEnableOption "campus works showcase";

    package = lib.mkOption {
      type = lib.types.package;
      description = "works の packages.default（bin/works-server と share/works/pb_public を含む）";
    };

    dataDir = lib.mkOption {
      type = lib.types.path;
      default = "/var/lib/works";
      description = "pb_data を置くディレクトリ";
    };

    listenAddr = lib.mkOption {
      type = lib.types.str;
      default = "127.0.0.1:8090";
      description = "待ち受けアドレス";
    };

    environment = lib.mkOption {
      type = lib.types.attrsOf lib.types.str;
      default = { WORKS_FFMPEG_THREADS = "2"; };
      description = "WORKS_* 環境変数";
    };
  };

  config = lib.mkIf cfg.enable {
    users.users.works = {
      isSystemUser = true;
      group = "works";
      home = cfg.dataDir;
    };
    users.groups.works = { };

    systemd.services.works-server = {
      description = "campus works showcase";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      environment = cfg.environment;
      serviceConfig = {
        User = "works";
        Group = "works";
        StateDirectory = "works";
        WorkingDirectory = cfg.dataDir;
        ExecStart = ''
          ${cfg.package}/bin/works-server serve \
            --http=${cfg.listenAddr} \
            --dir=${cfg.dataDir}/pb_data \
            --publicDir=${cfg.package}/share/works/pb_public
        '';
        Restart = "always";
        RestartSec = 5;
        NoNewPrivileges = true;
        ProtectSystem = "strict";
        ReadWritePaths = [ cfg.dataDir ];
        PrivateTmp = true;
      };
    };
  };
}
