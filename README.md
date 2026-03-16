# Stasharr

Stasharr is a self-hosted pipeline tool that bridges StashDB with StashApp via automated download workflows. It uses Prowlarr for indexer search and SABnzbd as the download client.

## Quick Start

### Prerequisites

- Docker and Docker Compose
- An existing arr stack (Prowlarr, SABnzbd) on a shared Docker network
- A running StashApp instance

### Setup

1. Clone the repository:
   ```bash
   git clone https://github.com/mononen/stasharr.git
   cd stasharr
   ```

2. Copy the example environment file and configure it:
   ```bash
   cp .env.example .env
   ```

3. Edit `.env` with your values:
   - Set `POSTGRES_PASSWORD` and update it in `STASHARR_DB_DSN`
   - Generate a secret key: `openssl rand -hex 32` and set `STASHARR_SECRET_KEY`
   - Set `DOWNLOADS_PATH` to your SABnzbd complete directory
   - Set `MEDIA_PATH` to your Stash library root

4. Ensure your arr stack Docker network exists (default: `arr_network`), or update `docker-compose.yml` with your network name.

5. Start the stack:
   ```bash
   docker compose up -d
   ```

6. Open the UI at `http://localhost:3000` and configure your service connections (Prowlarr, SABnzbd, StashDB API keys).

### Tampermonkey Script

1. Install the [Tampermonkey](https://www.tampermonkey.net/) browser extension
2. Create a new script and paste the contents of `scripts/tampermonkey/stasharr.user.js`
3. Configure the script via Tampermonkey menu > "Stasharr Settings" with your API URL and secret key

### Development

```bash
cp dev.env.example dev.env
docker compose -f docker-compose.yml -f docker-compose.dev.yml up
```

- API with hot-reload: `http://localhost:8080`
- UI with HMR: `http://localhost:5173`
- Debugger port: `2345`
