# DogStatsD Metric Filtering - CPU

## Overview

This experiment measures the Agent's CPU cost when DogStatsD traffic goes
through the metric filters: `metric_tag_filterlist` (tag stripping) and
`metric_filterlist` (dropping metrics by name). Every other DogStatsD
experiment runs with both lists empty.

## Workload

- **Transport**: 10 MiB/s over a DogStatsD Unix datagram socket, from three
  generators.
- **Tag-filtered (6 MiB/s)**: Non-timestamped counts and distributions named
  `smp.filter.tagged.0` through `smp.filter.tagged.19`. Each has an entry in
  `metric_tag_filterlist`. Even-numbered metrics exclude `request_id` and
  `pod_name`. Odd-numbered metrics include only `env` and `service`.
- **Name-filtered (2 MiB/s)**: Counts and gauges named `smp.filter.blocked.0`
  through `smp.filter.blocked.19`. All of them are listed in
  `metric_filterlist`, so the DogStatsD workers drop them at enrichment.
- **Unfiltered (2 MiB/s)**: Counts and distributions named
  `smp.filter.other.0` through `smp.filter.other.19`, which match neither list.
- **Tags**: Six tags per message. Names come from `env`, `service`, `version`,
  `pod_name`, `request_id` and `container_id`, and values from `v-0` to
  `v-999`. Lading picks names and values independently. Each generator builds
  3,000 context templates, so the number of contexts stays bounded.

Tag stripping only applies to counts and distributions. By default it also
only applies when the Agent Data Plane is enabled, so the Agent configuration
sets `metric_tag_filterlist_adp_only: false`.

## What It Tests

- Tag-filter lookup by metric name, and the miss path for unlisted names.
- Tag stripping on a tag-filter cache miss, and the post-filter context-key
  lookup on a cache hit. Contexts are bounded, so the cache stays warm in
  steady state, as it does in production.
- Metric-name filter matching in the DogStatsD workers and at flush.

To confirm a run exercised these paths, check the captured Agent telemetry.
`aggregator.filtered_tags`, `aggregator.filtered_tags_cache_hit` and
`dogstatsd.listener_filtered_points` should all be non-zero.
