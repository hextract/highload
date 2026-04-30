import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:8080';
const API_PREFIX = __ENV.API_PREFIX || '/api/v1';
const RESTAURANT_ID = __ENV.RESTAURANT_ID || 'f47ac10b-58cc-4372-a567-0e02b2c3d479';
const ITEM_1 = __ENV.MENU_ITEM_1 || 'a1b2c3d4-e5f6-7890-abcd-ef1234567890';
const ITEM_2 = __ENV.MENU_ITEM_2 || 'e3f4a5b6-c7d8-9012-cdef-123456789012';
const SEARCH_TERMS = (__ENV.SEARCH_TERMS || 'sushi,pizza,burger,wok,salad,dessert')
  .split(',')
  .map((v) => v.trim())
  .filter(Boolean);
const RESTAURANT_IDS = (__ENV.RESTAURANT_IDS || RESTAURANT_ID)
  .split(',')
  .map((v) => v.trim())
  .filter(Boolean);
const STATUS_ORDER_IDS = (__ENV.STATUS_ORDER_IDS || __ENV.ORDER_IDS || '')
  .split(',')
  .map((v) => v.trim())
  .filter(Boolean);
const SEARCH_AND_MENU_RPS = Number(__ENV.SEARCH_AND_MENU_RPS || 750);
const CHECKOUT_RPS = Number(__ENV.CHECKOUT_RPS || 125);
const STATUS_RPS = Number(__ENV.STATUS_RPS || 630);
const DURATION = __ENV.DURATION || '150s';
const PRE_ALLOCATED_VUS_READS = Number(__ENV.PRE_ALLOCATED_VUS_READS || 240);
const MAX_VUS_READS = Number(__ENV.MAX_VUS_READS || 800);
const PRE_ALLOCATED_VUS_CHECKOUT = Number(__ENV.PRE_ALLOCATED_VUS_CHECKOUT || 120);
const MAX_VUS_CHECKOUT = Number(__ENV.MAX_VUS_CHECKOUT || 500);
const PRE_ALLOCATED_VUS_STATUS = Number(__ENV.PRE_ALLOCATED_VUS_STATUS || 200);
const MAX_VUS_STATUS = Number(__ENV.MAX_VUS_STATUS || 700);
const ORDER_ID = __ENV.ORDER_ID || '7f08ee66-7ebd-45fb-a6af-0d5fb015f1af';

const WARMUP_DURATION = __ENV.WARMUP_DURATION || '45s';
const WARMUP_ENABLED = !/^0s?$|^false$/i.test(String(WARMUP_DURATION).trim());
const WARMUP_RATE = Number(__ENV.WARMUP_RATE || 500);
const WARMUP_START_RATE = Number(__ENV.WARMUP_START_RATE ?? __ENV.WARMUP_INITIAL_RATE ?? 0);
const WARMUP_PRE_ALLOCATED_VUS = Number(__ENV.WARMUP_PRE_ALLOCATED_VUS || 40);
const WARMUP_MAX_VUS = Number(__ENV.WARMUP_MAX_VUS || 150);

const tSearch = new Trend('req_search_duration', true);
const tMenu = new Trend('req_menu_duration', true);
const tCheckout = new Trend('req_checkout_duration', true);
const tStatus = new Trend('req_status_duration', true);
const tStatusVisibilityLag = new Trend('status_visibility_lag_ms', true);

let lastKnownOrderId = null;

function randomInt(min, max) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

function randomJitter(base, amplitude) {
  return Number((base + (Math.random() * 2 - 1) * amplitude).toFixed(6));
}

function randomFrom(values, fallback) {
  if (!Array.isArray(values) || values.length === 0) {
    return fallback;
  }
  return values[randomInt(0, values.length - 1)];
}

function uniqueSuffix() {
  return `${__VU}-${__ITER}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function parseSearchResponseRestaurantId(response) {
  if (response.status < 200 || response.status >= 300) {
    return null;
  }
  const payload = response.json();
  if (Array.isArray(payload) && payload.length > 0) {
    return payload[0]?.id || payload[0]?.restaurant_id || null;
  }
  if (Array.isArray(payload?.restaurants) && payload.restaurants.length > 0) {
    return payload.restaurants[0]?.id || payload.restaurants[0]?.restaurant_id || null;
  }
  if (payload?.data) {
    if (Array.isArray(payload.data) && payload.data.length > 0) {
      return payload.data[0]?.id || payload.data[0]?.restaurant_id || null;
    }
    if (typeof payload.data === 'object') {
      return payload.data.id || payload.data.restaurant_id || null;
    }
  }
  return null;
}

function extractOrderId(response) {
  if (!response || response.status < 200 || response.status >= 300) {
    return null;
  }
  const payload = response.json();
  return (
    payload?.order_id ||
    payload?.id ||
    payload?.order?.id ||
    payload?.data?.order_id ||
    payload?.data?.id ||
    null
  );
}

const MAIN_SCENARIO_START = WARMUP_ENABLED ? WARMUP_DURATION : null;

export const options = {
  scenarios: {
    ...(WARMUP_ENABLED
      ? {
          warmup: {
            executor: 'ramping-arrival-rate',
            exec: 'warmupIteration',
            startRate: WARMUP_START_RATE,
            timeUnit: '1s',
            stages: [{ duration: WARMUP_DURATION, target: WARMUP_RATE }],
            preAllocatedVUs: WARMUP_PRE_ALLOCATED_VUS,
            maxVUs: WARMUP_MAX_VUS,
            gracefulStop: '30s',
          },
        }
      : {}),
    searchAndMenu: {
      executor: 'constant-arrival-rate',
      exec: 'searchAndMenuScenario',
      rate: SEARCH_AND_MENU_RPS,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: PRE_ALLOCATED_VUS_READS,
      maxVUs: MAX_VUS_READS,
      ...(MAIN_SCENARIO_START ? { startTime: MAIN_SCENARIO_START } : {}),
      gracefulStop: '30s',
    },
    checkout: {
      executor: 'constant-arrival-rate',
      exec: 'checkoutScenario',
      rate: CHECKOUT_RPS,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: PRE_ALLOCATED_VUS_CHECKOUT,
      maxVUs: MAX_VUS_CHECKOUT,
      ...(MAIN_SCENARIO_START ? { startTime: MAIN_SCENARIO_START } : {}),
      gracefulStop: '30s',
    },
    status: {
      executor: 'constant-arrival-rate',
      exec: 'statusScenario',
      rate: STATUS_RPS,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: PRE_ALLOCATED_VUS_STATUS,
      maxVUs: MAX_VUS_STATUS,
      ...(MAIN_SCENARIO_START ? { startTime: MAIN_SCENARIO_START } : {}),
      gracefulStop: '30s',
    },
  },
  summaryTrendStats: ['avg', 'med', 'max', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    'http_req_failed{scenario:searchAndMenu}': ['rate<0.1'],
    'http_req_failed{scenario:checkout}': ['rate<0.1'],
    'http_req_failed{scenario:status}': ['rate<0.1'],
    'http_req_duration{name:POST /api/v1/orders,scenario:checkout}': ['p(50)<400', 'p(99)<2000'],
    'http_req_duration{name:POST /api/v1/orders,scenario:status}': ['p(50)<400', 'p(99)<2000'],
    'http_req_duration{name:GET /api/v1/restaurants,scenario:searchAndMenu}': ['p(50)<150', 'p(99)<500'],
    'http_req_duration{name:GET /api/v1/restaurants/:id/menu,scenario:searchAndMenu}': [
      'p(50)<150',
      'p(99)<500',
    ],
    'http_req_duration{name:GET /api/v1/orders/:id/tracking,scenario:status}': ['p(50)<100', 'p(99)<300'],
    status_visibility_lag_ms: ['p(99)<3000'],
  },
};

function checkoutPayload(restaurantId) {
  const lat = randomJitter(55.7558, 0.05);
  const lon = randomJitter(37.6173, 0.05);

  return JSON.stringify({
    restaurant_id: restaurantId,
    items: [
      { menu_item_id: ITEM_1, quantity: randomInt(1, 2) },
      { menu_item_id: ITEM_2, quantity: randomInt(1, 2) },
    ],
    delivery_address: {
      lat,
      lon,
      address_text: `stress-${uniqueSuffix()}`,
    },
    comment: `k6-checkout-${uniqueSuffix()}`,
    payment: {
      method: 'card',
      token: `tok_visa_${randomInt(1000, 9999)}`,
    },
  });
}

function parseTimestampMs(value) {
  if (typeof value === 'number') {
    return value > 1e12 ? value : value * 1000;
  }
  if (typeof value === 'string') {
    const parsed = Date.parse(value);
    return Number.isNaN(parsed) ? null : parsed;
  }
  return null;
}
export function searchAndMenuScenario() {
  const lat = randomJitter(55.75, 0.08);
  const lon = randomJitter(37.61, 0.08);
  const radius = randomInt(1000, 7000);
  const query = randomFrom(SEARCH_TERMS, 'sushi');
  const cacheBust = uniqueSuffix();
  const search = http.get(
    `${BASE_URL}${API_PREFIX}/restaurants?lat=${lat}&lon=${lon}&radius=${radius}&query=${encodeURIComponent(query)}&cb=${cacheBust}`,
    { tags: { name: 'GET /api/v1/restaurants' } }
  );
  tSearch.add(search.timings.duration);
  check(search, { 'search: response received': (r) => r.status >= 200 && r.status < 500 });

  const restaurantId = parseSearchResponseRestaurantId(search) || randomFrom(RESTAURANT_IDS, RESTAURANT_ID);
  const menu = http.get(`${BASE_URL}${API_PREFIX}/restaurants/${restaurantId}/menu?cb=${cacheBust}`, {
    tags: { name: 'GET /api/v1/restaurants/:id/menu' },
  });
  tMenu.add(menu.timings.duration);
  check(menu, { 'menu: response received': (r) => r.status >= 200 && r.status < 500 });
  sleep(0.05);
}
export function checkoutScenario() {
  const restaurantId = randomFrom(RESTAURANT_IDS, RESTAURANT_ID);
  const checkout = http.post(`${BASE_URL}${API_PREFIX}/orders`, checkoutPayload(restaurantId), {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'POST /api/v1/orders' },
  });
  tCheckout.add(checkout.timings.duration);
  check(checkout, { 'checkout: response received': (r) => r.status >= 200 && r.status < 500 });

  const createdOrderId = extractOrderId(checkout);
  if (createdOrderId) {
    lastKnownOrderId = createdOrderId;
  }

  sleep(0.05);
}
export function statusScenario() {
  let orderId =
    lastKnownOrderId ||
    randomFrom(STATUS_ORDER_IDS, null) ||
    (ORDER_ID && ORDER_ID !== '7f08ee66-7ebd-45fb-a6af-0d5fb015f1af' ? ORDER_ID : null);

  if (!orderId || __ITER % 20 === 0) {
    const restaurantId = randomFrom(RESTAURANT_IDS, RESTAURANT_ID);
    const checkout = http.post(`${BASE_URL}${API_PREFIX}/orders`, checkoutPayload(restaurantId), {
      headers: { 'Content-Type': 'application/json' },
      tags: { name: 'POST /api/v1/orders' },
    });
    const createdOrderId = extractOrderId(checkout);
    if (createdOrderId) {
      orderId = createdOrderId;
      lastKnownOrderId = createdOrderId;
    }
  }

  if (!orderId) {
    sleep(0.05);
    return;
  }

  const status = http.get(`${BASE_URL}${API_PREFIX}/orders/${orderId}/tracking?cb=${uniqueSuffix()}`, {
    tags: { name: 'GET /api/v1/orders/:id/tracking' },
  });
  tStatus.add(status.timings.duration);
  check(status, { 'status: response received': (r) => r.status >= 200 && r.status < 500 });
  if (status.status >= 200 && status.status < 300) {
    const payload = status.json();
    const changedAt =
      parseTimestampMs(payload?.status_changed_at) ||
      parseTimestampMs(payload?.updated_at) ||
      parseTimestampMs(payload?.changed_at);
    if (changedAt !== null) {
      tStatusVisibilityLag.add(Date.now() - changedAt);
    }
  }
  sleep(0.05);
}

export function warmupIteration() {
  searchAndMenuScenario();
  checkoutScenario();
  statusScenario();
}