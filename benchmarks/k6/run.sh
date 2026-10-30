#!/usr/bin/env bash
# Runs k6 and cmd/bench back to back on the same disposable benchmark stack,
# resetting state before every run exactly like benchmarks/run.py.
# Usage: benchmarks/k6/run.sh <output-dir>   (env: OPS, VUS, WARMUP, DURATION)
set -euo pipefail
OUT=$(realpath -m "${1:?output dir}")
cd "$(dirname "$0")"
ROOT=../..
OPS=${OPS:-read views}
VUS=${VUS:-10}
WARMUP=${WARMUP:-5s}
DURATION=${DURATION:-30s}
COMPOSE=(docker compose -f "$ROOT/benchmarks/compose.yaml" -p counters-sis2-bench)

[ -e "$OUT" ] && { echo "$OUT exists, refusing to overwrite" >&2; exit 1; }
mkdir -p "$OUT"
go build -o "$OUT/bench" "$ROOT/cmd/bench"
# api, worker and migrate share one image tag; build it once to avoid a parallel-build clash.
"${COMPOSE[@]}" build api >/dev/null
"${COMPOSE[@]}" up -d --wait api worker >/dev/null

reset() {
  "${COMPOSE[@]}" stop worker >/dev/null
  "${COMPOSE[@]}" exec -T postgres psql -q -v ON_ERROR_STOP=1 -U bench -d counters_bench -c \
    "TRUNCATE likes, counter_flush_log, post_counters, posts; INSERT INTO posts SELECT n,'Post '||n FROM generate_series(1,100)n; INSERT INTO post_counters(post_id) SELECT id FROM posts;" >/dev/null
  # This Redis belongs ONLY to the disposable benchmark project.
  "${COMPOSE[@]}" exec -T redis redis-cli FLUSHDB >/dev/null
  "${COMPOSE[@]}" start worker >/dev/null
}

for op in $OPS; do
  for protocol in rest grpc; do
    addr=http://localhost:18080; [ "$protocol" = grpc ] && addr=localhost:19090
    reset
    k6 run -q --no-color -e PROTOCOL=$protocol -e OP=$op -e VUS=$VUS -e WARMUP=$WARMUP -e DURATION=$DURATION \
      -e OUT="$OUT/k6-$op-$protocol.json" counters.js >/dev/null
    reset
    "$OUT/bench" -protocol $protocol -address $addr -op $op -concurrency $VUS \
      -duration $DURATION -warmup $WARMUP -retries 0 \
      | python3 -c 'import json,sys; d=json.load(sys.stdin); d.pop("samples"); json.dump(d,sys.stdout,indent=2)' \
      > "$OUT/bench-$op-$protocol.json"
    echo "done $op $protocol"
  done
done
rm "$OUT/bench"
