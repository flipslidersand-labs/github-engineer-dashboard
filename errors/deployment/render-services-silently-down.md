---
title: "Renderの本番URLが全パス404で応答（コールドスタートではなく停止/削除の可能性）"
tags: [render, deployment]
severity: medium
date: "2026-09-07"
---

## 症状

README記載の本番URL（`https://github-engineer-dashboard-api.onrender.com` /
`-go.onrender.com`）に `/healthz` `/` `/docs` いずれもアクセスすると、待ち時間なしで
即座に404が返る。

```
curl .../healthz → 404 (0.2s)
```

## 判断ポイント

Render free tier のコールドスタート（README記載: 約50秒）であれば、初回アクセス時に
レスポンスまで時間がかかるはず。今回は即座に404が返ったため、コールドスタート待ちでは
なく**サービス自体が停止・削除・URL変更されている**可能性が高い。

## 教訓

README記載の「本番稼働中」「Render deployed」といった記述や、過去セッションのメモリ
（`project_github_engineer_dashboard.md`: 「本番稼働中」）を鵜呑みにせず、外部公開URLを
使う判断（例: 職務経歴書に「本番稼働中」と書く）の前には必ず実際に疎通確認する。
Render free tier は一定期間アクセスがないと自動休止/削除される場合がある。

関連: Issue #108
