# コミットとブランチのルール

プロジェクト固有規約がない場合、コミットは次の形式にする。

```text
<type>: <日本語の説明> <gitmoji>
```

| type | gitmoji | 用途 |
| --- | --- | --- |
| `feat` | ✨ | 新機能 |
| `fix` | 🐛 | バグ修正 |
| `docs` | 📝 | 文書 |
| `style` | 💄 | 見た目・format |
| `refactor` | ♻️ | 動作を変えない内部改善 |
| `test` | 🧪 | テスト |
| `chore` | 🔧 | 保守・開発ツール |
| `build` | 📦 | ビルド設定 |
| `ci` | 💚 | CI |

GitHubの誤通知を防ぐため、メッセージに`@`の直後へ英数字を置かない。

branchが必要でプロジェクト規約がない場合は`<type>/<task-id>-<short-kebab-summary>`を使う。
