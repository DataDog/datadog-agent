# Experimental container performance counters

The Linux `noisy_neighbor` check can collect interval performance-counter metrics for containers. This feature is experimental and disabled by default. It requires Linux kernel 6.2 or newer, a cgroup v2 host, access to the kernel PMU, and both the system-probe module and an Agent check instance.

Enable the module and selected events in `system-probe.yaml`:

```yaml
noisy_neighbor:
  enabled: true
  max_tracked_cgroups: 64
  pmu_metrics:
    cycles: true
    instructions: true
    cache_misses: true
    cache_references: true
    itlb_misses: true
    branch_misses: true
    cpu_migrations: true
```

Enable the check with `conf.d/noisy_neighbor.d/conf.yaml`:

```yaml
instances:
  - {}
```

The check reports interval counts named `noisy_neighbor.cycles`, `noisy_neighbor.instructions`, `noisy_neighbor.cache_misses`, `noisy_neighbor.cache_references`, `noisy_neighbor.itlb_misses`, `noisy_neighbor.branch_misses`, and `noisy_neighbor.cpu_migrations`. Metrics are emitted only for cgroups resolved to containers and include high-cardinality container tags.

Hardware events are multiplex-scaled. An unavailable hardware event is disabled unless it can be opened on every online CPU. Hardware PMU collection is disabled on hosts with more than 128 online CPUs, bounding usage to 768 perf FDs. The check samples up to `max_tracked_cgroups` per 10-second window and rotates fairly through live containers. Module statistics report the configured and effective event masks, perf FD count, online CPUs, watchlist state, errors, and last rotation time.
