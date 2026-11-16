// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { inspectionWindow, recordingWindow, selectMetricGroups } from '../src/metricInspection.ts';
import type { EpisodeInfo } from '../src/api/client.ts';

const episode = {
  start_time: '2026-04-02T16:00:00Z', end_time: '2026-04-02T16:30:00Z',
  baseline: { start: '2026-04-02T16:00:00Z', end: '2026-04-02T16:10:00Z' },
  disruption: { start: '2026-04-02T16:10:00Z', end: '2026-04-02T16:20:00Z' },
  cooldown: { start: '2026-04-02T16:20:00Z', end: '2026-04-02T16:30:00Z' },
} as EpisodeInfo;

test('phase presets use recording metadata and custom selection is independent of zoom', () => {
  const full = recordingWindow(episode)!;
  for (const scope of ['baseline', 'disruption', 'cooldown'] as const) {
    const range = inspectionWindow(scope, episode, null, full)!;
    assert.equal(range.end - range.start, 600);
    assert.ok(range.start >= full.start && range.end <= full.end);
  }
  const custom = { start: full.start + 5, end: full.start + 10 };
  assert.deepEqual(inspectionWindow('custom', episode, custom, full), custom);
  assert.deepEqual(inspectionWindow('recording', episode, custom, full), full);
});

test('missing or invalid phases cannot masquerade as full-recording inspection', () => {
  assert.equal(inspectionWindow('disruption'), null);
  assert.equal(inspectionWindow('custom', episode, null), null);
  assert.equal(inspectionWindow('baseline', { baseline: { start: 'bad', end: 'bad' } } as EpisodeInfo), null);
  assert.deepEqual(recordingWindow(null, { start: 1, end: 10 }), { start: 1, end: 10 });
});

test('reranking replaces automatic results while preserving manual pins in order', () => {
  const pins = new Set(['waiting-clients', 'max-wait']);
  const before = selectMetricGroups(pins, ['dns', 'disk', 'cpu'], 2);
  const after = selectMetricGroups(pins, ['cpu', 'dns', 'new'], 2);
  assert.deepEqual([...before.selected], ['waiting-clients', 'max-wait', 'dns', 'disk']);
  assert.deepEqual([...after.selected], ['waiting-clients', 'max-wait', 'cpu', 'dns']);
  assert.deepEqual([...pins], ['waiting-clients', 'max-wait']);
});

test('expansion adds results, deduplicates pinned candidates, and vanishes at capacity', () => {
  const candidates = Array.from({ length: 60 }, (_, i) => `metric-${i}`);
  const initial = selectMetricGroups(['manual', 'metric-0'], candidates, 5);
  const expanded = selectMetricGroups(['manual', 'metric-0'], candidates, 50);
  assert.equal(initial.selected.size, 7);
  assert.equal(initial.more, 43);
  assert.equal(expanded.selected.size, 50);
  assert.equal(expanded.more, 0);
  assert.ok([...initial.selected].every(key => expanded.selected.has(key)));
  assert.equal(expanded.ranked.includes('metric-0'), false);
});

test('empty inspection keeps pinned charts available for comparison', () => {
  const selection = selectMetricGroups(['waiting-clients'], [], 5);
  assert.deepEqual([...selection.selected], ['waiting-clients']);
  assert.equal(selection.more, 0);
  assert.equal(selectMetricGroups([], [], 5).selected.size, 0);
});

test('a pinned-only view and a full pin budget never accidentally add automatic charts', () => {
  assert.deepEqual([...selectMetricGroups(['one'], ['two'], 0).selected], ['one']);
  const pins = Array.from({ length: 50 }, (_, i) => `pin-${i}`);
  const selection = selectMetricGroups(pins, ['automatic'], 50);
  assert.equal(selection.selected.size, 50);
  assert.equal(selection.ranked.length, 0);
  assert.equal(selection.more, 0);
});

test('healthy controls inspect their evaluation window without inventing fault phases', () => {
  const healthy = {mode: 'healthy-control', evaluation_window: {start: episode.start_time, end: episode.end_time}} as EpisodeInfo;
  assert.deepEqual(inspectionWindow('evaluation', healthy), recordingWindow(episode));
  assert.equal(inspectionWindow('disruption', healthy), null);
});
