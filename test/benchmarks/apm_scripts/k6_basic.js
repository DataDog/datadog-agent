import http from 'k6/http';
import { check, sleep } from 'k6';

const payloads = {
  "10traces210spans": open("payloads/10traces210spans.msgp"),
  "10traces210spans.v05": open("payloads/10traces210spans.v5.msgp"),
  "3885traces3891spans.v05": open("payloads/3885traces3891spans.msgp.v0.5"),
  "3885traces3891spans": open("payloads/3885traces3891spans.msgp"),
};

const msgpack_headers = {
  headers: {
    'Content-Type': 'application/msgpack',
  },
};

const json_headers = {
  headers: {
    'Content-Type': 'application/json',
  },
};

const v04_url = 'http://localhost:8126/v0.4/traces';
const v05_url = 'http://localhost:8126/v0.5/traces';

function wait_for_trace_agent_to_startup() {
  while (true) {
    const res = http.post(v04_url, "[]", json_headers);
    sleep(1);
    if (res.status === 200) {
      break;
    }
  }
}

export const options = {
  scenarios: {
    "10traces210spans": {
      exec: 'req_v04',
      executor: 'constant-arrival-rate',
      env: {
        PAYLOAD_NAME: "10traces210spans"
      },
      rate: 1000,
      timeUnit: '1s',
      duration: '600s',
      preAllocatedVUs: 5,
    },
    "3885traces3891spans": {
      exec: 'req_v04',
      executor: 'constant-arrival-rate',
      env: {
        PAYLOAD_NAME: "3885traces3891spans"
      },
      rate: 10,
      timeUnit: '1s',
      duration: '600s',
      preAllocatedVUs: 5,
    },
  },
};

export function setup() {
  wait_for_trace_agent_to_startup()
}

export function req_v04 () {
  const res = http.post(v04_url, payloads[__ENV.PAYLOAD_NAME], msgpack_headers);
  check(res, {
    'is status 200': (r) => r.status === 200,
  });
}

export function req_v05 () {
  const res = http.post(v05_url, payloads[__ENV.PAYLOAD_NAME], msgpack_headers);
  check(res, {
    'is status 200': (r) => r.status === 200,
  });
}

export default function () {

}
