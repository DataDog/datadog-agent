# Observer - Product Roadmap

## Current Capabilities

| Layer | Component | Purpose |
|-------|-----------|---------|
| Detection | CUSUM, Z-Score | Change-points, outliers |
| Dedup | Bloom Filter | 98.7% noise reduction |
| Correlation | TimeCluster | Blast radius (what broke together) |
| | LeadLag | Causal chains (A → B → C) |
| | Surprise | Unexpected co-occurrences |
| | GraphSketch | Learned recurring patterns |

---

## Product Directions

### Edge Monitors
Run monitors on the agent instead of backend.

| Benefit | Description |
|---------|-------------|
| Resilience | Works during ingestion latency/downtime |
| Speed | Detect faster → remediate faster |
| Cost | Monitor signals too expensive to send |

Approaches:
- Zero-config anomaly detection (what we have now)
- Interpret existing configured monitors, re-impl on agent
- Watch dropped tags without sending them

### MCP Integration
- Expose anomalies as a tool for LLM to read
- Kick off investigations proactively
- Anomaly-aware flares / logs / reports

### Cost Control
- Correlate: new config + anomaly → increased cost
- Watch more data locally, only send what matters
- Alternative to sending everything to backend
- Supplement backend data with local detail (do they need this?)

### Anomaly Classification
- Classify anomaly types/patterns
- Better detection via pattern recognition
- Surface insights to users ("this looks like a memory leak pattern")

---

## Open Questions

1. What is the acceptable level of false positives?
2. How much faster do we need to detect? Is speed the killer feature?
3. Do we have data to support the resilience use case?
4. Cost control vs proactive data use - which resonates more?
5. Supplement backend vs replace backend signals?

---

## Action Items

- [ ] Expose anomalies as MCP tool
- [ ] Prototype edge monitor (re-impl simple monitor on agent)
- [ ] Prototype dropped-tag watching
- [ ] Anomaly-aware flare integration
