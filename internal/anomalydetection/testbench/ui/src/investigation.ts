// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

import type { Anomaly, SeriesID, SeriesInfo } from './api/client';

export interface TimeWindow { start: number; end: number }
export function inWindow(timestamp: number, range?: TimeWindow | null): boolean {
  return !range || (timestamp >= range.start && timestamp <= range.end);
}

/** A raw detector hit is counted once, independently of scorer window occupancy. */
export function countHitsBySeries(anomalies: Anomaly[], range?: TimeWindow | null): Map<SeriesID, number> {
  const counts = new Map<SeriesID, number>();
  for (const a of anomalies) {
    if (a.sourceSeriesId && inWindow(a.timestamp, range)) counts.set(a.sourceSeriesId, (counts.get(a.sourceSeriesId) ?? 0) + 1);
  }
  return counts;
}

/** Resolve only an unambiguous descriptor; never silently choose another tagged series. */
export function resolveMetricContributor(name: string, series: SeriesInfo[]): SeriesInfo | undefined {
  if (name.startsWith('log: ')) return undefined;
  const brace = name.indexOf('{');
  if (brace >= 0 && !name.endsWith('}')) return undefined;
  const metricName = brace < 0 ? name : name.slice(0, brace);
  const tags = brace < 0 ? [] : name.slice(brace + 1, -1).split(',').filter(Boolean).sort();
  const matches = series.filter(s => {
    if (s.name !== metricName) return false;
    const ownTags = [...new Set([...(s.tags ?? []), ...(s.host ? [`host:${s.host}`] : [])])].sort();
    return ownTags.length === tags.length && ownTags.every((t, i) => t === tags[i]);
  });
  return matches.length === 1 ? matches[0] : undefined;
}


export interface SourceInspectionActions {
  onInspectSeries?: (series: SeriesInfo, range: TimeWindow) => void;
  onInspectLogs?: (range: TimeWindow, tags: string) => void;
}

export function onsetWindow(timestamp: number): TimeWindow {
  return { start: timestamp - 60, end: timestamp + 120 };
}

/** Use source identity, not representative log text, to find relevant log rows. */
export function contributorLogTags(name: string): string {
  const separator = name.lastIndexOf(' — {');
  if (!name.startsWith('log: ') || separator < 0 || !name.endsWith('}')) return '';
  return name.slice(separator + 4, -1).split(',')
    .filter(tag => /^(service|container_name|host):/.test(tag)).join(' ');
}
