# Интеграционные проверки

```sh
docker compose --env-file .env.example up -d postgres redis mailhog
TEST_POSTGRES_DSN='postgres://counters:counters_local@localhost:5432/counters?sslmode=disable' TEST_REDIS_ADDR=localhost:6379 go test ./... -race -count=1 -v
```

Без переменных окружения интеграционные пакеты пропускаются; unit-тесты продолжают выполняться. Тесты создают временные PostgreSQL-схемы и Redis namespace. Данные приложения не сбрасываются.

`auth/`: авторизация, конкуренция refresh/reset, отзыв сессий. `counters/`: пункты 1–11 SIS2 — лайки, дедупликация между REST/gRPC, окно TTL без продления, предварительная валидация batch, конкурентный reconciler, свежесть, сбои зависимостей, deadlines/отмена, повтор после принятого запроса. Падение worker проверяется отдельным дочерним процессом с exit(99) в точках до и после commit. Проверяется сохранение новых pending событий во время in-flight.

Сырые результаты последнего прогона: benchmarks/results/correctness.jsonl.
