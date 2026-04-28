import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:8080';
const RESTAURANT_ID = __ENV.RESTAURANT_ID || 'f47ac10b-58cc-4372-a567-0e02b2c3d479';
const SPIKE_RPS = Number(__ENV.SPIKE_RPS || 300);
const RAMP_UP = __ENV.RAMP_UP || '15s';
const HOLD = __ENV.HOLD || '60s';
const RAMP_DOWN = __ENV.RAMP_DOWN || '15s';
const PRE_ALLOCATED_VUS = Number(__ENV.PRE_ALLOCATED_VUS || 350);
const MAX_VUS = Number(__ENV.MAX_VUS || 800);
const tRestaurants = new Trend('req_restaurants_duration', true);
const tMenu = new Trend('req_menu_duration', true);

export const options = {
  scenarios: {
    spike_rps: {
      executor: 'ramping-arrival-rate',
      startRate: 1,
      timeUnit: '1s',
      preAllocatedVUs: PRE_ALLOCATED_VUS,
      maxVUs: MAX_VUS,
      stages: [
        { duration: RAMP_UP, target: SPIKE_RPS },
        { duration: HOLD, target: SPIKE_RPS },
        { duration: RAMP_DOWN, target: 0 },
      ],
    },
  },
  summaryTrendStats: ['avg', 'med', 'max', 'p(99)', 'p(99.9)'],
  thresholds: {
    http_req_failed: ['rate<0.1'],
    http_req_duration: ['p(99)<7000', 'p(99.9)<10000'],
    req_restaurants_duration: ['p(99)<7000', 'p(99.9)<10000'],
    req_menu_duration: ['p(99)<7000', 'p(99.9)<10000'],
  },
};

export default function () {
  // Flash sale pattern: mostly bursty reads.
  const restaurants = http.get(`${BASE_URL}/api/v1/restaurants?lat=55.75&lon=37.61&radius=5000`, {
    tags: { name: 'GET /api/v1/restaurants' },
  });
  tRestaurants.add(restaurants.timings.duration);
  check(restaurants, { 'restaurants 200': (r) => r.status === 200 });

  if (__ITER % 2 === 0) {
    const menu = http.get(`${BASE_URL}/api/v1/restaurants/${RESTAURANT_ID}/menu`, {
      tags: { name: 'GET /api/v1/restaurants/:id/menu' },
    });
    tMenu.add(menu.timings.duration);
    check(menu, { 'menu 200': (r) => r.status === 200 });
  }

  sleep(0.1);
}
