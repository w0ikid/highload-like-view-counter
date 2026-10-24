# highload-like-view-counter

Учебный SIS2 / T7: лайки и просмотры через REST/JSON и gRPC/Protobuf с общей бизнес-логикой, PostgreSQL и Redis. Реализация на Go согласована пользователем. Дополнительно есть email/password auth и минимальный React frontend.

## Запуск

Нужны Docker Compose и свободные порты 8080, 9090, 5432, 6379, 1025, 8025.

```sh
cp .env.example .env
docker compose up --build -d
docker compose --profile tools run --rm seed -posts 100
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

Если `.env` уже есть, добавьте новые переменные из `.env.example`. Compose применяет миграции перед стартом API/worker. Запускается один API и один worker. Worker держит PostgreSQL advisory lock; дополнительная копия не начинает работу. `/readyz` проверяет PostgreSQL и Redis; `/healthz` — доступность HTTP-процесса. API корректно завершает HTTP/gRPC по SIGTERM, worker прекращает задачи; незавершённый batch остаётся в Redis.

Swagger: http://localhost:8080/swagger/ . Scalar: http://localhost:8080/scalar . Документация включается тегом `swagger` (в Docker включён). UI использует публичный CDN. MailHog: http://localhost:8025.

`.env.example` включает **COUNTER_AUTH_MODE=test** для учебных примеров. Это доверенный X-User-ID / gRPC metadata x-user-id, не production-авторизация. Для зарегистрированных пользователей установите `COUNTER_AUTH_MODE=bearer` и передавайте access JWT через Authorization / metadata authorization. Без переменной режим по умолчанию bearer.

## REST

```sh
curl -X PUT -H 'X-User-ID: 1' http://localhost:8080/v1/posts/1/like
curl -X DELETE -H 'X-User-ID: 1' http://localhost:8080/v1/posts/1/like
curl -X POST http://localhost:8080/v1/views \
  -H 'X-User-ID: 1' -H 'Content-Type: application/json' \
  -d '{"events":[{"postId":"1","viewerId":"42"}]}'
curl -H 'X-User-ID: 1' 'http://localhost:8080/v1/counters?postIds=1,2'
```

JSON использует camelCase. int64 представлены десятичными **строками**, чтобы браузер не терял точность; входной Protobuf JSON также принимает целые JSON-числа. RecordViews принимает 1–50 событий и возвращает 202 с accepted/duplicates. GetCounters принимает 1–100 уникальных ID, сохраняет порядок запроса. Неизвестный пост отклоняет весь запрос до записи. Ошибки: code, message, requestId и необязательный field.

## gRPC

Контракт: `api/proto/counters/v1/counters.proto`, сгенерированный Go-код уже включён в репозиторий. С grpcurl:

```sh
grpcurl -plaintext -import-path api/proto -proto counters/v1/counters.proto \
  -H 'x-user-id: 1' -d '{"postId":"1"}' localhost:9090 counters.v1.CounterService/LikePost
grpcurl -plaintext -import-path api/proto -proto counters/v1/counters.proto \
  -H 'x-user-id: 1' -d '{"events":[{"postId":"1","viewerId":"42"}]}' localhost:9090 counters.v1.CounterService/RecordViews
grpcurl -plaintext -import-path api/proto -proto counters/v1/counters.proto \
  -H 'x-user-id: 1' -d '{"postIds":["1","2"]}' localhost:9090 counters.v1.CounterService/GetCounters
```

Повтор просмотра через другой протокол — duplicate: хранилище и дедупликация общие. gRPC возвращает x-request-id в metadata. Коды: INVALID_ARGUMENT, UNAUTHENTICATED, NOT_FOUND, UNAVAILABLE, DEADLINE_EXCEEDED, INTERNAL; отмена клиента — CANCELED. REST соответствует 400/401/404/503/504/500.

## Архитектура и восстановление

```text
cmd/http, worker, migrate, seed, reconcile, bench
internal/bootstrap                  конфигурация и wiring
internal/domain                     модели и типизированные ошибки
internal/usecase/auth, counters      общая бизнес-логика
internal/transport/rest, grpc        адаптеры
internal/infrastructure/postgres     репозитории и транзакции
internal/infrastructure/redis        дедупликация, pending/in-flight
internal/worker                      flusher и reconciler
api/proto, api/gen                   контракт и generated Go
pkg/counterclient                    клиент и общий retry budget
web-service                         React + TypeScript + Vite
benchmarks                          CLI orchestration, результаты и отчёт
```

Лайки: блокировка строки post_counters, фактический INSERT/DELETE likes и изменение likes_count в одной READ COMMITTED транзакции. Повторы не меняют счётчик. Redis не участвует в лайках. Reconciler берёт ту же блокировку и выполняет COUNT после её получения; старый снимок не перезаписывает новые лайки. Он запускается каждую минуту и вручную:

```sh
docker compose exec worker /app/reconcile
```

Просмотры: Lua атомарно ставит dedup-ключ с TTL 3600 с без продления при повторе и увеличивает pending. Один стабильный Redis hash tag `{views}`; случайного распределения ключей нет. Это одна логическая партиция, не Redis Cluster.

Flusher каждую секунду переносит pending в in-flight через Lua/RENAME со стабильным UUID. Новые события идут в новый pending. PostgreSQL в одной транзакции вставляет batch_id в counter_flush_log и применяет delta только при новой вставке. После commit Lua Ack удаляет только совпадающий batch. Падение до commit или после commit до Ack безопасно: новый worker повторяет batch с тем же UUID.

При недоступности Redis новые просмотры отклоняются; лайки продолжают работать. При недоступности PostgreSQL новые лайки не подтверждаются; ранее принятые просмотры остаются в Redis. Приём новых просмотров также проверяет существование постов в PostgreSQL и при его недоступности отклоняется. После восстановления worker продолжает flush. Потеря самого Redis может уничтожить ещё не сохранённые просмотры: AOF everysec не гарантирует потерю максимум одной секунды. Redis использует noeviction; при заполнении памяти новые записи отклоняются. Не удаляйте pending/in-flight вручную.

Счётчики читаются из PostgreSQL primary одним запросом, без кэша чтения. Views включают только применённые batch. Цель свежести ≤2 с проверяется отдельно; при сбоях и превышении измеренной нагрузки она не гарантируется.

Бюджеты env: REQUEST_TIMEOUT=1s, POSTGRES_TIMEOUT=500ms, REDIS_TIMEOUT=100ms, WORKER_BUDGET=5s. Дочерние context ограничены оставшимся общим временем. Клиентский бюджет 2 с; максимум два дополнительных повтора с jitter, по умолчанию ноль в измерениях. Серверные Redis-команды не повторяются автоматически. PostgreSQL connection pool — максимум 20 соединений.

## Проверки и измерения

```sh
TEST_POSTGRES_DSN='postgres://counters:counters_local@localhost:5432/counters?sslmode=disable' \
TEST_REDIS_ADDR=localhost:6379 go test ./... -race -count=1 -v
```

Без TEST_* интеграционные тесты пропускаются. Проверки создают временные схемы/namespace. Сценарии: [test/integration/README.md](test/integration/README.md). Отчёт и ограничения: [docs/sis2-report.md](docs/sis2-report.md). Воспроизведение измерений: [benchmarks/README.md](benchmarks/README.md).

```sh
python3 benchmarks/run.py --duration 3s --warmup 1s --runs 3 --output benchmarks/results/my-run
# Полный рекомендуемый профиль 10 с + 60 с:
python3 benchmarks/run.py --output benchmarks/results/full-run
```

Benchmark использует отдельный Docker-проект counters-sis2-bench и сбрасывает только его данные перед каждым прогоном. Соединения переиспользуются, ретраи выключены, CPU/RAM и сырые latency сохраняются. Основные измерения напрямую к API, без Caddy и TLS. 20 000 increments/s — проектная цель SIS1, не заявленная гарантия.

## Локальная разработка

Go 1.26.2+. Env нужно экспортировать перед запуском Go-команд:

```sh
set -a
. ./.env
set +a
docker compose up -d postgres redis mailhog
go run ./cmd/migrate up
go run ./cmd/seed -posts 100
go run -tags swagger ./cmd/http
# В другом терминале с теми же переменными:
go run ./cmd/worker
```

Не запускайте одновременно Docker API и локальный API на тех же портах. Seed повторяемый и не обнуляет данные. `docker compose down` сохраняет основные volumes.

Генерация proto (проверенные версии: buf 1.68.3, protoc-gen-go 1.36.11, protoc-gen-go-grpc 1.5.1): `buf generate`. Без изменения контракта генераторы для сборки не нужны.

Auth и MailHog: [docs/auth.md](docs/auth.md). Frontend: `cd web-service && npm ci && npm run dev`; UI не участвует в измерениях и пока содержит проверку подключения/страницу reset, без ленты постов.

Кэш чтения, Caddy, Redis Cluster, Kafka, replicas, Kubernetes и HPA не входят в этот базовый этап. Для будущего асинхронного подсчёта лайков потребуется надёжная доставка, например transactional outbox. AI (Codex) использован при реализации и подготовке проверок; автору нужно уметь объяснить решения и оформить раскрытие AI согласно правилам курса.
