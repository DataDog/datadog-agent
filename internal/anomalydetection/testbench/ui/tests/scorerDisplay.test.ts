// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

import assert from 'node:assert/strict';
import test from 'node:test';
import { aggregateScoreBuckets, visibleSeverityEvents } from '../src/scorerDisplay.ts';

const bucket = (second: number, count = 4) => ({second, count, bins: [0, 0, count, 0, 0] as [number, number, number, number, number], weight_sum: count, ewma: second / 100});

test('a persistent input is not counted repeatedly when zoom changes bar width', () => {
  const input = Array.from({length: 25}, (_, second) => bucket(second));
  for (const width of [1, 5, 10, 25]) {
    const bars = aggregateScoreBuckets(input, width);
    assert.ok(bars.every(bar => bar.count === 4 && bar.bins[2] === 4));
  }
});

test('a partial final bar uses its actual sample count and final EWMA', () => {
  const bars = aggregateScoreBuckets([bucket(0, 2), bucket(1, 4), bucket(2, 8)], 2);
  assert.deepEqual(bars.map(bar => bar.count), [3, 8]);
  assert.deepEqual(bars.map(bar => bar.ewma), [0.01, 0.02]);
});

test('display aggregation handles missing seconds and empty timelines', () => {
  assert.deepEqual(aggregateScoreBuckets([], 10), []);
  assert.deepEqual(aggregateScoreBuckets([bucket(0), bucket(20)], 10).map(bar => bar.startSec), [0, 20]);
});

const events = [
  {timestamp: 10, from_level: 0, to_level: 1},
  {timestamp: 20, from_level: 1, to_level: 2},
  {timestamp: 30, from_level: 2, to_level: 0},
];

test('zoom retains the preceding transition so ongoing severity remains visible', () => {
  assert.deepEqual(visibleSeverityEvents(events, {start: 25, end: 28}), [events[1]]);
});

test('a transition exactly at the left edge does not duplicate the previous interval', () => {
  const visible = visibleSeverityEvents(events, {start: 20, end: 30});
  assert.deepEqual(visible, [events[1], events[2]]);
});

test('zoom before the first event and a missing range are handled', () => {
  assert.deepEqual(visibleSeverityEvents(events, {start: 1, end: 5}), []);
  assert.deepEqual(visibleSeverityEvents(events), events);
});
