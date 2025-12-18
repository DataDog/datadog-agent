# CPU Oscillation Test Scenarios - Design Decisions

## The Core Question

We built a 1Hz CPU oscillation detector for containers. The question became: **what should we test it with?**

The naive answer was "create pods that oscillate CPU." But that misses the point. The real question is: **what real-world problems create CPU oscillation patterns that are invisible at standard 15-second sampling but visible at 1Hz?**

## The Nyquist Constraint

Standard container metrics sample at 15-second intervals. By Nyquist, this means:
- 15s sampling can only detect oscillations with period > 30 seconds
- 1Hz sampling can detect oscillations with period > 2 seconds

**The 2-30 second band is invisible at 15s but visible at 1Hz.** This is where our detector provides unique value.

So the design question became: **what interesting failure modes have 2-30 second periodicity?**

## Why We Rejected Our Initial Scenarios

### CFS Throttling (Rejected)

Initial idea: Show a CPU-hungry workload being throttled by CFS limits.

Problem: CFS throttling happens at ~100ms scale (10ms quota per 100ms period). At 1Hz sampling, this gets **aliased** - we just see a steady 10% CPU (the limit averaged out). The oscillation is too fast to detect.

Lesson: Not all oscillation happens in our detectable band.

### Simple I/O Stalls (Rejected)

Initial idea: Alternate between CPU work and blocking I/O.

Problem: While technically correct, this pattern is artificial. Real apps don't cleanly alternate "500ms CPU, then block." The pattern was too synthetic to be useful for validation.

Lesson: Scenarios must be realistic, not just technically correct.

### Simple Retry Storm (Evolved)

Initial idea: App retrying a dead service with exponential backoff.

Problem: The backoff caps quickly and CPU work was too brief. The pattern wasn't prominent enough to validate detection.

Lesson: Scenarios need tuning to produce visible signatures.

## How We Arrived at the Current Scenarios

We asked: **What real production problems would an SRE care about, that K8s can't detect, but that show up as CPU oscillation?**

Constraints:
1. Must have 2-30 second periodicity (our detectable band)
2. Must look "healthy" to K8s (Running, passing probes, no events)
3. Must actually be broken or problematic
4. Must be self-contained (no external dependencies)
5. Must be implementable in Python (simple, portable)

### Scenario 1: Connection Pool Starvation

**How we got here:** "What causes bimodal CPU that averages to 'looks fine'?"

Answer: Resource contention where you're either working at 100% or blocked at 0%.

Real-world example: Database connection pool too small for the number of workers. Each worker either has a connection (doing 2s of CPU-intensive query processing) or is blocked waiting (0% CPU). The average looks moderate, but p99 latency is terrible.

Why K8s can't see it: Pod is Running, CPU averages 25%, no OOM, no restarts. Looks healthy.

Why 1Hz catches it: Clear bimodal distribution - samples are either HIGH or LOW, never medium.

### Scenario 2: Batch Micro-Processing

**How we got here:** "What common pattern has obvious periodicity in the 2-30s range?"

Answer: Queue consumers. They process batches, then wait for more.

Real-world example: Kafka/SQS consumer that processes messages in batches. Batch arrives → CPU spike → processing complete → wait for next batch. Period matches batch arrival rate (3-8s typically).

Why K8s can't see it: Pod is Running, processing messages, average CPU looks fine.

Why 1Hz catches it: Clear sawtooth pattern - spike during processing, valley while waiting.

### Scenario 3: Self-Induced Feedback Loop

**How we got here:** "What's the most insidious 'looks healthy but isn't' scenario?"

Answer: Internal feedback loops that K8s readiness probes can't see.

Real-world example: App with internal health score that degrades under load. When healthy, it accepts more work → degrades → backs off → recovers → accepts more work. Meanwhile, the HTTP health endpoint always returns 200 because it just checks "is the port open?"

Why K8s can't see it: Readiness probe passes. Liveness probe passes. No events. Status: Running.

Why 1Hz catches it: CPU oscillates through three phases (healthy/degraded/critical) with characteristic pattern.

## What These Scenarios Have in Common

1. **They look healthy to K8s.** No restarts, no OOM, probes pass.
2. **They're actually problematic.** Pool starvation causes latency spikes. Batch processing may fall behind. Feedback loops indicate instability.
3. **The oscillation pattern is the signal.** You can't see these problems in average CPU. You need variance/distribution information.
4. **Period is in the 2-30s band.** Too slow for CFS, too fast for 15s sampling.

## What We're Still Learning

- Do the oscillation signatures actually differ between scenarios? Can we classify by pattern?
- What threshold settings minimize false positives while catching real issues?
- Does cross-pod correlation (multiple pods oscillating in sync) indicate different problems?

## Files

- `connection_pool_starvation.py` - 8 workers, 2 connections, bimodal CPU
- `batch_processor.py` - Queue consumer, sawtooth CPU
- `feedback_loop.py` - Internal health oscillation, always-passing readiness probe
- `test-scenarios.yaml` - K8s manifests to deploy all three
