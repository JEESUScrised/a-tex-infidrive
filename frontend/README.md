# YTDrive Frontend

React + TypeScript + Vite + Gravity UI SPA.

## Разработка

В одном терминале backend:

```bash
go run ./cmd/server
```

В другом — Vite с proxy `/api` → `:8080`:

```bash
cd frontend
npm install
npm run dev
```

Откройте http://localhost:5173

## Production

```bash
cd frontend
npm run build
cd ..
go run ./cmd/server
```

Go отдаёт `frontend/dist` и API с одного адреса (по умолчанию `:8080`).
OAuth redirect: `http://localhost:8080/api/youtube/callback` → `/youtube?connected=1`.

## Проверки

```bash
npm run typecheck
npm run test
npm run build
```
