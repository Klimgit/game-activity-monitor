# Game Activity Monitor

A client-server system for automatic detection and real-time monitoring of user gaming activity.

## Components

| Component | Tech | Description |
|-----------|------|-------------|
| `client/` | Go | Desktop agent: collects input/system metrics, sends to server |
| `server/` | Go + Gin + TimescaleDB | REST API, stores metrics, manages sessions |
| `dashboard/` | React + TypeScript | Web dashboard for analytics and visualization |

## Architecture

```
Desktop Client (Go)
  ├── Collectors (mouse, keyboard, CPU, GPU, memory, process)
  ├── Hotkey Manager (manual session/state labeling)
  └── API Client (in-memory queue → HTTP batch POST)
          │
          │ HTTPS
          ▼
Backend Server (Go/Gin)
  ├── REST API (JWT auth)
  ├── TimescaleDB (metrics, sessions, activity intervals)
  └── Retention: raw events kept 1 hour, sessions kept forever

Dashboard (React)
  └── HTTP polling → Go server API
```

## Quick Start

### Server (minimal, Linux)

No Docker group setup — the helper script uses `sudo`:

```bash
cd server
chmod +x run.sh
./run.sh                    # creates .env on first run; edit DB_PASSWORD + JWT_SECRET, then ./run.sh again
```

Or manually: `cp .env.example .env`, edit, then `sudo docker compose up -d --build`.

Details: [server/QUICKSTART.md](server/QUICKSTART.md). Production with HTTPS/Nginx: [server/DEPLOY.md](server/DEPLOY.md).

### Client

```bash
cd client
# edit configs/config.yaml — server.url (e.g. http://127.0.0.1:8000) and auth
go run ./cmd
```

### Dashboard

```bash
cd dashboard
npm install
npm run dev     # runs on http://localhost:5173, proxied to server :8000
```

## Environment Variables (server)

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8000` | HTTP listen port |
| `DATABASE_URL` | — | PostgreSQL connection string |
| `JWT_SECRET` | — | Secret key for JWT signing |

## Data Model

- **raw_input_events** — TimescaleDB hypertable, 1-hour retention, stores every mouse/keyboard/system event
- **activity_sessions** — one row per gaming session with aggregated durations and activity score
- **activity_intervals** — ground-truth time ranges (`active_gameplay` / `afk` / `menu` / `loading`) via dev hotkeys or API. ML CSV is built **inside the service** with `go run ./cmd/collectdataset` (uses `DATABASE_URL`, not end-user HTTP).
- **predicted_windows** — per aggregation window, model output (`predicted_state`, optional `confidence`, `model_version`). Ingest with `POST /api/v1/predictions/batch` (JWT = same user as session owner). Dashboard **States** page (`/timeline`) compares labels vs predictions over time.
