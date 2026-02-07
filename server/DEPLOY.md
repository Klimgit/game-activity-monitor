# Deployment guide

Full step-by-step instructions for bringing up the game-activity-monitor stack
on a fresh Ubuntu 24.04 VPS.

## Recommended VPS specs

| Provider | Plan | vCPU | RAM | Disk | Cost |
|---|---|---|---|---|---|
| **Hetzner** (recommended) | CX32 | 4 | 8 GB | 80 GB NVMe | ~€9/mo |
| DigitalOcean | 4 vCPU / 8 GB | 4 | 8 GB | 160 GB SSD | ~$48/mo |
| Vultr | 4 vCPU / 8 GB | 4 | 8 GB | 160 GB NVMe | ~$40/mo |

Choose Ubuntu 24.04 LTS. No GPU needed — ML inference runs on CPU.

---

## 1. Initial server setup

```bash
# Log in as root, create a non-root user
adduser deploy
usermod -aG sudo deploy
# Copy your SSH public key
rsync --archive --chown=deploy:deploy ~/.ssh /home/deploy

# Switch to deploy user for everything else
su - deploy
```

**Firewall (UFW):**
```bash
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
```

---

## 2. Install Docker

```bash
curl -fsSL https://get.docker.com | sudo sh
docker --version
```

Optional: add your user to the `docker` group so you can run `docker` without `sudo` (`sudo usermod -aG docker $USER`, then log out and back in). For a minimal setup you can skip this and use **`sudo docker compose`** everywhere (see [QUICKSTART.md](QUICKSTART.md)).

---

## 3. Install Nginx + Certbot

```bash
sudo apt install -y nginx certbot python3-certbot-nginx

# Verify Nginx is running
sudo systemctl enable nginx
sudo systemctl start nginx
```

---

## 4. Clone the repository

```bash
git clone https://github.com/YOUR_USERNAME/game-activity-monitor.git
cd game-activity-monitor/server
```

---

## 5. Create the .env file

```bash
cp .env.example .env
nano .env   # fill in all CHANGE_ME values
```

Generate secrets:
```bash
# DB password
openssl rand -hex 16

# JWT secret
openssl rand -hex 32
```

---

## 6. Configure Nginx

```bash
# Replace YOUR_DOMAIN in the config
sed 's/YOUR_DOMAIN/yourdomain.com/g' nginx.conf \
  | sudo tee /etc/nginx/sites-available/game-monitor

sudo ln -s /etc/nginx/sites-available/game-monitor \
           /etc/nginx/sites-enabled/game-monitor
sudo rm -f /etc/nginx/sites-enabled/default

# Test config (will warn about missing SSL cert — that is expected)
sudo nginx -t

# Reload
sudo systemctl reload nginx
```

---

## 7. Obtain SSL certificate (Let's Encrypt)

```bash
sudo certbot --nginx -d yourdomain.com
# Follow the prompts — certbot will patch the Nginx config automatically.

# Verify auto-renewal works
sudo certbot renew --dry-run
```

---

## 8. Deploy the React dashboard

On your local machine:
```bash
cd dashboard
npm install
npm run build
```

Copy the built files to the server:
```bash
rsync -avz dist/ deploy@yourdomain.com:/var/www/game-monitor/
```

Or on the server if you have Node.js installed:
```bash
sudo mkdir -p /var/www/game-monitor
# copy files here
```

---

## 9. Start all services

**Bind API to localhost only** (recommended on a public VPS so port 8000 is not exposed to the internet — Nginx will proxy to `127.0.0.1:8000`):

In `docker-compose.yml`, under `server:` → `ports:`, use:

```yaml
    ports:
      - "127.0.0.1:8000:8000"
```

The default in the repo is `8000:8000` for easy home-LAN access; change it before exposing the machine to the public internet.

```bash
cd ~/game-activity-monitor/server

docker compose up -d

# Watch startup logs
docker compose logs -f
```

Expected healthy state (~30 seconds after start):
```
game-monitor-db       ... healthy
game-monitor-server   ... started
```

---

## 10. Verify everything works

```bash
# API health check
curl https://yourdomain.com/api/v1/health
```

Open the React dashboard in a browser: `https://yourdomain.com`

### Container logs

There is no bundled log UI. View API and database logs with Docker:

```bash
docker compose logs -f server
docker compose logs -f timescaledb
```

### Ad-hoc SQL (optional)

To run SQL against TimescaleDB, use an SSH tunnel or a one-off `psql` inside the DB container:

```bash
docker exec -it game-monitor-db psql -U postgres -d game_metrics
```

Example queries:

```sql
-- Sessions in the last 7 days
SELECT session_start, game_name, total_duration/60 AS minutes, activity_score
FROM activity_sessions
WHERE session_start > NOW() - INTERVAL '7 days'
ORDER BY session_start DESC;

-- Average activity score per game
SELECT game_name, ROUND(AVG(activity_score)::numeric, 3) AS avg_score, COUNT(*) AS n
FROM activity_sessions
GROUP BY game_name ORDER BY n DESC;
```

---

## Updating the application

```bash
cd ~/game-activity-monitor
git pull

# Rebuild and restart only changed containers
cd server
docker compose up -d --build server

# Or restart all
docker compose down && docker compose up -d
```

---

## Useful commands

```bash
# Live logs from a specific container
docker compose logs -f server

# Disk usage
docker system df

# Stop everything (data volumes are preserved)
docker compose down

# Full wipe including volumes (DELETES ALL DATA)
docker compose down -v
```
