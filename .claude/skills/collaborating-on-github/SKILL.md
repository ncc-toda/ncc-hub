---
name: collaborating-on-github
description: gh CLIでIssue、PR、レビューコメント、review thread、CIを操作するときに使う。
---

# GitHubでの協業

GitHub操作は`gh`、ローカル操作は`git`を使う。文面はプロジェクト慣例がなければ日本語にする。

## 対象の確定

- bare numberはremoteのIssueとPRを照合して種別を確定する。
- PR作業では`gh pr view <number> --json baseRefName,headRefName,headRefOid,state,isDraft`で現在のheadを確認する。
- 読み取り、修正、返信、Resolveは同じheadを対象にする。headが変わった場合は再取得する。

## PR

- 新規 PR は作業と検証が未完なら Draft とする。
- 完了していればプロジェクト慣例に従う。
- review対応ではREST commentsとGraphQL `reviewThreads`の両方を取得する。
- 修正が必要なら、検証、commit、pushの後に返信・Resolveする。
- 完了前に未解決thread、CI、review decision、merge state、PR bodyを再取得する。
- mergeはユーザーの明示承認がある場合だけ行う。

## Issueと進捗共有

- Issueの状態変更、blocker、仕様変更、レビュー完了はGitHubへ簡潔に記録する。
- Issue、PR 本文、コメントの文面は `concise-writing` の基準に従う。段落内で改行せず、段落単位で書く。
- 短い内部作業の逐次コメントは投稿しない。
- sub-issueやreview threadのGraphQL操作は[非自明なghコマンド](references/gh-commands.md)を使う。

外部操作が失敗した場合は、失敗内容、未完了の操作、再開条件をユーザーへ報告する。
