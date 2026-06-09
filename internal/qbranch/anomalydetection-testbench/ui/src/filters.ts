// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Used to filter elements by tags.

export interface ParsedTagFilter {
  /** key:value inclusion tokens, ORed within a key and ANDed across keys */
  include: Map<string, Set<string>>;
  /**
   * Exclusion tokens. Two forms are supported:
   *   `-key:value`  → exact tag excluded (series having that specific tag are excluded)
   *   `-key`        → key excluded (series having any tag with that key are excluded)
   */
  exclude: Set<string>;
}

export function parseTagFilter(input: string): ParsedTagFilter {
  const include = new Map<string, Set<string>>();
  const exclude = new Set<string>();
  for (const token of input.trim().split(/\s+/)) {
    if (!token) continue;
    if (token.startsWith('-') && token.length > 1) {
      exclude.add(token.slice(1)); // store "key:value" or "key"
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

/**
 * 3-state toggle for a tag chip (`key:value`):
 *   not selected  → included  (`key:value` added)
 *   included      → excluded  (`key:value` removed, `-key:value` added)
 *   excluded      → off       (`-key:value` removed)
 */
export function toggleTagInInput(input: string, tag: string): string {
  const sep = tag.indexOf(':');
  if (sep <= 0) return input;
  const negTag = `-${tag}`;

  let tokens = input.trim().split(/\s+/).filter(Boolean);

  if (tokens.includes(negTag)) {
    // Currently excluded → remove exclusion
    tokens = tokens.filter((t) => t !== negTag);
  } else if (tokens.includes(tag)) {
    // Currently included → switch to excluding this specific value
    tokens = tokens.filter((t) => t !== tag);
    tokens.push(negTag);
  } else {
    // Not selected → include
    tokens.push(tag);
  }

  return tokens.join(' ');
}

/** Apply a ParsedTagFilter to a list of tags. Returns true if the tags pass the filter. */
export function matchesTagFilter(tags: string[], filter: ParsedTagFilter): boolean {
  const tagSet = new Set(tags);
  for (const excl of filter.exclude) {
    if (excl.includes(':')) {
      // Exact value exclusion: -key:value
      if (tagSet.has(excl)) return false;
    } else {
      // Key exclusion: -key (any value)
      if ([...tagSet].some((t) => t.startsWith(`${excl}:`))) return false;
    }
  }
  for (const [, tagValues] of filter.include) {
    if (![...tagValues].some((t) => tagSet.has(t))) return false;
  }
  return true;
}