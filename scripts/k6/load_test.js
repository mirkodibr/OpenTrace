/**
 * OpenTrace k6 load test
 *
 * Stages:
 *   0–30s   ramp to 50 VUs   (warm-up)
 *   30–90s  hold at 50 VUs   (sustained load)
 *   90–120s ramp to 200 VUs  (peak load)
 *   120–150s hold at 200 VUs
 *   150–180s ramp down to 0
 *
 * Run:
 *   COLLECTOR_URL=http://localhost:8080 QUERY_URL=http://localhost:8081 \
 *     k6 run scripts/k6/load_test.js
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const collectorURL = __ENV.COLLECTOR_URL || 'http://localhost:8080';
const queryURL     = __ENV.QUERY_URL     || 'http://localhost:8081';

// Custom metrics
const ingestErrorRate  = new Rate('ingest_errors');
const queryErrorRate   = new Rate('query_errors');
const ingestLatency    = new Trend('ingest_latency_ms', true);
const queryLatency     = new Trend('query_latency_ms', true);

export const options = {
  stages: [
    { duration: '30s', target: 50  },
    { duration: '60s', target: 50  },
    { duration: '30s', target: 200 },
    { duration: '30s', target: 200 },
    { duration: '30s', target: 0   },
  ],
  thresholds: {
    // Ingest p95 must stay under 500 ms
    ingest_latency_ms: ['p(95)<500'],
    // Query p95 must stay under 200 ms
    query_latency_ms:  ['p(95)<200'],
    // Error rate must stay below 1%
    ingest_errors:     ['rate<0.01'],
    query_errors:      ['rate<0.01'],
    // HTTP failure rate across all requests
    http_req_failed:   ['rate<0.01'],
  },
};

// Severity levels and service names used to generate realistic data
const severities    = ['debug', 'info', 'warn', 'error', 'fatal'];
const services      = ['api-gateway', 'auth-service', 'payment-service', 'user-service', 'mailer'];
const sampleBodies  = [
  'request completed successfully',
  'database query took longer than expected',
  'cache miss for key user:session',
  'payment gateway timeout after 3 retries',
  'email delivery failed: SMTP connection refused',
  'rate limit exceeded for client 192.168.1.1',
  'JWT token expired, issuing refresh',
  'health check passed',
];

function randomItem(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

function makeTraceID() {
  const hex = '0123456789abcdef';
  let s = '';
  for (let i = 0; i < 32; i++) s += hex[Math.floor(Math.random() * 16)];
  return s;
}

function makeSpanID() {
  const hex = '0123456789abcdef';
  let s = '';
  for (let i = 0; i < 16; i++) s += hex[Math.floor(Math.random() * 16)];
  return s;
}

function buildIngestPayload(batchSize) {
  const events = [];
  const now = new Date();
  for (let i = 0; i < batchSize; i++) {
    events.push({
      trace_id:     makeTraceID(),
      span_id:      makeSpanID(),
      timestamp:    new Date(now - Math.floor(Math.random() * 60000)).toISOString(),
      service_name: randomItem(services),
      severity:     randomItem(severities),
      body:         randomItem(sampleBodies),
      resource_attributes: {
        'service.version': '1.0.0',
        'host.name':       `host-${Math.floor(Math.random() * 10)}`,
      },
      log_attributes: {
        'http.status_code': 200 + Math.floor(Math.random() * 3) * 100,
        'http.method':      randomItem(['GET', 'POST', 'PUT', 'DELETE']),
      },
    });
  }
  return JSON.stringify({ events });
}

// Scenario A: ingest a batch of 10 events
function scenarioIngest() {
  const payload = buildIngestPayload(10);
  const params  = { headers: { 'Content-Type': 'application/json' } };

  const res = http.post(`${collectorURL}/api/v1/logs`, payload, params);

  ingestLatency.add(res.timings.duration);
  const ok = check(res, {
    'ingest: status 202': (r) => r.status === 202,
    'ingest: has batch_id': (r) => {
      try { return JSON.parse(r.body).batch_id !== undefined; } catch { return false; }
    },
  });
  ingestErrorRate.add(!ok);
}

// Scenario B: query the last 5 minutes with a random service filter
function scenarioQuery() {
  const end   = new Date();
  const start = new Date(end - 5 * 60 * 1000);
  const svc   = randomItem(services);
  const level = randomItem(['info', 'error', 'warn']);

  const url = `${queryURL}/api/v1/logs` +
    `?start_time=${start.toISOString()}` +
    `&end_time=${end.toISOString()}` +
    `&service_name=${svc}` +
    `&level=${level}` +
    `&limit=50`;

  const res = http.get(url);

  queryLatency.add(res.timings.duration);
  const ok = check(res, {
    'query: status 200': (r) => r.status === 200,
    'query: has data array': (r) => {
      try { return Array.isArray(JSON.parse(r.body).data); } catch { return false; }
    },
  });
  queryErrorRate.add(!ok);
}

// Scenario C: health checks (baseline — should always be fast)
function scenarioHealth() {
  const r1 = http.get(`${collectorURL}/healthz`);
  check(r1, { 'collector health 200': (r) => r.status === 200 });

  const r2 = http.get(`${queryURL}/healthz`);
  check(r2, { 'query-api health 200': (r) => r.status === 200 });
}

export default function () {
  const roll = Math.random();
  if (roll < 0.50) {
    scenarioIngest();
  } else if (roll < 0.90) {
    scenarioQuery();
  } else {
    scenarioHealth();
  }

  // Random think time 100–500 ms to avoid thundering herd
  sleep(0.1 + Math.random() * 0.4);
}

export function handleSummary(data) {
  return {
    'scripts/k6/results/summary.json': JSON.stringify(data, null, 2),
    stdout: textSummary(data),
  };
}

function textSummary(data) {
  const m = data.metrics;
  const lines = [
    '=== OpenTrace Load Test Summary ===',
    `  Ingest p95 latency  : ${m.ingest_latency_ms?.values?.['p(95)']?.toFixed(1) ?? 'n/a'} ms (threshold: 500 ms)`,
    `  Query  p95 latency  : ${m.query_latency_ms?.values?.['p(95)']?.toFixed(1) ?? 'n/a'} ms (threshold: 200 ms)`,
    `  Ingest error rate   : ${((m.ingest_errors?.values?.rate ?? 0) * 100).toFixed(2)}% (threshold: 1%)`,
    `  Query  error rate   : ${((m.query_errors?.values?.rate ?? 0) * 100).toFixed(2)}% (threshold: 1%)`,
    `  HTTP requests total : ${m.http_reqs?.values?.count ?? 0}`,
    `  VU peak             : ${data.root_group?.checks ? 'see k6 output' : 'n/a'}`,
  ];
  return lines.join('\n') + '\n';
}
