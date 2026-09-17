# 非自明なghコマンド

## Review thread

```bash
# REST review comments
gh api "repos/<owner>/<repo>/pulls/<number>/comments" --paginate

# comment databaseIdとthread IDの対応
gh api graphql -f query='
query($owner:String!, $repo:String!, $number:Int!) {
  repository(owner:$owner, name:$repo) {
    pullRequest(number:$number) {
      reviewThreads(first:100) {
        nodes {
          id
          isResolved
          comments(first:100) { nodes { databaseId url } }
        }
      }
    }
  }
}' -F owner="<owner>" -F repo="<repo>" -F number="<number>"

# review commentへ返信
gh api -X POST "repos/<owner>/<repo>/pulls/<number>/comments/<comment-id>/replies" \
  -f body="<返信本文>"

# threadをResolve
gh api graphql -f query='
mutation($threadId:ID!) {
  resolveReviewThread(input:{threadId:$threadId}) {
    thread { id isResolved }
  }
}' -F threadId="<thread-id>"
```

## Sub-issue

```bash
# node IDと親子関係
gh api graphql -f query='
query($owner:String!, $repo:String!, $number:Int!) {
  repository(owner:$owner, name:$repo) {
    issue(number:$number) {
      id
      parent { id number url }
      subIssues(first:100) { nodes { id number url state } }
    }
  }
}' -F owner="<owner>" -F repo="<repo>" -F number="<number>"

# 既存Issueをsub-issueとして接続
gh api graphql -f query='
mutation($issueId:ID!, $subIssueId:ID!) {
  addSubIssue(input:{issueId:$issueId, subIssueId:$subIssueId}) {
    issue { number url }
    subIssue { number url }
  }
}' -F issueId="<parent-id>" -F subIssueId="<sub-issue-id>"

# 接続解除
gh api graphql -f query='
mutation($issueId:ID!, $subIssueId:ID!) {
  removeSubIssue(input:{issueId:$issueId, subIssueId:$subIssueId}) {
    issue { number url }
    subIssue { number url }
  }
}' -F issueId="<parent-id>" -F subIssueId="<sub-issue-id>"
```
