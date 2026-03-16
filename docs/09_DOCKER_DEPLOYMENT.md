# Stasharr — Docker & Deployment

## Container Overview

| Container | Base Image | Exposed Port |
|---|---|---|
| `stasharr-api` | `golang:1.23-alpine` (build) → `alpine:3.19` (run) | `8080` |
| `stasharr-ui` | `node:20-alpine` (build) → `nginx:alpine` (run) | `3000` |
| `postgres` | `postgres:16-alpine` | `5432` (internal only) |

The user is expected to already be running Prowlarr, SABnzbd, and StashApp. These are not included in the Stasharr compose file but are referenced by service name or IP in the app config.

---

## `docker-compose.yml` (Production)

```yaml
version: "3.9"

services:
  stasharr-api:
    build:
      context: .
      dockerfile: docker/api.Dockerfile
    container_name: stasharr-api
    restart: unless-stopped
    env_file:
      - .env
    environment:
      - STASHARR_PORT=8080
    volumes:
      - ${DOWNLOADS_PATH}:/downloads:ro        # SABnzbd complete dir (read initially, then move)
      - ${MEDIA_PATH}:/media                   # Final destination for moved files
    ports:
      - "${API_PORT:-8080}:8080"
    depends_on:
      postgres:
        condition: service_healthy
    networks:
      - stasharr
      - arr_network  # user's existing arr stack network

  stasharr-ui:
    build:
      context: .
      dockerfile: docker/ui.Dockerfile
    container_name: stasharr-ui
    restart: unless-stopped
    ports:
      - "${UI_PORT:-3000}:80"
    depends_on:
      - stasharr-api
    networks:
      - stasharr

  postgres:
    image: postgres:16-alpine
    container_name: stasharr-postgres
    restart: unless-stopped
    environment:
      POSTGRES_DB: stasharr
      POSTGRES_USER: stasharr
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    volumes:
      - stasharr-pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U stasharr"]
      interval: 5s
      timeout: 5s
      retries: 5
    networks:
      - stasharr

volumes:
  stasharr-pgdata:

networks:
  stasharr:
    driver: bridge
  arr_network:
    external: true  # must exist; name matches user's existing arr stack network
```

### Volume Mounts

Two host paths must be configured in `.env`:

| Variable | Description |
|---|---|
| `DOWNLOADS_PATH` | Path to SABnzbd's complete download directory. Stasharr reads files from here. Must be the same physical path that SABnzbd writes to, mounted into this container. |
| `MEDIA_PATH` | Stash library root. Stasharr moves files here. Must be the same physical path that Stash scans. |

Both paths should be absolute on the host. The `sabnzbd.complete_dir` config value in the app must match the **container-internal** path (`/downloads`), not the host path.

---

## `docker-compose.dev.yml` (Development)

```yaml
version: "3.9"

services:
  stasharr-api:
    build:
      context: .
      dockerfile: docker/api.Dockerfile
      target: dev
    env_file:
      - dev.env
    volumes:
      - .:/app                               # full source mount for hot reload
      - /app/vendor                          # preserve vendor dir
      - ${DOWNLOADS_PATH:-./dev-data/downloads}:/downloads
      - ${MEDIA_PATH:-./dev-data/media}:/media
    ports:
      - "8080:8080"
      - "2345:2345"                          # delve debugger port

  stasharr-ui:
    command: npm run dev -- --host 0.0.0.0
    build:
      context: ./web
      dockerfile: ../docker/ui.Dockerfile
      target: dev
    volumes:
      - ./web:/app
      - /app/node_modules
    ports:
      - "5173:5173"                          # Vite dev server
    environment:
      - VITE_API_URL=http://localhost:8080
```

---

## `docker/api.Dockerfile`

```dockerfile
FROM golang:1.23-alpine AS dev
WORKDIR /app
RUN apk add --no-cache git
# Air for hot-reload in dev
RUN go install github.com/air-verse/air@latest
COPY go.mod go.sum ./
RUN go mod download
CMD ["air", "-c", ".air.toml"]

FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o stasharr ./cmd/stasharr

FROM alpine:3.19 AS production
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/stasharr .
EXPOSE 8080
CMD ["./stasharr"]
```

---

## `docker/ui.Dockerfile`

```dockerfile
FROM node:20-alpine AS dev
WORKDIR /app
COPY web/package*.json ./
RUN npm install
COPY web/ .
EXPOSE 5173
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]

FROM node:20-alpine AS builder
WORKDIR /app
COPY web/package*.json ./
RUN npm install
COPY web/ .
ARG VITE_API_URL
ENV VITE_API_URL=${VITE_API_URL}
RUN npm run build

FROM nginx:alpine AS production
COPY --from=builder /app/dist /usr/share/nginx/html
COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
```

---

## `docker/nginx.conf`

```nginx
server {
    listen 80;
    root /usr/share/nginx/html;
    index index.html;

    # React SPA routing — all non-asset paths serve index.html
    location / {
        try_files $uri $uri/ /index.html;
    }

    # Proxy API requests to the backend
    # (Alternative: configure VITE_API_URL to point directly to the API port)
    location /api/ {
        proxy_pass http://stasharr-api:8080;
        proxy_http_version 1.1;

        # SSE support
        proxy_set_header Connection '';
        proxy_buffering off;
        proxy_cache off;
        chunked_transfer_encoding on;
    }
}
```

With this nginx config, the UI and API are accessible on the same port (3000). The frontend makes all API calls to `/api/v1/...` with no explicit host — nginx proxies them to the API container. SSE connections are handled correctly via the `proxy_buffering off` directive.

---

## `.env.example`

```bash
# Postgres password — used by both compose and the DSN below
POSTGRES_PASSWORD=change-me

# Full Postgres DSN passed to the API container
STASHARR_DB_DSN=postgres://stasharr:change-me@stasharr-postgres:5432/stasharr?sslmode=disable

# API authentication key — generate with: openssl rand -hex 32
STASHARR_SECRET_KEY=generate-me

# Host port bindings
API_PORT=8080
UI_PORT=3000

# Filesystem paths — must match SABnzbd and Stash configurations
DOWNLOADS_PATH=/path/to/sabnzbd/complete
MEDIA_PATH=/path/to/stash/library

# Log level: debug, info, warn, error
STASHARR_LOG_LEVEL=info
```

Copy to `.env` and populate before running `docker compose up`.

---

## Network Integration

Stasharr needs to reach Prowlarr, SABnzbd, and StashApp by hostname. The standard approach for users with an existing arr stack is to attach the `stasharr-api` container to the same Docker network as those services.

In `docker-compose.yml`, the `arr_network` is declared as external, referencing a network the user has already created for their arr stack. The user sets this network name in their `.env`:

```bash
ARR_NETWORK=arr_bridge  # or whatever the user's arr network is named
```

If the user is not using a shared Docker network, service URLs in the config can use `host.docker.internal` or explicit IP addresses.

---

## Upgrade Process

1. `git pull` the new version
2. `docker compose build`
3. `docker compose up -d`

Database migrations run automatically on startup. Migrations are always additive and backward-compatible within a major version — there are no destructive schema changes without a major version bump.

---

## Logging

Structured JSON logs via `zerolog`. All log lines include:
- `level`
- `time` (RFC3339)
- `worker` (for worker logs)
- `job_id` (when applicable)
- `msg`

Example:
```json
{"level":"info","time":"2024-03-15T10:00:12Z","worker":"resolver","job_id":"abc-123","msg":"resolved scene","stashdb_id":"def-456","title":"Scene Title"}
```

Log level is set via `STASHARR_LOG_LEVEL` env var.
