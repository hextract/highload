import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:8080';
const RESTAURANT_ID = __ENV.RESTAURANT_ID || 'f47ac10b-58cc-4372-a567-0e02b2c3d479';
const ITEM_1 = __ENV.MENU_ITEM_1 || 'a1b2c3d4-e5f6-7890-abcd-ef1234567890';
const ITEM_2 = __ENV.MENU_ITEM_2 || 'e3f4a5b6-c7d8-9012-cdef-123456789012';
const READ_RPS = Number(__ENV.READ_RPS || 200);
const WRITE_RPS = Number(__ENV.WRITE_RPS || 40);
const DURATION = __ENV.DURATION || '150s';
const PRE_ALLOCATED_VUS_READS = Number(__ENV.PRE_ALLOCATED_VUS_READS || 220);
const MAX_VUS_READS = Number(__ENV.MAX_VUS_READS || 500);
const PRE_ALLOCATED_VUS_WRITES = Number(__ENV.PRE_ALLOCATED_VUS_WRITES || 60);
const MAX_VUS_WRITES = Number(__ENV.MAX_VUS_WRITES || 250);
const tRestaurants = new Trend('req_restaurants_duration', true);
const tMenu = new Trend('req_menu_duration', true);
const tCreate = new Trend('req_create_order_duration', true);
const tPay = new Trend('req_pay_duration', true);
const tTracking = new Trend('req_tracking_duration', true);

export const options = {
  scenarios: {
    reads: {
      executor: 'constant-arrival-rate',
      exec: 'readsScenario',
      rate: READ_RPS,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: PRE_ALLOCATED_VUS_READS,
      maxVUs: MAX_VUS_READS,
    },
    writes: {
      executor: 'constant-arrival-rate',
      exec: 'writesScenario',
      rate: WRITE_RPS,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: PRE_ALLOCATED_VUS_WRITES,
      maxVUs: MAX_VUS_WRITES,
    },
  },
  summaryTrendStats: ['avg', 'med', 'max', 'p(99)', 'p(99.9)'],
  thresholds: {
    http_req_failed: ['rate<0.08'],
    http_req_duration: ['p(99)<6000', 'p(99.9)<9000'],
    req_restaurants_duration: ['p(99)<5500', 'p(99.9)<8500'],
    req_menu_duration: ['p(99)<5500', 'p(99.9)<8500'],
    req_create_order_duration: ['p(99)<7000', 'p(99.9)<10000'],
    req_pay_duration: ['p(99)<7000', 'p(99.9)<10000'],
    req_tracking_duration: ['p(99)<6500', 'p(99.9)<9500'],
  },
};

function randomUUID() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    const v = c === 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

function createOrderPayload() {
  return JSON.stringify({
    restaurant_id: RESTAURANT_ID,
    items: [
      { menu_item_id: ITEM_1, quantity: 1 },
      { menu_item_id: ITEM_2, quantity: 1 },
    ],
    delivery_address: {
      lat: 55.7558,
      lon: 37.6173,
      address_text: 'stress test',
    },
    comment: 'k6-stress',
  });
}

export function readsScenario() {
  const restaurants = http.get(`${BASE_URL}/api/v1/restaurants?lat=55.75&lon=37.61&radius=5000`, {
    tags: { name: 'GET /api/v1/restaurants' },
  });
  tRestaurants.add(restaurants.timings.duration);
  check(restaurants, { 'reads: restaurants 200': (r) => r.status === 200 });

  const menu = http.get(`${BASE_URL}/api/v1/restaurants/${RESTAURANT_ID}/menu`, {
    tags: { name: 'GET /api/v1/restaurants/:id/menu' },
  });
  tMenu.add(menu.timings.duration);
  check(menu, { 'reads: menu 200': (r) => r.status === 200 });

  sleep(0.2);
}

export function writesScenario() {
  const create = http.post(`${BASE_URL}/api/v1/orders`, createOrderPayload(), {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'POST /api/v1/orders' },
  });
  tCreate.add(create.timings.duration);
  const created = check(create, { 'writes: order 201': (r) => r.status === 201 });
  if (!created) {
    sleep(0.2);
    return;
  }

  const order = create.json();
  if (!order?.order_id) {
    sleep(0.2);
    return;
  }

  const pay = http.post(
    `${BASE_URL}/api/v1/orders/${order.order_id}/pay`,
    JSON.stringify({ payment_method: 'card', card_token: 'tok_visa_4242' }),
    {
      headers: {
        'Content-Type': 'application/json',
        'Idempotency-Key': randomUUID(),
      },
      tags: { name: 'POST /api/v1/orders/:id/pay' },
    }
  );
  tPay.add(pay.timings.duration);
  check(pay, { 'writes: pay 202': (r) => r.status === 202 });

  const tracking = http.get(`${BASE_URL}/api/v1/orders/${order.order_id}/tracking`, {
    tags: { name: 'GET /api/v1/orders/:id/tracking' },
  });
  tTracking.add(tracking.timings.duration);
  check(tracking, { 'writes: tracking 200': (r) => r.status === 200 });

  sleep(0.2);
}
