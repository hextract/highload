import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:8080';
const API_PREFIX = __ENV.API_PREFIX || '/api/v1';
const RESTAURANT_ID = __ENV.RESTAURANT_ID || '';
const ITEM_1 = __ENV.MENU_ITEM_1 || '';
const ITEM_2 = __ENV.MENU_ITEM_2 || '';
const SEARCH_TERMS = (__ENV.SEARCH_TERMS || ',дом,семейный,уютный,острый,пицца,бургер')
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
const SEARCH_AND_MENU_RPS = Number(__ENV.SEARCH_AND_MENU_RPS || 150);
const CHECKOUT_RPS = Number(__ENV.CHECKOUT_RPS || 30);
const STATUS_RPS = Number(__ENV.STATUS_RPS || 150);
const DURATION = __ENV.DURATION || '120s';
const PRE_ALLOCATED_VUS_READS = Number(__ENV.PRE_ALLOCATED_VUS_READS || 240);
const MAX_VUS_READS = Number(__ENV.MAX_VUS_READS || 800);
const PRE_ALLOCATED_VUS_CHECKOUT = Number(__ENV.PRE_ALLOCATED_VUS_CHECKOUT || 120);
const MAX_VUS_CHECKOUT = Number(__ENV.MAX_VUS_CHECKOUT || 500);
const PRE_ALLOCATED_VUS_STATUS = Number(__ENV.PRE_ALLOCATED_VUS_STATUS || 200);
const MAX_VUS_STATUS = Number(__ENV.MAX_VUS_STATUS || 700);
const ORDER_ID = __ENV.ORDER_ID || '';

const WARMUP_DURATION = __ENV.WARMUP_DURATION || '40s';
const WARMUP_ENABLED = !/^0s?$|^false$/i.test(String(WARMUP_DURATION).trim());
const WARMUP_RATE = Number(__ENV.WARMUP_RATE || 40);
const WARMUP_START_RATE = Number(__ENV.WARMUP_START_RATE ?? __ENV.WARMUP_INITIAL_RATE ?? 0);
const WARMUP_PRE_ALLOCATED_VUS = Number(__ENV.WARMUP_PRE_ALLOCATED_VUS || 40);
const WARMUP_MAX_VUS = Number(__ENV.WARMUP_MAX_VUS || 150);
const SEARCH_ONLY_RATIO = Number(__ENV.SEARCH_ONLY_RATIO || 0.55);
const MENU_VIEW_RATIO = Number(__ENV.MENU_VIEW_RATIO || 0.35);
const MENU_CACHE_SIZE = Number(__ENV.MENU_CACHE_SIZE || 250);
const MAX_STATUS_POOL = Number(__ENV.MAX_STATUS_POOL || 120);

const tSearch = new Trend('req_search_duration', true);
const tMenu = new Trend('req_menu_duration', true);
const tCreateOrder = new Trend('req_create_order_duration', true);
const tPayOrder = new Trend('req_pay_order_duration', true);
const tStatus = new Trend('req_status_duration', true);
const tStatusVisibilityLag = new Trend('status_visibility_lag_ms', true);

let lastKnownOrderId = null;
const knownRestaurantIds = [...RESTAURANT_IDS].filter(Boolean);
const recentOrderIds = [...STATUS_ORDER_IDS];
const menuCache = new Map();

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

function uuidV4() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    const v = c === 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
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

function tryJson(response) {
  try {
    return response?.json?.();
  } catch (_) {
    return null;
  }
}

function addKnownRestaurantId(id) {
  if (!id || knownRestaurantIds.includes(id)) {
    return;
  }
  knownRestaurantIds.push(id);
}

function addRecentOrderId(id) {
  if (!id) {
    return;
  }
  recentOrderIds.push(id);
  if (recentOrderIds.length > MAX_STATUS_POOL) {
    recentOrderIds.splice(0, recentOrderIds.length - MAX_STATUS_POOL);
  }
}

function pickRecentOrderId() {
  if (recentOrderIds.length === 0) {
    return null;
  }
  return recentOrderIds[randomInt(0, recentOrderIds.length - 1)];
}

function extractRestaurantIds(payload) {
  const restaurants = payload?.restaurants || payload?.data?.restaurants || payload?.data || [];
  if (!Array.isArray(restaurants)) {
    return [];
  }
  const ids = [];
  for (const r of restaurants) {
    const id = r?.id || r?.restaurant_id || r?.uuid;
    if (id) {
      ids.push(id);
    }
  }
  return ids;
}

function extractMenuItems(payload) {
  const categories = payload?.categories || payload?.data?.categories || [];
  const extracted = [];
  if (!Array.isArray(categories)) {
    return extracted;
  }
  for (const cat of categories) {
    const items = cat?.items || [];
    if (!Array.isArray(items)) {
      continue;
    }
    for (const item of items) {
      const itemId = item?.id || item?.menu_item_id;
      if (!itemId) {
        continue;
      }
      extracted.push({
        id: itemId,
        available: item?.is_available !== false,
      });
    }
  }
  return extracted;
}

function rememberMenu(restaurantId, items) {
  if (!restaurantId || !Array.isArray(items) || items.length === 0) {
    return;
  }
  menuCache.set(restaurantId, items);
  if (menuCache.size > MENU_CACHE_SIZE) {
    const firstKey = menuCache.keys().next().value;
    if (firstKey) {
      menuCache.delete(firstKey);
    }
  }
}

function fetchMenu(restaurantId, tagsSuffix = {}) {
  const menu = http.get(`${BASE_URL}${API_PREFIX}/restaurants/${restaurantId}/menu?cb=${uniqueSuffix()}`, {
    headers: {
      'Cache-Control': 'no-cache',
      Pragma: 'no-cache',
    },
    tags: { name: 'GET /api/v1/restaurants/:id/menu', ...tagsSuffix },
  });
  tMenu.add(menu.timings.duration);
  check(menu, { 'menu: response received': (r) => r.status >= 200 && r.status < 500 });
  if (menu.status < 200 || menu.status >= 300) {
    return [];
  }
  const payload = tryJson(menu);
  const items = extractMenuItems(payload);
  rememberMenu(restaurantId, items);
  return items;
}

function chooseRestaurantId() {
  if (knownRestaurantIds.length === 0) {
    return null;
  }
  return randomFrom(knownRestaurantIds, null);
}

function chooseOrderItems(restaurantId) {
  const cachedItems = menuCache.get(restaurantId) || [];
  const available = cachedItems.filter((i) => i.available);
  if (available.length >= 2) {
    const first = available[randomInt(0, available.length - 1)];
    const second = available[randomInt(0, available.length - 1)];
    return [
      { menu_item_id: first.id, quantity: randomInt(1, 3) },
      { menu_item_id: second.id, quantity: randomInt(1, 3) },
    ];
  }

  const fallback = [ITEM_1, ITEM_2].filter(Boolean);
  return fallback.map((itemId) => ({ menu_item_id: itemId, quantity: randomInt(1, 2) }));
}

function discoverRestaurants() {
  const lat = randomJitter(55.75, 0.12);
  const lon = randomJitter(37.61, 0.12);
  const radius = randomInt(3000, 20000);
  const search = http.get(`${BASE_URL}${API_PREFIX}/restaurants?lat=${lat}&lon=${lon}&radius=${radius}&cb=${uniqueSuffix()}`, {
    headers: {
      'Cache-Control': 'no-cache',
      Pragma: 'no-cache',
    },
    tags: { name: 'GET /api/v1/restaurants', path: 'discover' },
  });
  tSearch.add(search.timings.duration);
  if (search.status < 200 || search.status >= 300) {
    return 0;
  }
  const payload = tryJson(search);
  const ids = extractRestaurantIds(payload);
  for (const id of ids) {
    addKnownRestaurantId(id);
  }
  return ids.length;
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
    'http_req_duration{scenario:searchAndMenu}': ['p(50)<150', 'p(99)<500'],
    'http_req_duration{scenario:checkout}': ['p(50)<400', 'p(99)<2000'],
    'http_req_duration{scenario:status}': ['p(50)<100', 'p(99)<300'],
    'http_req_failed{scenario:searchAndMenu}': ['rate<0.1'],
    'http_req_failed{scenario:checkout}': ['rate<0.1'],
    'http_req_failed{scenario:status}': ['rate<0.1'],
    'status_visibility_lag_ms{scenario:status}': ['p(99)<3000'],
  },
};

function checkoutPayload(restaurantId, items) {
  const lat = randomJitter(55.7558, 0.05);
  const lon = randomJitter(37.6173, 0.05);

  return JSON.stringify({
    restaurant_id: restaurantId,
    items,
    delivery_address: {
      lat,
      lon,
      address_text: `stress-${uniqueSuffix()}`,
    },
    comment: `k6-checkout-${uniqueSuffix()}`,
  });
}

function paymentPayload() {
  return JSON.stringify({
    payment_method: 'card',
    card_token: `tok_visa_${randomInt(1000, 9999)}`,
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

function createAndPayOrder(restaurantId, tagsSuffix) {
  if (!restaurantId) {
    return null;
  }
  if (!menuCache.has(restaurantId) && Math.random() < 0.8) {
    fetchMenu(restaurantId, tagsSuffix);
  }
  const orderItems = chooseOrderItems(restaurantId);
  if (!Array.isArray(orderItems) || orderItems.length < 2) {
    return null;
  }
  const cacheBust = uniqueSuffix();
  const createOrder = http.post(
    `${BASE_URL}${API_PREFIX}/orders?cb=${cacheBust}`,
    checkoutPayload(restaurantId, orderItems),
    {
      headers: {
        'Content-Type': 'application/json',
        'Cache-Control': 'no-cache',
        Pragma: 'no-cache',
      },
      tags: { name: 'POST /api/v1/orders', ...tagsSuffix },
    }
  );
  tCreateOrder.add(createOrder.timings.duration);
  check(createOrder, { 'create order: response received': (r) => r.status >= 200 && r.status < 500 });

  const createdOrderId = extractOrderId(createOrder);
  if (!createdOrderId) {
    return null;
  }
  addRecentOrderId(createdOrderId);

  const payOrder = http.post(
    `${BASE_URL}${API_PREFIX}/orders/${createdOrderId}/pay?cb=${uniqueSuffix()}`,
    paymentPayload(),
    {
      headers: {
        'Content-Type': 'application/json',
        'Idempotency-Key': uuidV4(),
        'Cache-Control': 'no-cache',
        Pragma: 'no-cache',
      },
      tags: { name: 'POST /api/v1/orders/:id/pay', ...tagsSuffix },
    }
  );
  tPayOrder.add(payOrder.timings.duration);
  check(payOrder, { 'pay order: response received': (r) => r.status >= 200 && r.status < 500 });
  return createdOrderId;
}

export function searchAndMenuScenario() {
  const lat = randomJitter(55.75, 0.08);
  const lon = randomJitter(37.61, 0.08);
  const radius = randomInt(800, 12000);
  const query = randomFrom(SEARCH_TERMS, '');
  const includeQuery = query.length > 0 && Math.random() < 0.75;
  const queryPart = includeQuery ? `&query=${encodeURIComponent(query)}` : '';
  const cacheBust = uniqueSuffix();
  const search = http.get(
    `${BASE_URL}${API_PREFIX}/restaurants?lat=${lat}&lon=${lon}&radius=${radius}${queryPart}&cb=${cacheBust}`,
    {
      headers: {
        'Cache-Control': 'no-cache',
        Pragma: 'no-cache',
      },
      tags: { name: 'GET /api/v1/restaurants' },
    }
  );
  tSearch.add(search.timings.duration);
  check(search, { 'search: response received': (r) => r.status >= 200 && r.status < 500 });
  if (search.status >= 200 && search.status < 300) {
    const payload = tryJson(search);
    const ids = extractRestaurantIds(payload);
    for (const id of ids.slice(0, 8)) {
      addKnownRestaurantId(id);
    }
  }
  if (knownRestaurantIds.length === 0) {
    discoverRestaurants();
  }

  if (Math.random() > SEARCH_ONLY_RATIO) {
    const restaurantId = chooseRestaurantId();
    if (restaurantId) {
      fetchMenu(restaurantId);
    }
  }

  if (Math.random() > SEARCH_ONLY_RATIO + MENU_VIEW_RATIO) {
    const restaurantId = chooseRestaurantId();
    if (restaurantId) {
      const createdOrderId = createAndPayOrder(restaurantId, { path: 'from_read_flow' });
      if (createdOrderId) {
        lastKnownOrderId = createdOrderId;
      }
    }
  }
  sleep(0.05);
}
export function checkoutScenario() {
  const restaurantId = chooseRestaurantId();
  if (!restaurantId) {
    discoverRestaurants();
    sleep(0.05);
    return;
  }
  if (Math.random() < 0.5) {
    fetchMenu(restaurantId, { path: 'checkout_prefetch' });
  }
  const createdOrderId = createAndPayOrder(restaurantId);
  if (createdOrderId) {
    lastKnownOrderId = createdOrderId;
    addRecentOrderId(createdOrderId);
  }

  sleep(0.05);
}
export function statusScenario() {
  let orderId =
    lastKnownOrderId ||
    pickRecentOrderId() ||
    (ORDER_ID || null);

  if (!orderId || __ITER % 20 === 0) {
    const restaurantId = chooseRestaurantId();
    if (restaurantId) {
      const createdOrderId = createAndPayOrder(restaurantId);
      if (createdOrderId) {
        orderId = createdOrderId;
        lastKnownOrderId = createdOrderId;
        addRecentOrderId(createdOrderId);
      }
    }
  }

  if (!orderId) {
    if (knownRestaurantIds.length === 0) {
      discoverRestaurants();
    }
    sleep(0.05);
    return;
  }

  const status = http.get(
    `${BASE_URL}${API_PREFIX}/orders/${orderId}/tracking?cb=${uniqueSuffix()}`,
    {
      headers: {
        'Cache-Control': 'no-cache',
        Pragma: 'no-cache',
      },
      tags: { name: 'GET /api/v1/orders/:id/tracking' },
    }
  );
  tStatus.add(status.timings.duration);
  check(status, { 'status: response received': (r) => r.status >= 200 && r.status < 500 });
  if (status.status >= 200 && status.status < 300) {
    const payload = status.json();
    const changedAt =
      parseTimestampMs(payload?.status_changed_at) ||
      parseTimestampMs(payload?.updated_at) ||
      parseTimestampMs(payload?.changed_at);
    if (changedAt !== null) {
      tStatusVisibilityLag.add(Date.now() - changedAt, { scenario: 'status' });
    }
  }
  sleep(0.05);
}

export function warmupIteration() {
  searchAndMenuScenario();
  checkoutScenario();
  statusScenario();
}