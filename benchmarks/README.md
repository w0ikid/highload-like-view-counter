# Сравнение REST и gRPC

Из корня проекта (нужны Go, Docker Compose, Python 3):

```sh
python3 benchmarks/run.py --output benchmarks/results/full
```

По умолчанию: 10 с прогрев, 60 с измерение, три прогона, параллелизм 1/10/50, операции read/ views/ likes. Отдельный Docker-проект counters-sis2-bench с фиксированными ресурсами. Перед каждым прогоном worker останавливается, сбрасываются только тестовые PostgreSQL/Redis, создаются 100 постов, worker запускается снова. API не кэширует чтения. Порядок протоколов чередуется между прогонами. Внимание: эта команда занимает около часа.

Короткий сохранённый профиль:

```sh
python3 benchmarks/run.py --duration 3s --warmup 1s --runs 3 --output benchmarks/results/sis2-local
```

Повторы просмотров и популярный пост проверяются отдельным запуском:

```sh
python3 benchmarks/run.py --duration 3s --warmup 1s --runs 1 --concurrency 10 --operations duplicates,hot --output benchmarks/results/sis2-special
```

Результирующая директория должна отсутствовать: скрипт не перезаписывает измерения. В `summary.json` — успехи, ошибки, RPS, p50/p95/p99 и точные команды. В `*.json.gz` — каждый измеренный end-to-end запрос, включая ошибки. В `*-resources.json` — временные выборки docker stats (включая прогрев). В `environment.json` — CPU/RAM хоста, лимиты и image IDs контейнеров, Go/зависимости, TLS/HTTP/сжатие. Фоновая нагрузка других проектов на хосте не исключена.

Клиент переиспользует HTTP-соединения и одно gRPC-соединение. Основные прогоны без ретраев. Клиентская задержка включает сериализацию, транспорт, сервер и чтение ответа. RPS делится на фактическое время с завершением запросов, начатых до конца окна. Warmup тоже использует новые viewer_id; повторный запуск полностью сбрасывает состояние.

Payload: `go run ./cmd/bench -payload`. Измеряются одинаковые логические структуры, JSON int64 — строки. У настоящих GET/PUT/DELETE параметры находятся в URL, поэтому сериализованный JSON request для этих операций — модель для сравнения, а не реальное HTTP body. URL, заголовки, gRPC framing, HTTP/2 и TLS не включены. Сетевой трафик не измерялся.

CLI для отдельного сценария:

```sh
go run ./cmd/bench -protocol rest -address http://localhost:8080 -op read -concurrency 10
go run ./cmd/bench -protocol grpc -address localhost:9090 -op views -concurrency 10
go run ./cmd/bench -protocol rest -op views -duration 5s -warmup 1s -retries 2
```

Требуется COUNTER_AUTH_MODE=test. Ретраи — максимум две дополнительные попытки, 100/200 мс backoff с jitter 50–100%, общий бюджет 2 с. Повторяются только 503/UNAVAILABLE и сетевые ошибки; идентичность операции сохраняется. Проверки ретраев и восстановления находятся в test/integration/counters и pkg/counterclient.

После измерений: `docker compose -f benchmarks/compose.yaml -p counters-sis2-bench down`. Данные этого окружения временные (tmpfs).
