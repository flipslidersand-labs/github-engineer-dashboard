---
title: "ruff format が errors/ 配下の Markdown 内 Python コードブロックも書き換える"
tags: [ruff, markdown, ci]
severity: low
date: "2026-09-15"
---

## 症状

`ruff.toml` を新設して `ruff format .` を実行したところ、`errors/python/*.md` の
サンプルコードブロック（トラブルシュート説明用の再現コード）まで整形され、
本来触るべきでないドキュメントに意図しない差分が出た。

## 原因

ruff 0.16 以降、`ruff format` はデフォルトで対象拡張子に `.md` 内のフェンス付き
Pythonコードブロックも含む。`errors/` 配下のナレッジ文書は例示コードであり
実行対象ではないため、勝手に書き換えられると教訓の再現手順が変わってしまう。

## 解決策

`ruff.toml` に `extend-exclude = ["*.md"]` を追加し、Markdownファイルを
lint/format 対象から除外する。

## 予防

pyproject.toml が無くルート直下に `ruff.toml` を新設するリポでは、CIに
`ruff format --check .` を組み込む前に `git status` で意図しない `.md` 差分が
出ていないか確認する。既にqa-platform導入時にも同じ罠が確認されている
（`project_qa_platform_reusable_workflows` メモリ参照）。
