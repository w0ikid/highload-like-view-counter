Сырые результаты и команды запусков (команды лежат в `summary.json` каждого каталога).

- `sis2-main` — `read`, полный профиль (прогрев 10 с, замер 60 с, 3 прогона, c=1/10/50).
- `sis2-short` — `views`, `likes`, `duplicates`, `hot`, `freshness`; прогрев 2 с, замер 8 с, 1 прогон, c=10. `VIEW_TTL=5s`.
- `sis2-oom-failed` — упавший прогон `views` (TTL 1 ч): Redis исчерпал память, см. docs/sis2-report.md.
- `sis2-local` — ранний 3-секундный прогон (`read`, `views`, `likes`; `VIEW_TTL` 1 ч, Redis 256 МБ).
- `correctness.jsonl` — вывод `go test -json` интеграционных проверок.

Итоги и выводы: [docs/sis2-report.md](../../docs/sis2-report.md).
- `sis2-k6` — перекрёстная проверка `cmd/bench` против k6 (`read`, `views`, c=10, 5 с + 30 с, 1 прогон), `benchmarks/k6/run.sh`.
