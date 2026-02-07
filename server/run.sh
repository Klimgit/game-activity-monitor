#!/usr/bin/env bash
# Минимальный запуск без настройки группы docker: используется sudo.
set -euo pipefail
cd "$(dirname "$0")"

if [[ ! -f .env ]]; then
	cp .env.example .env
	echo ""
	echo "  Создан файл .env — задайте DB_PASSWORD, JWT_SECRET и тот же пароль в DATABASE_URL."
	echo "  Потом снова:  ./run.sh"
	echo ""
	exit 1
fi

exec sudo docker compose up -d --build "$@"
