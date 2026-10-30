// Cross-check of cmd/bench with k6: same operations, same request shapes.
// PROTOCOL=rest|grpc, OP=read|views, VUS (default 10), WARMUP (5s), DURATION (30s).
import http from 'k6/http';
import grpc from 'k6/net/grpc';
import exec from 'k6/execution';
import { check } from 'k6';

const PROTOCOL = __ENV.PROTOCOL || 'rest';
const OP = __ENV.OP || 'read';
const VUS = Number(__ENV.VUS || 10);
const WARMUP = __ENV.WARMUP || '5s';
const DURATION = __ENV.DURATION || '30s';
const REST = __ENV.REST_ADDR || 'http://localhost:18080';
const GRPC = __ENV.GRPC_ADDR || 'localhost:19090';

const duration = PROTOCOL === 'rest' ? 'http_req_duration' : 'grpc_req_duration';

export const options = {
  scenarios: {
    warmup: { executor: 'constant-vus', vus: VUS, duration: WARMUP, gracefulStop: '0s' },
    measure: { executor: 'constant-vus', vus: VUS, duration: DURATION, startTime: WARMUP, gracefulStop: '5s' },
  },
  summaryTrendStats: ['avg', 'med', 'p(95)', 'p(99)', 'max'],
  // Thresholds only make k6 report the measure-scenario submetrics.
  thresholds: {
    [`${duration}{scenario:measure}`]: ['max>=0'],
    'iterations{scenario:measure}': ['count>=0'],
    'checks{scenario:measure}': ['rate>=0'],
  },
};

const client = new grpc.Client();
if (PROTOCOL === 'grpc') client.load(['../../api/proto'], 'counters/v1/counters.proto');

// int64 fields travel as decimal strings in protojson.
const ids = Array.from({ length: 20 }, (_, i) => String(i + 1));

function viewEvents() {
  // Unique viewer ids across VUs and iterations, like cmd/bench.
  const base = exec.vu.idInTest * 1e9 + exec.vu.iterationInInstance * 50;
  return Array.from({ length: 50 }, (_, i) => ({ postId: String((i % 20) + 1), viewerId: String(base + i + 1) }));
}

export default function () {
  if (PROTOCOL === 'rest') {
    const params = { headers: { 'X-User-ID': '1', 'Content-Type': 'application/json' } };
    const res = OP === 'read'
      ? http.get(`${REST}/v1/counters?postIds=${ids.join(',')}`, params)
      : http.post(`${REST}/v1/views`, JSON.stringify({ events: viewEvents() }), params);
    check(res, { ok: (r) => r.status === (OP === 'read' ? 200 : 202) });
    return;
  }
  if (exec.vu.iterationInInstance === 0) client.connect(GRPC, { plaintext: true });
  const params = { metadata: { 'x-user-id': '1' } };
  const res = OP === 'read'
    ? client.invoke('counters.v1.CounterService/GetCounters', { postIds: ids }, params)
    : client.invoke('counters.v1.CounterService/RecordViews', { events: viewEvents() }, params);
  check(res, { ok: (r) => r && r.status === grpc.StatusOK });
}

export function handleSummary(data) {
  const m = data.metrics;
  const d = m[`${duration}{scenario:measure}`].values;
  const it = m['iterations{scenario:measure}'].values;
  const out = {
    tool: 'k6', protocol: PROTOCOL, operation: OP, concurrency: VUS, warmup: WARMUP, duration: DURATION,
    // Counter rate spans the whole test, so divide by the measure window only.
    rps: it.count / parseFloat(DURATION), requests: it.count, okRate: m['checks{scenario:measure}'].values.rate,
    p50ms: d.med, p95ms: d['p(95)'], p99ms: d['p(99)'], maxms: d.max,
  };
  return { stdout: JSON.stringify(out) + '\n', [__ENV.OUT || '/dev/null']: JSON.stringify(out, null, 2) };
}
