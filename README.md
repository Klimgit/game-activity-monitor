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
  ├── TimescaleDB (metrics, sessions, labels)
  └── Retention: raw events kept 1 hour, sessions kept forever

Dashboard (React)
  └── HTTP polling → Go server API
```

## Quick Start

### Server

```bash
cd server
cp .env.example .env        # fill in DB_PASSWORD, JWT_SECRET
docker-compose up -d        # starts TimescaleDB + server + nginx
```

### Client

```bash
cd client
cp configs/config.yaml.example configs/config.yaml   # set server URL + credentials
go run cmd/main.go
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
- **activity_labels** — manual ground-truth labels attached via hotkeys (for ML dataset annotation)
