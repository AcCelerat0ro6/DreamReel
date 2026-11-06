import http from 'k6/http';
import { check } from 'k6';
import { Trend, Rate, Counter } from 'k6/metrics';
import encoding from 'k6/encoding';

const mode = __ENV.MODE || 'cached';
const base = __ENV.BASE_URL || 'http://127.0.0.1:18081';
const duration = __ENV.DURATION || '20s';
const latency = new Trend('recommendation_latency', true);
const firstLatency = new Trend('first_page_latency', true);
const cachedLatency = new Trend('cached_page_latency', true);
const refillLatency = new Trend('refill_page_latency', true);
const valid = new Rate('valid_page');
const duplicate = new Counter('duplicate_videos');
const requests = new Counter('recommendation_requests');
const journeys = new Counter('completed_journeys');
const received = new Counter('returned_videos');
let stats = {};

export const options = {
  scenarios: {
    measure: mode === 'scroll8' || mode === 'long'
      ? { executor: 'constant-vus', vus: Number(__ENV.VUS || 20), duration, gracefulStop: '10s' }
      : { executor: 'constant-arrival-rate', rate: Number(__ENV.RATE || (mode === 'first' ? 50 : 100)), timeUnit: '1s', duration, preAllocatedVUs: 30, maxVUs: 60, gracefulStop: '10s' },
  },
  thresholds: { valid_page: ['rate==1'], duplicate_videos: ['count==0'] },
  summaryTrendStats: ['avg', 'med', 'p(90)', 'p(95)', 'p(99)', 'min', 'max'],
};

function post(data, user, cursor, tag) {
  return http.post(base + '/api/feed-queries', JSON.stringify({
    scene: 'recommend', cursor, limit: 10,
  }), { headers: { Authorization: 'Bearer ' + data.tokens[user], 'Content-Type': 'application/json' }, tags: { phase: tag }, timeout: '10s' });
}

export function setup() {
  const info = JSON.parse(http.get(base + '/__bench/info').body);
  if (!info || !info.tokens) throw new Error('benchmark server unavailable');
  http.post(base + '/__bench/clear');
  info.cursors = [];
  // 两个版本都预热实际业务入口，建会话的开销不计入固定游标翻页场景。
  const warmUsers = mode === 'cached' ? 60 : mode === 'long' ? 10 : 20;
  for (let i = 0; i < warmUsers; i++) {
    const response = post(info, i % info.tokens.length, '', 'setup');
    if (response.status !== 200) throw new Error(response.body);
    info.cursors.push(response.json('next_cursor'));
    const warm = post(info, i % info.tokens.length, response.json('next_cursor'), 'setup');
    if (warm.status !== 200 || warm.json('items').length !== 10) throw new Error('page warmup failed');
  }
  if (info.cursors.length !== warmUsers || info.cursors.some(cursor => typeof cursor !== 'string' || !cursor)) throw new Error('missing pagination cursors');
  http.post(base + '/__bench/reset');
  return info;
}

function record(response, phase) {
  requests.add(1);
  duplicate.add(0);
  latency.add(response.timings.duration);
  if (phase === 'first') firstLatency.add(response.timings.duration);
  else if (phase === 'refill') refillLatency.add(response.timings.duration);
  else cachedLatency.add(response.timings.duration);
  let body;
  try { body = response.json(); } catch (_) { body = {}; }
  const ok = response.status === 200 && Array.isArray(body.items) && body.items.length === 10;
  valid.add(ok);
  check(response, { 'HTTP 200 and 10 videos': () => ok });
  if (ok) received.add(body.items.length);
  return body;
}

export default function (data) {
  const user = (__VU - 1) % data.tokens.length;
  if (mode === 'first' || mode === 'cached') {
    record(post(data, user, mode === 'first' ? '' : data.cursors[__VU - 1], mode), mode);
    return;
  }
  let cursor = '';
  const seen = new Set();
  const pages = mode === 'long' ? 25 : 8;
  for (let page = 0; page < pages; page++) {
    const phase = page === 0 ? 'first' : mode === 'long' && page === 17 ? 'refill' : 'cached';
    const body = record(post(data, user, cursor, phase), phase);
    for (const item of body.items || []) {
      if (seen.has(item.video_id)) duplicate.add(1);
      seen.add(item.video_id);
    }
    cursor = body.next_cursor;
    if (!body.has_more && page < pages - 1) {
      valid.add(false);
      return;
    }
  }
  journeys.add(1);
}

export function teardown() {
  http.post(base + '/__bench/finalize');
}

export function handleSummary(data) {
  delete data.setup_data;
  data.benchmark = { mode, base, duration, server: stats, k6Version: 'installed k6', startedAt: new Date().toISOString() };
  return { [__ENV.RESULT || 'summary.json']: JSON.stringify(data, null, 2) };
}
