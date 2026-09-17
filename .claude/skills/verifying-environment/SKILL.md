---
name: verifying-environment
description: PATH、バージョン、キャッシュ、ピン留め環境の差によって検証が再現しないときに使う。
---

# 環境検証

環境差が疑われる証拠を集め、コードの失敗と区別する。

## 診断

1. 失敗したコマンド、エラー、実行場所を記録する。
2. `flake.nix`、mise、devbox、package manager の marker を確認する。
3. 関連ツールの path と version を host とピン留め環境で比較する。
4. 固有 cache、重複 process、worktree で欠落した設定を確認する。
5. ピン留め環境で同じコマンドを再実行する。

`flake.nix`があるのにツールが global PATH へ解決される場合は環境差を疑う。
`nix develop -c <command>`の結果と比較する。

## 境界

- cache は再生成可能と確認できる場合だけ消す。
- ピン留め環境でも再現した場合だけ pre-existing 候補とする。
- path、version、コマンド、エラーを報告する。
- 検証が成功している作業では追加の環境調査をしない。
