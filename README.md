# highload-like-view-counter

Учебный сервис счётчиков лайков и просмотров, SIS2 / T7. Сейчас реализован **начальный каркас**, а не полный сервис по ТЗ. Использование Go вместо Java требует согласования с преподавателем.

Структура следует qrgo-back; инфраструктура использует только публичные Go-модули. Бизнес-код QRGO, Adata, RabbitMQ и приватные библиотеки не перенесены.

```text
cmd/
  http/                 HTTP API и graceful shutdown
  migrate/              Goose: up/down/status/version
  seed/                 тестовые посты и нулевые счётчики
  worker/               точка входа будущих фоновых задач
internal/
  bootstrap/            config, deps, app.go, Swagger/Scalar
  domain/counter/       доменные модели
  usecase/counters/     место для общей бизнес-логики
  transport/rest/        health/readiness
  transport/grpc/        место для gRPC-адаптера
  infrastructure/
    postgres/           пул PostgreSQL
    redis/              собственное подключение go-redis
    sqltx/              транзакции через context
  worker/
    flusher/            будущий перенос просмотров
    reconciler/         будущая сверка лайков
pkg/traceid/            генерация request ID
migrations/             embedded SQL Goose
api/proto/counters/v1/  исходный контракт Protobuf
docs/                  OpenAPI для реализованных endpoints
benchmarks/results/     место для сырых измерений
test/integration/      место для интеграционных проверок
```

Работают HTTP-запуск, `/healthz`, `/readyz`, миграции и повторяемый seed. `/readyz` проверяет соединения PostgreSQL и Redis; наличие схемы обеспечивает отдельная migration-команда. REST-операции счётчиков, gRPC-сервер, worker, измерения и отчёт ещё предстоит реализовать. Worker намеренно завершается с ошибкой до подключения задач.

## Запуск Docker Compose

```sh
cp .env.example .env
docker compose up --build -d
docker compose --profile tools run --rm seed -posts 100
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

Compose сначала ждёт PostgreSQL и применяет миграции, затем запускает один API. Redis хранит AOF, использует `noeviction` и лимит 256 MiB. При заполнении памяти записи могут отклоняться. `appendfsync everysec` не гарантирует потерю максимум одной секунды просмотров. Данные лежат в именованных volumes. `docker compose down` останавливает окружение без удаления volumes.

Swagger UI: http://localhost:8080/swagger/ . Scalar: http://localhost:8080/scalar . Оба используют `/swagger/openapi.json`. UI загружает JavaScript/CSS из публичного CDN, поэтому браузеру нужен интернет. В обычной сборке документация выключена; для неё используется build tag `swagger` (в Docker включён).

## Локальный Go

Нужен Go 1.26.2 или совместимая более новая версия. `.env` автоматически читается Compose; Go-процессу переменные передаются через окружение:

```sh
cp .env.example .env
set -a
. ./.env
set +a
docker compose up -d postgres redis
go run ./cmd/migrate up
go run ./cmd/seed -posts 100
go run -tags swagger ./cmd/http
```

Не запускайте локальный API одновременно с Compose API на том же порту. По умолчанию PostgreSQL/Redis используют 5432/6379; при конфликте измените пробросы портов и локальные переменные.

```sh
make test
go vet ./...
go build ./...
```

Seed создаёт посты с ID 1..N и недостающие строки счётчиков в одной транзакции. Повтор не меняет существующие лайки/просмотры; это не сброс benchmark-состояния.

## Следующие шаги по ТЗ

1. Общий service, валидация, ошибки, LikePost/UnlikePost и массовое чтение PostgreSQL.
2. Redis Lua: дедупликация просмотра на 3600 секунд без продления TTL и атомарное накопление delta.
3. Восстанавливаемый flusher, журнал batch и reconciler с согласованной блокировкой.
4. REST и gRPC с общими правилами, deadline и отменой; генерация кода из proto.
5. Проверки конкурентности/сбоев, CLI сравнения протоколов, сырые результаты и отчёт.

Планируемые REST-операции: `PUT/DELETE /v1/posts/{id}/like`, `POST /v1/views`, `GET /v1/counters?postIds=1,2`. Тестовая идентификация: `X-User-ID` / metadata `x-user-id`; это не production-авторизация. Представление int64 в REST JSON нужно зафиксировать при реализации адаптера и сравнении с Protobuf.

Сохранённые лайки будут читаться из PostgreSQL primary, просмотры — после flush. Кэша чтения, Redis Cluster, Caddy, Kafka и Kubernetes в каркасе нет. Kafka — отдельный следующий этап с надёжной доставкой, например outbox. Пропускная способность и свежесть пока не измерены.

При подготовке каркаса использован AI (Codex); формат раскрытия использования AI в итоговом отчёте нужно согласовать с правилами курса.

## Web service

В `web-service/` находится минимальный React + TypeScript + Vite frontend со стартовой страницей и проверкой подключения к API.

```sh
cd web-service
npm ci
npm run dev
```

Открыть http://localhost:5173. API должен работать на порту 8080. Подробности: [web-service/README.md](web-service/README.md).
