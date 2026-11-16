// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

import type { ScoreBucket, SeverityEvent } from './api/client';

export interface DisplayBucket {
  bins: [number, number, number, number, number];
  count: number;
  ewma: number;
  startSec: number;
}

/** Mean active scorer inputs per sampled second; never a count of new detections. */
export function aggregateScoreBuckets(buckets: ScoreBucket[], windowSecs: number): DisplayBucket[] {
  if (buckets.length === 0) return [];
  const width = Math.max(1, Math.floor(windowSecs));
  const start = buckets[0].second;
  const result: (DisplayBucket & { samples: number })[] = [];
  for (const bucket of buckets) {
    const index = Math.floor((bucket.second - start) / width);
    const target = result[index] ??= {
      bins: [0, 0, 0, 0, 0], count: 0, ewma: 0, startSec: start + index * width, samples: 0,
    };
    for (let level = 0; level < 5; level++) target.bins[level] += bucket.bins[level];
    target.count += bucket.count;
    target.ewma = bucket.ewma;
    target.samples++;
  }
  return result.filter(Boolean).map(({ samples, ...bucket }) => ({
    ...bucket,
    bins: bucket.bins.map(value => value / samples) as DisplayBucket['bins'],
    count: bucket.count / samples,
  }));
}

/** Include the transition that establishes the severity at the left edge of a zoom. */
export function visibleSeverityEvents(events: SeverityEvent[], range?: { start: number; end: number } | null): SeverityEvent[] {
  if (!range) return events;
  let previous: SeverityEvent | undefined;
  for (const event of events) {
    if (event.timestamp >= range.start) break;
    previous = event;
  }
  const inside = events.filter(event => event.timestamp >= range.start && event.timestamp <= range.end);
  return previous && inside[0]?.timestamp !== range.start ? [previous, ...inside] : inside;
}
