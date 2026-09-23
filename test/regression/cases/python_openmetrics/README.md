# Quality Gate OpenMetrics Experiment

## Overview

This experiment measures the Datadog Agent's CPU, memory, and average 
check execution time when scraping a high-volume OpenMetrics endpoint. It validates 
that the agent can process many concurrent OpenMetrics check instances without exceeding
defined resource bounds.

## What It Tests

The experiment models a scenario where a single agent is responsible for
scraping 500 unique OpenMetrics check instances, all pointed at the same
endpoint but differentiated by a `target` query parameter. Each instance
collects all metrics (`.*`) every 15 seconds with a cap of 10,000 returned
metrics per scrape.

This simulates a deployment where one agent fans out across many scrape targets
(e.g. a large fleet of exporters or a multi-tenant Prometheus setup) and
exercises the agent's ability to schedule and execute checks at scale.

## Parameter Derivation

### OpenMetrics Endpoint (lading)

Lading serves a synthetic OpenMetrics payload on port 9100 with approximately
9,000 samples per scrape:

- **4,000 counters** and **4,000 gauges** (prefixed `om_test_`)
- **50 histograms** with 10 buckets each
- **50 summaries** with 5 quantiles each

Each metric is labeled with a realistic set of dimensions:

- 20 services (e.g. `checkout`, `catalog`, `payments`)
- 8 regions (e.g. `us-east-1`, `eu-central-1`)
- 6 HTTP methods, 4 status classes
- 30 consumers
- 200 routes (generated via `route_count`)

### Check Configuration

- **500 check instances**, each targeting a unique `?target=NNNN` endpoint
- **15-second collection interval** per instance
- **4 check runners** to parallelize execution
- All metrics matched via `.*` wildcard

### Experiment  Bounds

- **CPU usage**: average total CPU must stay below 1,500 millicores
- **Memory usage**: total PSS must stay below 4.75 GiB
- **Check execution time**: average per-instance execution time must stay below 100ms. 
  This is critical — if individual check runs take too long, the 500 instances cannot 
  all complete within their 15-second collection interval, causing scheduling delays and
  stale metrics.
