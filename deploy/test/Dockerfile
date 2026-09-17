# NccHub の install.sh を実機相当で検証するためのコンテナ。
# PID 1 を systemd にするため、起動時に --privileged と cgroup のマウントが要る。
# 使い方は deploy/test/run.sh を参照。
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV container=docker

# systemd 本体と、install.sh が前提とする最低限のツール。
# install.sh 自身も apt で導入するが、ここで入れておくと検証の反復が速い。
RUN apt-get update && apt-get install -y --no-install-recommends \
      systemd systemd-sysv dbus dbus-user-session \
      ca-certificates curl git xz-utils jq util-linux sudo procps iproute2 less \
 && apt-get clean && rm -rf /var/lib/apt/lists/*

# コンテナで不要かつ起動を遅らせる unit を無効化する。
RUN systemctl mask \
      systemd-udevd.service systemd-udevd-kernel.socket systemd-udevd-control.socket \
      systemd-modules-load.service sys-kernel-debug.mount sys-kernel-tracing.mount \
      systemd-journald-audit.socket getty.target console-getty.service

STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
