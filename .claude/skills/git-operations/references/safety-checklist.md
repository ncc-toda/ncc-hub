# 安全チェックリスト

.gitignore の漏れとシークレットの流出を検出するためのコミット前検証。

## 手順

1. **ステージングされたファイルの一覧を確認:** `git diff --cached --name-only`

2. **機密パターンのチェック:**
   - `.env`、`.env.*`
   - `*.pem`、`*.key`、`*.p12`
   - `id_rsa`、`id_ed25519`
   - `*credentials*`、`*secret*`、`*token*`
   - `service-account*.json`、`firebase-admin*.json`

3. **必要に応じて .gitignore を修正:**
   - `git check-ignore -v <file>` で確認する
   - `.gitignore` の更新は同じコミットに含める。

4. **シークレットが検出された場合:** コミットを直ちに中止する。
   - ユーザーに報告する。
   - 該当ファイルをアンステージする。

## 補足ガイダンス

- 新しい生成ファイル、キャッシュ、ローカル設定がステージングに現れた場合は、`.gitignore` への追加を検討する。
- `.gitignore` の更新は関連する変更と同じコミットに含めるべきである。
