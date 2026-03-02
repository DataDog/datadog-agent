// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Used to filter elements by tags.

export interface ParsedTagFilter {
  /** key:value inclusion tokens, ORed within a key and ANDed across keys */
  include: Map<string, Set<string>>;
  /** tag keys whose presence on a series excludes it (-tagkey syntax) */
  exclude: Set<string>;
}

export function parseTagFilter(input: string): ParsedTagFilter {
  const include = new Map<string, Set<string>>();
  const exclude = new Set<string>();
  for (const token of input.trim().split(/\s+/)) {
    if (!token) continue;
    if (token.startsWith('-') && token.length > 1 && !token.includes(':')) {
      exclude.add(token.slice(1));
    } else {
      const sep = token.indexOf(':');
      if (sep <= 0 || sep === token.length - 1) continue;
      const key = token.slice(0, sep);
      if (!include.has(key)) include.set(key, new Set());
      include.get(key)!.add(token);
    }
  }
  return { include, exclude };
}

export function extractTagGroups(tagLists: string[][]): Map<string, string[]> {
  const groups = new Map<string, Set<string>>();
  for (const tags of tagLists) {
    for (const tag of tags ?? []) {
      const sep = tag.indexOf(':');
      if (sep === -1) continue;
      const key = tag.slice(0, sep);
      if (!groups.has(key)) groups.set(key, new Set());
      groups.get(key)!.add(tag);
    }
  }
  return new Map([...groups.entries()].map(([k, v]) => [k, [...v].sort()]));
}

export function toggleTagInInput(input: string, tag: string): string {
  const tokens = input.trim().split(/\s+/).filter(Boolean);
  const idx = tokens.indexOf(tag);
  if (idx >= 0) tokens.splice(idx, 1);
  else tokens.push(tag);
  return tokens.join(' ');
}

/** Apply a ParsedTagFilter to a list of tags. Returns true if the tags pass the filter. */
export function matchesTagFilter(tags: string[], filter: ParsedTagFilter): boolean {
  const tagSet = new Set(tags);
  for (const key of filter.exclude) {
    if ([...tagSet].some((t) => t.startsWith(`${key}:`))) return false;
  }
  for (const [, tagValues] of filter.include) {
    if (![...tagValues].some((t) => tagSet.has(t))) return false;
  }
  return true;
}