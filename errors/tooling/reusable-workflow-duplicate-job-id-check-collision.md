---
title: "同一リポに複数reusable workflowを導入するとjob id衝突でcheck-run名が区別不能になる"
tags: [github-actions, qa-workflows, branch-protection]
severity: medium
date: "2026-09-15"
---

## 症状

`python-test.yml` と `go-test.yml` の2つのcaller workflowを同時に追加し、両方とも
job idを `test:` にしたところ、GitHub上のcheck-run名が両方とも `test / test` になり、
`gh api .../commits/main/check-runs` で見分けがつかなかった。branch protectionの
required status checksでPython側だけ/Go側だけを指定することができない。

## 原因

reusable workflowのcheck-run名は `<caller側job id> / <reusable内部job id>` の形式になる。
qa-workflowsのpython-test.yml/go-test.ymlは両方とも内部job idが `test` なので、
caller側も両方 `test:` にすると完全に同じ文字列 `test / test` が2つ生成される。

## 解決策

caller側のjob idをワークフローごとに一意にする（`python:` / `go:`）。
結果として `python / test` / `go / test` という区別可能な名前になる。

```yaml
jobs:
  python:   # ← test: ではなく一意な名前
    uses: flipslidersand-labs/qa-workflows/.github/workflows/python-test.yml@main
```

## 予防

1リポに複数のqa-workflows reusable workflowを導入するときは、追加直後に
`gh api repos/<owner>/<repo>/commits/<branch>/check-runs --jq '.check_runs[].name'`
で実際のcheck-run名一覧を確認し、重複がないことを確かめてからbranch protectionの
required checksに登録する。
