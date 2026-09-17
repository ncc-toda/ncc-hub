---
name: syncing-ai-assets
description: ai-assets正本をプロジェクトへ同期し、CLAUDE.mdとClaude/Codex共通symlinkを更新するときに使う。
---

# AI資産の同期

対象プロジェクトで実行し、正本から`.claude/skills`へ冪等に同期する。
同期依頼は管理対象を更新する許可を含む。
競合する実体、orphanedの削除、判定不能な適応では停止する。

## 正本

- Skill: `~/Projects/tool/dotfiles/ai-assets/skills/`
- catalog: `~/Projects/tool/dotfiles/ai-assets/skills/skill-catalog.yaml`
- registry: `~/Projects/tool/dotfiles/ai-assets/registry/project-registry.yaml`
- 適応規則: [references/adaptation-rules.md](references/adaptation-rules.md)

catalog に登録された全 Skill を `core` として同期する。

## 同期

1. repository root、既存 AI 資産、Git 規約、検証コマンドを確認する。
2. catalog の全 Skill を`.claude/skills`へ同期する。
3. [適応規則](references/adaptation-rules.md)を適用する。
4. CLAUDE.md の管理節と必要な symlink を更新する。
5. digest と schema v2 の registry を更新する。
6. frontmatter、相対 link、symlink、Git 差分を検証する。

正本にない local Skill は変更しない。
orphaned 候補は報告し、削除しない。

## CLAUDE.md

template の次の節だけを追加または上書きする。

- `## 文章規範`
- `## 完了条件`

`## 作業ログ規約`と`## 完了報告フォーマット（必須）`は再生成しない。
template 由来の節が残っていれば削除する。
プロジェクト固有の記録方針は保持する。

プロジェクト概要、クイックリファレンス、設計・検証規約、ユーザー追加節は変更しない。

## Codex併用

Codex 併用が明示されている場合は次を使う。
`AGENTS.md`、`.agents`、`.codex`のいずれかが既にある場合も対象とする。

```text
AGENTS.md -> CLAUDE.md
.agents/skills -> ../.claude/skills
```

- 期待どおりのsymlinkはそのままにする。
- パスがなければ相対 symlink を作る。
- 通常ファイル、実体ディレクトリ、異なる symlink は上書きせず報告する。
- ユーザー設定と runtime 設定は同期しない。
- 例: `.claude/settings.json`、`.codex/config.toml`

## Registry

各 Skill ディレクトリの全ファイルを path 順に連結する。
sha256を`sha256:<hex>`で記録する。

- `intent`: `deployed`、`excluded`、`local`の人間判断を保持する。
- `state`は実体から再計算する。
- 値: `in-sync`、`local-modified`、`source-ahead`、`both`、`orphaned`
- `source_commit`と`last_scanned`を更新する。
- registryがv2でなければ変換せず、dotfiles側での移行が必要と報告する。

orphaned Skillは自動削除しない。
