# github-engineer-dashboard

A dashboard that analyzes and visualizes any GitHub URL (user / repository / PR / issue) via the GitHub API.  
Ships two backends — Python (FastAPI) and Go — so you can compare their latency side by side.

GitHub API で任意の URL（ユーザー / リポジトリ / PR / Issue）を分析・可視化するダッシュボード。  
Python (FastAPI) と Go の 2 バックエンドを搭載し、レイテンシを直接比較できる。

## Demo / デモ

> **Recording guide / 録画手順**: [docs/recording-guide.md](docs/recording-guide.md)

![demo](docs/demo.gif)

<!-- demo.gif が未作成の場合は上の行を削除し、以下を残す
録画後に docs/demo.gif を追加してください。
-->

| Service / サービス  | URL                                                |
| ------------------- | -------------------------------------------------- |
| Python (Render)     | https://github-engineer-dashboard-api.onrender.com |
| Go (Render)         | https://github-engineer-dashboard-go.onrender.com  |

> Render free tier — expect a cold start (~50s) on first access.  
> Render free tier のため初回アクセスは Cold start（約 50s）あり。

## Features / 機能

| Tab / タブ       | Description / 説明                                                                         |
| ---------------- | ------------------------------------------------------------------------------------------ |
| **Analyze**      | Auto-detects URL type (user/repo/PR/issue) and runs the appropriate analysis / GitHub URL を入力するだけで User / Repo / PR / Issue を自動判定して分析 |
| **Summary**      | Aggregates all repositories for a user or org (total stars, language breakdown, fork count) / ユーザー・組織の全リポジトリを集計（スター合計・言語分布・フォーク数） |
| **Compare**      | Side-by-side comparison of two users or two repositories / User 同士・Repo 同士を横並び比較 |
| **⚡ Benchmark** | Visualizes Python vs Go response time as a CSS bar chart with a history table / Python vs Go のレスポンス速度を CSS バーグラフで可視化、履歴テーブル付き |

- Toggle the backend via the header switch (**Backend: Python \| Go ⚡**) / ヘッダのトグル（**Backend: Python \| Go ⚡**）でバックエンドを切り替え
- Click **AI Review** on a PR analysis page to run a diff review via Claude / Ollama / PR 分析画面から **AI Review** ボタンで Claude / Ollama による diff レビューを実行
- SQLite TTL cache (300s) prevents hitting rate limits / SQLite TTL キャッシュ（300s）でレート制限を回避

## Tech Stack / 技術スタック

| Layer / レイヤー | Technology / 技術                                    |
| ---------------- | ---------------------------------------------------- |
| API (Python)     | Python 3.12 / FastAPI / Uvicorn                      |
| API (Go)         | Go 1.25 / chi / modernc.org/sqlite                   |
| Cache / キャッシュ | SQLite (TTL 300s, independent per backend / 各バックエンドで独立) |
| Frontend / フロント | Vanilla HTML + CSS + Fetch API                    |
| AI Review        | Claude Haiku (`ANTHROPIC_API_KEY`) / Ollama fallback |
| Deploy / デプロイ | Render (`render.yaml` Blueprint)                    |
| Tests / テスト   | pytest (35 tests)                                    |

## Endpoints / エンドポイント

### Shared (Python & Go) / 共通（Python / Go 両方）

| Method | Path                             | Description / 説明                                     |
| ------ | -------------------------------- | ------------------------------------------------------ |
| GET    | `/healthz`                       | Liveness check (no auth) / Liveness（認証不要）        |
| GET    | `/api/rate-limit`                | GitHub rate-limit remaining / GitHub rate-limit 残量   |
| GET    | `/api/analyze?url=`              | Auto-detect and analyze URL / URL 自動判定分析（user/repo/pr/issue） |
| GET    | `/api/summary?url=`              | User / org repository summary / ユーザー・組織集計     |
| GET    | `/api/users/{username}/activity` | Detailed user activity / ユーザーアクティビティ詳細    |
| GET    | `/api/benchmark?url=`            | Go latency measurement / Go レイテンシ計測             |

### Python only / Python のみ

| Method | Path                  | Description / 説明                                              |
| ------ | --------------------- | --------------------------------------------------------------- |
| GET    | `/`                   | Serve the frontend UI / フロント UI 配信                        |
| GET    | `/api/config`         | Expose Go backend URL to the frontend / Go バックエンド URL 通知（フロント用） |
| GET    | `/api/review?url=`    | AI diff review (PR URLs only) / AI diff レビュー（PR URL のみ） |
| GET    | `/api/benchmark?url=` | Measure and compare both backends / Python + Go 両方を計測して比較 |

### Authentication / 認証

All `/api/*` endpoints read from the `X-GitHub-Token` header or the `GITHUB_TOKEN` environment variable.  
A Fine-grained PAT with public repos read access is sufficient.

すべての `/api/*` は `X-GitHub-Token` ヘッダまたは `GITHUB_TOKEN` 環境変数を参照。  
Fine-grained PAT（public repos: read）で OK。

## Local Development / ローカル開発

```bash
# Python backend / Python バックエンド
python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
export GITHUB_TOKEN=ghp_xxx
uvicorn app.main:app --reload --port 8000
# → http://localhost:8000

# Go backend (separate terminal) / Go バックエンド（別ターミナル）
cd backend-go
go build -o server . && GITHUB_TOKEN=$GITHUB_TOKEN ./server
# → http://localhost:8080

# Point Python at the local Go backend / Go を Python から参照する場合
export GO_BACKEND_URL=http://localhost:8080

# Tests / テスト
pytest -q
```

OpenAPI docs / OpenAPI ドキュメント: `http://localhost:8000/docs`

## Deploy (Render) / デプロイ（Render）

Both services are defined in `render.yaml`.  
`render.yaml` に Python + Go の両サービスを定義済み。

```
Render Dashboard → New → Blueprint → select this repository / このリポジトリを選択
```

**Environment variables to set manually on first deploy / 初回のみ Render Dashboard で設定する環境変数:**

| Service / サービス | Variable / 変数  | Note / 説明                                   |
| ------------------ | ---------------- | --------------------------------------------- |
| Python             | `GITHUB_TOKEN`   | Fine-grained PAT                              |
| Python             | `CORS_ORIGINS`   | Auto-injected (fromService) / 自動設定        |
| Python             | `GO_BACKEND_URL` | Auto-injected (fromService) / 自動設定        |
| Go                 | `GITHUB_TOKEN`   | Fine-grained PAT                              |
| Go                 | `CORS_ORIGINS`   | Auto-injected (fromService) / 自動設定        |

`fromService` injects URLs automatically after both services are deployed.  
`fromService` により、両サービスのデプロイ後に URL が自動注入される。

## Repository Structure / リポジトリ構成

```
app/
  main.py           FastAPI app + all routes / FastAPI アプリ + 全ルート
  config.py         Env vars → Settings / 環境変数 → Settings
  github_client.py  GitHub REST client (parallel fetching) / GitHub REST クライアント（並列取得）
  cache.py          SQLite TTL cache / SQLite TTL キャッシュ
  models.py         Pydantic models (shared contract with Go) / Pydantic モデル（Go 版との共有契約）
  reviewer.py       AI diff review (Claude / Ollama) / AI diff レビュー
  url_parser.py     GitHub URL parser / GitHub URL パーサー
  static/
    index.html      Vanilla HTML dashboard / Vanilla HTML ダッシュボード
backend-go/
  main.go                     Entry point + CORS middleware / エントリポイント + CORS ミドルウェア
  internal/cache/             SQLite TTL cache (Go) / SQLite TTL キャッシュ（Go）
  internal/github/client.go   GitHub REST client (parallel fetching) / GitHub REST クライアント（並列取得）
  internal/handler/routes.go  Route registration + fromCache generics helper / ルート登録 + fromCache ジェネリクスヘルパー
  internal/model/types.go     Response types (matches Python) / レスポンス型（Python 版と一致）
docs/
  recording-guide.md  Demo GIF recording guide / デモ GIF 録画手順
tests/              pytest suite (35 tests) / pytest スイート（35 tests）
render.yaml         Render Blueprint (Python + Go services) / Render Blueprint（Python + Go 両サービス）
```
