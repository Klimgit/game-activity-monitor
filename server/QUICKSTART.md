# Быстрый запуск на ноутбуке (Linux)

Без Nginx, без SSL, без добавления пользователя в группу `docker` — только Docker и `sudo`.

## 1. Установите Docker

Один раз: [https://docs.docker.com/engine/install/](https://docs.docker.com/engine/install/) (или пакет `docker.io` из репозитория дистрибутива).

Проверка:

```bash
sudo docker run --rm hello-world
```

## 2. Запустите API и базу

```bash
cd server
chmod +x run.sh
./run.sh
```

При первом запуске скрипт создаст `.env` из `.env.example` и попросит его отредактировать:

- **`DB_PASSWORD`** — любой надёжный пароль.
- **`DATABASE_URL`** — тот же пароль в строке после `postgres:` (вместо `CHANGE_ME`).
- **`JWT_SECRET`** — случайная строка, например: `openssl rand -hex 32`.

Сохраните файл и снова выполните `./run.sh`.

Проверка:

```bash
curl -s http://127.0.0.1:8000/api/v1/health
```

Логи: `sudo docker compose logs -f server`

Остановка: `sudo docker compose down` (из каталога `server`).

## 3. Регистрация пользователя

```bash
curl -s -X POST http://127.0.0.1:8000/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"you@local","password":"ваш-пароль"}'
```

## 4. Клиент на этом же ПК

В `client/configs/config.yaml`:

```yaml
server:
  url: "http://127.0.0.1:8000"
auth:
  email: "you@local"
  password: "ваш-пароль"
```

```bash
cd client
go run ./cmd
```

## 5. Клиент на другом компьютере в той же Wi‑Fi

Узнайте IP ноутбука с сервером: `ip -br a` (например `192.168.1.10`).

В `config.yaml` на втором ПК:

```yaml
server:
  url: "http://192.168.1.10:8000"
```

Разрешите входящие на ноутбуке с сервером, если мешает файрвол:

```bash
sudo ufw allow 8000/tcp
```

---

**Продакшен с доменом и HTTPS** — см. [DEPLOY.md](DEPLOY.md). Там же: при выкладке на VPS в `docker-compose.yml` лучше вернуть привязку API только к loopback (`127.0.0.1:8000:8000`), чтобы порт 8000 не был открыт в интернет, а наружу шёл только Nginx на 443.
