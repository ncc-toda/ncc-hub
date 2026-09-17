# CLAUDE.md

このファイルは Claude Code と Codex 共通の指示の正本です。
`AGENTS.md` は本ファイルへの symlink です。片方だけを編集しないでください。

## プロジェクト

NccHub は、校内ハッカソンの制作物を投稿・閲覧・「いいね」できる学内サイトです。
学生同士は匿名で、先生（管理者）だけが作者を確認できます。
仕様は [SPEC.md](./SPEC.md) が唯一の定義元です。実装判断で迷ったら SPEC を参照します。

- `server/`: PocketBase を組み込んだ Go 単一バイナリ
- `web/`: Vite + Svelte 5 + TypeScript
- `deploy/`: 学内サーバー（systemd）と Cloudflare Tunnel の配布物
- `docs/`: 先生向け運用マニュアルおよびデプロイ手順書

## クイックリファレンス

開発環境は Nix flake で固定します。コマンドは justfile が唯一の定義元です。

```bash
nix develop     # go / node / ffmpeg / exiftool / just が入る
just dev        # server(127.0.0.1:8090) と web(127.0.0.1:5173) を同時起動
just lint       # golangci-lint + svelte-check
just test       # go test ./...
just build      # nix build .#default
```

ツールチェーンは `nix develop` 済みシェルか `nix develop -c <cmd>` で呼び出します。

## AI 資産

- 共通 Skill: `.claude/skills/`（`syncing-ai-assets` で配備する）
- Skill の正本: dotfiles の `ai-assets/skills/`
- Codex 用 Skill: `.agents/skills -> ../.claude/skills`
- Codex 用指示: `AGENTS.md -> CLAUDE.md`
- 個人設定、MCP、plugin、runtime 設定は配布しない
- `.claude/` と `.agents/` は lint と format の対象外にする。整形すると正本との差分が生まれ、再同期で衝突する。

## プロジェクト固有ルール

- 文書、Issue、PR、commit message は日本語で書く。コード識別子とコマンドは英語表記を維持する。
- Skill で定義済みの手順をこのファイルへ転記しない。Skill 名で参照する。
- SPEC.md に反する実装を入れない。仕様変更が必要なら SPEC.md を先に更新する。

## Git 運用

main へ直接 commit します。feature branch は任意です。
手順と commit 規約は `git-operations` に従います。
GitHub 操作は `collaborating-on-github` に従います。

## 完了条件

実装依頼では、依頼範囲の変更と関連検証まで続けます。
将来の変更で参照する設計判断は、必要に応じて ADR または設計文書へ残します。
適用、merge、未依頼の外部操作は自動実行しません。

## 文章規範

- 結論と必要な行動を先に書き、前置き、賛辞、定型の報告枠を使わない。
- 承認依頼、失敗、未完了、破壊的操作の予告は必ず明示する。
- 静的な Markdown は 1 文 1 行で書く。Issue、PR、コメント、回答は段落内で改行しない。
- 文章の基準と推敲手順は `concise-writing` に従う。
