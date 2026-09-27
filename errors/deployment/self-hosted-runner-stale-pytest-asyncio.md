---
title: "self-hosted runner の残留 pytest-asyncio が pytest 9 の collection を落とす"
tags: [ci, python, self-hosted-runner, pytest]
severity: medium
date: "2026-09-17"
---

## 症状

PR #134 の `python / test` CI が突然赤くなった。ローカルでは `pytest -q` が 37 passed
で通るのに、CI だけ以下の INTERNALERROR で collection 段階から失敗:

```
INTERNALERROR> AttributeError: 'Package' object has no attribute 'obj'
  File ".../pytest_asyncio/plugin.py", line 612, in pytest_collectstart
```

## 原因

`requirements.txt` が `pytest>=8.0` のように上限なしピンだったため、CI 実行のたびに
その日の最新 pytest（このときは 9.1.1）が新規インストールされていた。一方、この
リポジトリは `pytest-asyncio` を一切使っていない（依存にも入っていない）のに、
自己ホスト runner（複数リポで共有）の site-packages には**他リポが入れた古い
pytest-asyncio が残留**しており、pytest はプラグインとして自動ロードしてしまう。
pytest 9 とその古い pytest-asyncio の組み合わせが collection 時にクラッシュする。

## 解決策

1. `requirements.txt` を `~=` で現在動作確認済みのバージョンにピン留め（unbounded
   `>=` を廃止）。
2. `.github/workflows/python-test.yml` の reusable workflow 呼び出しに
   `pytest-args: "-p no:asyncio"` を追加し、async テストが無いことを前提に
   pytest-asyncio の自動ロード自体を無効化。
3. 同時に `ruff-version` も明示的にピン留め（ruff の自動更新によるlint drift対策）。

（Issue #122 / PR #135）

## 予防

- 依存関係ファイルに unbounded `>=` を書かない。CI の再現性は「今日インストールされる
  もの」ではなく「昨日動いたもの」に固定する。
- 自己ホスト runner を複数リポで共有している場合、`pip install` はグローバル環境を
  汚染しうる前提でCIを組む（venv 隔離、またはプラグイン明示無効化）。
- ローカルで pytest が通っても CI が落ちる場合、まず「ローカルに存在しないパッケージが
  CI 側だけにある」可能性（runner 環境の汚染）を疑う。
