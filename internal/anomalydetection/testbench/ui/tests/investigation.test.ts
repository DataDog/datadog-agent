// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

import test from 'node:test';
import assert from 'node:assert/strict';
import { countHitsBySeries, resolveMetricContributor, contributorLogTags, onsetWindow } from '../src/investigation.ts';
import type { Anomaly, SeriesInfo, SeriesID } from '../src/api/client.ts';
const id = (value: string) => value as SeriesID;
const metric = (name: string, tags: string[], namespace = 'parquet', host?: string) => ({ id: id(`${namespace}/${name}/${tags}`), namespace, name, tags, host }) as SeriesInfo;

test('raw detector hits count once and include both inspection boundaries', () => {
  const hits = [9, 10, 11, 20, 21].map(timestamp => ({sourceSeriesId: id('one'), timestamp})) as Anomaly[];
  assert.equal(countHitsBySeries(hits, {start: 10, end: 20}).get(id('one')), 3);
  assert.equal(countHitsBySeries(hits).get(id('one')), 5);
});

test('contributor links preserve aggregation, host, and all tag values', () => {
  const wanted = metric('lag:count', ['service:kafka', 'env:test'], 'parquet', 'worker');
  const variants = [metric('lag:avg', ['service:kafka', 'env:test'], 'parquet', 'worker'), metric('lag:count', ['service:other', 'env:test']), wanted];
  assert.equal(resolveMetricContributor('lag:count{host:worker,env:test,service:kafka}', variants), wanted);
  assert.equal(resolveMetricContributor('lag:count{service:kafka}', variants), undefined);
});

test('ambiguous, malformed, or log descriptors never open an unrelated metric', () => {
  assert.equal(resolveMetricContributor('x:avg', [metric('x:avg', [], 'a'), metric('x:avg', [], 'b')]), undefined);
  assert.equal(resolveMetricContributor('log: x:avg', [metric('x:avg', [])]), undefined);
  assert.equal(resolveMetricContributor('x:avg{service:api', [metric('x:avg', ['service:api'])]), undefined);
});

test('log links use source tags rather than a sample line or its embedded timestamp', () => {
  assert.equal(contributorLogTags('log: sample at 03:00 — {env:test,service:checkout,host:worker,container_name:web}'), 'service:checkout host:worker container_name:web');
  assert.equal(contributorLogTags('log: sample without tags'), '');
  assert.deepEqual(onsetWindow(1000), {start: 940, end: 1120});
});
