# Observer Correlator Evaluation Matrix

Generated: 2026-01-30 13:33

## Full Results Matrix

| Scenario | TC | TC+D | LL | LL+D | LL+D+T | SP | SP+D | SP+D+T | Best |
|----------|-----|------|-----|------|--------|-----|------|--------|------|
| memory-leak | 78 | 92 | 85 | 95 | 95.0 | 78 | 82 | - | **95** (LL+D) |
| crash-loop | 92 | 85 | 2 | 75 | - | 78 | 78 | 95.0 | **95.0** (SP+D+T) |
| connection-timeout | 8 | 5 | 3 | 5 | - | 5 | 5 | - | **8** (TC) |
| memory-exhaustion | 15 | 15 | 75 | 75 | 78.0 | 5 | 45 | 40.0 | **78.0** (LL+D+T) |
| traffic-spike | 65 | 35 | 5 | 65 | 15.0 | 25 | 35 | - | **65** (TC) |
| network-latency | 15 | 15 | 5 | 5 | - | 2 | 2 | - | **15** (TC) |

**Legend:** TC=TimeCluster, LL=LeadLag, SP=Surprise, +D=Dedup, +T=Tuned

## Best Config Per Scenario

| Scenario | Best Config | Score |
|----------|-------------|-------|
| memory-leak | timecluster+dedup+tuned | **98.0** |
| crash-loop | surprise+dedup+tuned | **95.0** |
| connection-timeout | timecluster+no_dedup | **8** |
| memory-exhaustion | leadlag+dedup+tuned | **78.0** |
| traffic-spike | timecluster+dedup+tuned | **78.0** |
| network-latency | timecluster+no_dedup | **15** |

**Best possible average: 62.0**
