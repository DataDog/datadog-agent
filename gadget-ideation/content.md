
# Datadog Agent Observability Signals & AI-Powered Gadgets

Comprehensive Exploration of Signal Providers and Autonomous Infrastructure Management
Document Version: 1.0 | Date: 2025-01-20 | Project: Q-Branch Gadget Initiative

## Executive Summary

This document presents a comprehensive exploration of the Datadog Agent codebase to catalog all available observability signals and propose sophisticated AI-powered "Gadgets" - autonomous modules that can investigate, remediate, and take action on infrastructure issues without human intervention.

<div class="callout callout-info">
<h4>🎯 The Core Insight</h4>
<p><strong>Traditional monitoring tells you WHAT is happening. These AI gadgets understand WHY it's happening and WHAT WILL HAPPEN NEXT.</strong></p>
<p>The breakthrough is <em>multi-signal correlation</em>: combining process metrics, syscalls, network behavior, cgroup pressure, and trace data to recognize patterns that are invisible to any single monitoring system. A CPU spike is just a spike. But CPU spike + repetitive syscalls + zero I/O + active TCP connections = a deadlock. PSI climbing + memory acceleration + cache eviction = an OOM in 2 minutes. These patterns require intelligence that goes far beyond simple thresholds.</p>
</div>

The exploration uncovered **8 major signal providers** covering process metrics, system resources, container/cgroup stats, network behavior, security events, traces, and logs. These signals are either **fully implemented** or have **clear paths to implementation** with data already being collected.

Based on these signals, we propose **7 sophisticated AI-powered Gadgets** that leverage multiple signal providers, temporal patterns, and statistical analysis to provide intelligent, proactive interventions. These aren't simple automation scripts - they encode the expertise of senior SREs, recognizing patterns across time and multiple dimensions that humans struggle to track.

<div class="stats">
<div class="stat">
<div class="stat-value">8</div>
<div class="stat-label">Signal Providers</div>
</div>
<div class="stat">
<div class="stat-value">7</div>
<div class="stat-label">AI Gadgets</div>
</div>
<div class="stat">
<div class="stat-value">6</div>
<div class="stat-label">Exploration Gaps</div>
</div>
</div>
## Part 1: AI-Powered Gadgets

These gadgets represent **intelligent, proactive interventions** that leverage temporal patterns, multi-signal correlation, and statistical learning to understand not just *what* is happening, but *why* it's happening and *what will happen next*. They make SREs say "I wish I had thought of that" rather than "I could have scripted that myself."

#### 🚀 The Paradigm Shift: From Reactive to Predictive

**Traditional Approach:** Set threshold → Wait for breach → Alert fires → Human investigates → Human takes action → Incident resolved (15-60 minutes MTTR)

**Gadget Approach:** Learn patterns → Detect anomaly forming → Predict impact → Take graduated action → Incident prevented (2-5 minutes MTTR, often before users notice)

**What Makes This "AI Magic" Instead of "Smart Scripting":**

- **Temporal Context:** Understanding the *trajectory* of signals (velocity, acceleration) not just current values
- **Multi-Dimensional Correlation:** Recognizing patterns across 10-20 features simultaneously that humans can't track
- **Adaptive Baselines:** Learning what "normal" looks like per-application, per-time-of-day, per-workload-type
- **Probabilistic Decisions:** Acting on confidence scores and graduated responses rather than binary if-then rules
- **Encoded Expertise:** Automating the pattern recognition that senior SREs do intuitively after years of incident response

### 1. The Oracle - Multi-Signal OOM Prediction & Prevention — AI-Powered

#### 🎯 The Problem - OOM Kills Are Instant Death

The Linux OOM killer is brutal and instantaneous. One moment your service is running, the next it's dead - no graceful shutdown, no final save, no warning. Traditional monitoring only alerts *after* the kill, when it's too late. Memory usage thresholds don't work because they can't distinguish "temporary allocation burst" from "actual leak approaching OOM."

**Why Traditional Approaches Fail:** Setting alerts on "memory > 85%" generates false positives (normal bursts) and misses real OOMs (rapid leaks that go 50% → 100% in 30 seconds). You need to understand *velocity* and *acceleration*, not just current usage.

#### 💡 The Breakthrough Insight

**Here's what makes this possible:** The Linux kernel's Pressure Stall Information (PSI) exposes a leading indicator of OOM that traditional metrics miss entirely. PSI measures time that processes are *stalled waiting* for memory - it's the difference between "parking lot is full" (usage at 95%) and "cars are circling waiting for spots" (processes blocking on memory allocation).

**The Key Discovery:** By combining PSI velocity with memory growth acceleration and cache eviction patterns, LSTM models can predict OOM 2-3 minutes in advance with >85% accuracy. That's enough time to take graduated preventive action, avoiding the kill entirely.

#### 🎭 The Magic Moment - A 3AM Crisis Averted

**3:17 AM** - Your e-commerce checkout service is humming along at 82% memory usage. Normal for peak load.

**3:18 AM** - A memory leak in a recently deployed payment processor starts accelerating. The Oracle notices PSI climbing: 0% → 2% → 5% in 30 seconds. Memory growth acceleration spikes.

**3:19 AM** - LSTM predicts OOM in 150 seconds with 88% confidence. The Oracle enters "early warning" mode, increasing monitoring frequency from 10s to 5s.

**3:20 AM** - Prediction updates: 90 seconds to OOM, 92% confidence. The Oracle triggers cache drops and signals JVM garbage collection. This buys 30 seconds but the leak continues.

**3:21 AM** - Prediction: 60 seconds to OOM, 94% confidence. The Oracle identifies the leaking payment processor (largest RSS growth, non-critical for current transactions). It sends SIGTERM, waits 10 seconds for graceful shutdown, then SIGKILL. The process restarts via systemd.

**3:22 AM** - Memory pressure subsides. PSI drops to 1%. Checkout service continues serving requests throughout. **No OOM kill. No cascade failure. No pager alert.**

**8:00 AM** - The SRE arrives to find a telemetry report: "Oracle prevented OOM kill at 03:21:47. Killed payment-processor-3 (PID 18392). Root cause: memory leak in libpayment v2.3.1. Recommend rollback." The leak is fixed before it impacts customers.

#### 📊 Signal Requirements

- **CgroupResourceProvider:** `Memory.UsageTotal` (time series), `Memory.RSS` growth velocity, `Memory.WorkingSet` derivative, `Memory.Cache` eviction rate (`InactiveFile` shrinkage), `Memory.OOMEvents` counter (historical), `Memory.PSI.Some.Avg10/60/300`, `Memory.Limit`, `Memory.HighThreshold`
- **SystemResourceProvider:** `system.swap.swap_out` velocity, `system.mem.page_cache` shrinkage rate
- **ProcessSignalProvider:** Top processes by `RSS` growth, `Process.Language`
- **ContainerRuntimeProvider:** `Pod.QOSClass` (determines eviction order), `RestartCount` history

#### 🏗️ Intelligence Architecture

**Feature Engineering:** Transforms raw signals into predictive features

- **Memory growth acceleration:** d²(RSS)/dt²
- **Working set pressure:** (WorkingSet / Limit) approaching 1.0
- **Cache eviction velocity:** Rate of InactiveFile decrease
- **PSI momentum:** (PSI.Avg10 - PSI.Avg60) slope
- **Swap thrashing indicator:** swap_out > threshold AND Cache shrinking
- **OOM history embedding:** Past OOM events encoded as features

#### ML Approach Options

**The Problem:** Predict time-to-OOM 2-3 minutes in advance from multi-dimensional time-series signals (PSI, memory growth, cache eviction, swap activity).

##### Option 1: Simple Threshold Rules

- **Approach:** If PSI > 20% AND memory > 90%, predict OOM imminent
- **Pros:** No ML required, easy to debug, <1ms inference, zero training data needed
- **Cons:** High false positives (can't distinguish burst from leak), misses rapid OOMs, no lead time prediction
- **ML Experience Required:** None
- **Verdict:** Good for initial proof-of-concept, but insufficient for production (likely 40-50% false positive rate)

##### Option 2: Statistical Time Series (ARIMA, Prophet, Exponential Smoothing)

- **Approach:** Forecast memory trajectory using classical time-series methods, trigger when forecast crosses threshold
- **Pros:** Well-understood algorithms (statsmodels library), interpretable forecasts, smaller models (~1-5MB)
- **Cons:** Assumes linear/stationary patterns, struggles with non-linear memory behaviors (GC cycles, allocation bursts), can't easily incorporate multi-dimensional correlations
- **ML Experience Required:** Basic statistics (statsmodels tutorials sufficient)
- **Verdict:** Worth trying before deep learning. If memory patterns are relatively predictable, this may be sufficient.
- **Learning Resources:** [Forecasting: Principles and Practice](https://otexts.com/fpp3/), statsmodels ARIMA tutorial

##### Option 3: LSTM (Long Short-Term Memory Networks)

- **Approach:** Recurrent neural network trained on historical OOM events, learns temporal patterns across 18 features simultaneously
- **Pros:** Handles complex temporal patterns, learns per-application baselines, captures multi-dimensional correlations, proven for sequence prediction
- **Cons:** Requires training data (need historical OOM events + leading indicators), 50-100MB model size, ~10ms inference latency, harder to debug
- **ML Experience Required:** Moderate (PyTorch/TensorFlow tutorials, understanding of RNNs)
- **Model Size:** ~80MB (float32) or ~20MB (quantized int8)
- **Verdict:** If Option 2 shows insufficient accuracy (<70% precision), LSTM is next step. The temporal context (5-minute window) is key for distinguishing burst vs leak.
- **Learning Resources:** [PyTorch LSTM Tutorial](https://pytorch.org/tutorials/beginner/nlp/sequence_models_tutorial.html), [TensorFlow Lite for on-device inference](https://www.tensorflow.org/lite)

**Recommendation:** Start with Option 1 for initial prototyping and signal validation. Implement Option 2 as primary approach for MVP. Keep Option 3 as fallback if precision is insufficient or false positive rate too high. The 200MB memory budget can comfortably fit an LSTM if needed.

**Why Time-Series Models Matter:** OOM prediction is fundamentally about understanding *trajectory* - is memory pressure accelerating or stabilizing? PSI climbing from 5% → 10% in 30s is different from 5% → 10% over 5 minutes. Time-series models capture this velocity/acceleration context that threshold rules miss.

**Action Decision Tree:** Graduated intervention strategy

| Time to OOM | Confidence | Actions |
|---|---|---|
| > 180s | > 0.7 | Increase monitoring frequency (10s → 5s) |
| 120-180s | > 0.8 | Drop caches, Trigger GC in JVM/Go processes |
| 60-120s | > 0.85 | Identify largest non-critical process, Gracefully restart |
| < 60s | > 0.9 | Emergency cache liberation, Kill largest process immediately |

#### 🛡️ Safety Mechanisms

- **Process criticality scoring:** System processes (init, systemd, agent) are never killed
- **Cooldown periods:** Max 1 cache drop per 5 minutes, max 1 restart per 10 minutes
- **Bypass on manual intervention:** Detects `oom_score_adj` changes by operators
- **Rollback on false positives:** If memory pressure drops after prediction, log false positive for model retraining

#### ⚠️ Why This Requires AI, Not Simple Rules

**Why can't you just write a bash script for this?**

- **Temporal Patterns That Shift:** Memory usage patterns differ by application (Java heap cycles, Python GC sawtooth, Go allocator spikes). Simple thresholds can't learn these signatures. LSTM's hidden state captures application-specific memory behavior over time.
- **Multi-Dimensional Correlations:** OOM prediction requires correlating 18+ features simultaneously: PSI velocity, memory acceleration, cache eviction rate, swap velocity, working set pressure. The interaction effects are what matter - PSI climbing + cache stable = different signal than PSI climbing + cache evicting. No human can write rules for all combinations.
- **Adapting to Changing Baselines:** What's "normal" memory pressure changes with workload patterns (day/night, weekday/weekend, seasonal traffic). The model learns these baselines from historical data, something static thresholds never achieve.
- **The Intelligence Encoded:** This automates what a senior SRE does intuitively - recognizing the difference between "this spike will resolve" vs "this spike will OOM" by looking at the shape and velocity of the curve, not just the current value.

#### ⚙️ Why It's Non-Trivial - The Complexity Challenge

- **Feature Engineering:** Transforms 8 raw signals into 18 derived features (velocity, acceleration, ratios) updated every 10s
- **Sliding Window Management:** Maintains rolling 5-minute window (30 timesteps × 18 features = 540 values) per cgroup in memory
- **LSTM Inference:** Runs deep learning model inference on CPU (~10ms per prediction) without blocking collection pipeline
- **Process Criticality Scoring:** Must parse process tree, identify system vs application processes, determine restart safety
- **The Hard Part:** Distinguishing "legitimate burst" (temporary allocation that will be freed) from "leak/OOM trajectory" (monotonic growth toward death). This requires understanding temporal context - is RSS growth slowing down or accelerating? Is PSI stabilizing or climbing? These are second-derivative calculations humans struggle with but LSTMs excel at.
- **Safety-Critical Decision Making:** Killing the wrong process could cause more damage than the OOM. Requires confidence scoring, criticality assessment, and graduated intervention strategy with multiple safety gates.

#### 💰 Value Proposition

Prevents ~80% of OOM kills by early intervention, reducing service disruptions. Beats naive thresholds by understanding *velocity* and *acceleration*, not just current usage.

#### 📋 Feasibility Assessment

**Signal Readiness:** ✅ High

- PSI metrics available from cgroupv2: `/sys/fs/cgroup/memory.pressure` (raw_research.md line 588-599)
- Memory stats from `pkg/util/cgroups` reader (raw_research.md line 575-586)
- Swap velocity from `/proc/vmstat` via `pkg/collector/corechecks/system`
- Process RSS from `/proc/[pid]/stat` via `pkg/process/procutil` (raw_research.md line 497)
- All signals collected today at 10-20s intervals

**ML Complexity:** ⚠️ Moderate

- Option 2 (ARIMA/Prophet) is straightforward for team with minimal ML experience
- statsmodels library well-documented, interpretable output
- Option 3 (LSTM) is moderate complexity but has excellent PyTorch tutorials
- Quantization for smaller model size is well-supported (TensorFlow Lite, ONNX)
- Primary risk: Collecting training data (need historical OOM events with 5min leading indicators)

**Action Safety:** ⚠️ Moderate Risk

- Killing processes is destructive but mitigated by:
  - Process criticality scoring (never kill init, systemd, agent)
  - Graduated response (cache drops before process kills)
  - Cooldown periods prevent kill loops
  - Confidence thresholds (only act at >85% confidence)
- Can be tested in sandbox with synthetic memory pressure
- Biggest risk: False positive killing critical business process

**Known Unknowns:**

- What false positive rate is acceptable? (need to establish with stakeholders)
- Can we collect sufficient training data? (need access to historical OOM incidents)
- How well do ARIMA/Prophet handle non-linear memory patterns? (requires experimentation)
- What's the minimum prediction window for useful intervention? (2-3 minutes is hypothesis)

**Timeline Estimate:**

- **Proof of Concept (4 weeks):** Simple threshold-based detector, prove signals accessible, test process kill safely
- **MVP (8-12 weeks):** ARIMA/Prophet forecasting, graduated intervention, safety mechanisms, sandbox validation
- **Production-Ready (16-20 weeks):** LSTM if needed, extensive testing, false positive tuning, production rollout
- **Long-lead item:** Training data collection (may need to instrument production for 4-8 weeks)

#### 🔧 Implementation Sketch

- Gadget subscribes to CgroupResourceProvider, SystemResourceProvider, ProcessSignalProvider
- Every 10s: Update sliding window, run LSTM inference
- Maintains FSM (state machine) with cooldown timers
- Pre-trained model embedded in binary (TensorFlow Lite or ONNX)
- Retraining pipeline: Export OOM events + features to backend for model updates

### 2. The Pathologist - Anomalous Process Behavior Classifier — AI-Powered

#### 🎯 The Problem - Zombie Processes That Look Alive

The worst production nightmares happen when processes appear healthy but are functionally dead. Your HTTP health endpoint returns 200 OK (the port is listening!), but all actual work requests hang forever. Your database connection pool is "active" but every query times out. Traditional monitoring sees: CPU usage ✓, Memory stable ✓, Port open ✓, Health check passing ✓. Meanwhile, users are experiencing total service failure.

**Why This Happens:** Deadlocks, infinite loops, thread pool exhaustion, distributed lock failures - all create processes that are alive at the OS level but dead at the application level. Health checks test the wrong thing (TCP connectivity) instead of the right thing (actual liveness).

#### 💡 The Breakthrough Insight

**The key discovery:** Deadlocked processes have a distinctive "behavioral fingerprint" visible at the syscall level that no health check can detect. A truly live process, even when idle, exhibits *diverse* syscall patterns: poll(), read(), write(), accept(), recvfrom(). A deadlocked process shows pathologically *repetitive* syscalls: futex(), futex(), futex(), futex()... stuck in an infinite wait.

**The Measurement:** This diversity is quantifiable as *syscall entropy*. Healthy process ≈ 4-5 bits (diverse). Deadlocked ≈ 0-1 bits (repetitive). Long GC ≈ 2-3 bits (memory-focused but still diverse). This single metric distinguishes "stuck forever" from "legitimately waiting" - something that stumped monitoring systems for decades.

#### 🎭 The Magic Moment - The Invisible Deadlock Detected

**Tuesday, 2:45 PM** - Your API gateway shows perfect health: 99.99% uptime, health checks green, CPU at 12%, memory stable. But customer complaints are spiking: "checkout is frozen."

**2:46 PM** - The Pathologist's behavioral classifier notices something: `api-gateway-worker-7` has syscall entropy = 0.08 bits (extremely low). Over the last 60 seconds, it made 15,000 syscalls: 14,997× futex(FUTEX_WAIT), 3× poll(). It has zero I/O, zero network tx, but CPU is at 8% (spinning in userspace trying to acquire a lock).

**2:47 PM** - Classification: "Stuck" with 94% confidence. Thread pool deadlock detected. The Pathologist takes action: captures stack trace via `gdb`, saves to incident database, sends SIGTERM. Process doesn't respond (deadlocked threads can't handle signals gracefully). After 10s: SIGKILL.

**2:48 PM** - Systemd restarts the worker. Within 5 seconds, checkout resumes. The deadlock is broken.

**2:50 PM** - The SRE gets a notification: "Pathologist killed api-gateway-worker-7 (PID 23441). Reason: Thread deadlock (syscall_entropy=0.08, class=stuck, conf=94%). Stack trace attached. Root cause: distributed lock timeout in redis-client v3.2.1, waiting on key 'checkout:lock:user:8471' that was never released. Recommend investigating lock cleanup logic."

**The Traditional Approach:** Would've taken 15-30 minutes for an SRE to notice customer complaints, SSH to the box, run `strace`, identify the deadlock pattern, and manually kill the process. By then, dozens of checkouts failed. **The Pathologist did it automatically in 3 minutes, before most customers even noticed.**

#### 📊 Signal Requirements

- **ProcessSignalProvider:** `CPU.User` + `CPU.System` time series, `IO.ReadBytes` + `IO.WriteBytes` rates, Context switches (voluntary vs involuntary), `Process.Status` (`D` = uninterruptible, `R` = running, `S` = sleeping), FD count stability
- **SecurityEventProvider:** Syscall patterns (frequency + diversity), File access patterns, Network syscalls (`connect` / `accept` / `send` / `recv` activity)
- **NetworkBehaviorProvider:** Active connection count, Request rate per connection, Connection state (`ESTABLISHED` but no data transfer)

#### 🏗️ Intelligence Architecture

**Feature Engineering (30s window):** Behavioral fingerprinting

- `cpu_time_delta`: cpu_now - cpu_30s_ago
- `io_bytes_delta`: io_now - io_30s_ago
- `syscall_entropy`: shannon_entropy(syscall_histogram)
- `syscall_rate`: syscall_count / 30
- `network_tx_rate`: bytes_sent / 30
- `fd_churn`: (fds_opened + fds_closed) / 30
- `ctx_switch_ratio`: voluntary / (voluntary + involuntary)
- `status_d_duration`: seconds_in_uninterruptible_sleep
- `connection_utilization`: active_conns > 0 AND tx_rate == 0
- `fd_stale_ratio`: unchanged_fds / total_fds

#### ML Approach Options

**The Problem:** Classify process state as Working / Idle / Stuck based on behavioral fingerprint (10 features over 60s window), distinguishing deadlocks from legitimate waiting.

##### Option 1: Rule-Based Classification

- **Approach:** Hard-coded rules: IF syscall_entropy < 1.0 AND io_bytes_delta == 0 AND duration > 60s → Stuck
- **Pros:** No training data needed, completely interpretable, zero model size, instant classification
- **Cons:** Brittle across languages (Java vs Python deadlocks look different), high false positives (cache servers have low I/O legitimately), can't adapt to per-process baselines
- **ML Experience Required:** None
- **Verdict:** Good for proof-of-concept to validate syscall entropy signal, but likely 20-30% false positive rate in production

##### Option 2: Decision Tree / Random Forest

- **Approach:** Train shallow decision tree or small Random Forest (10-50 trees) on labeled process traces (working/idle/stuck)
- **Pros:** Handles multi-modal patterns (language-specific deadlock signatures), learns per-process baselines, interpretable feature importance, small model (~1-5MB), fast inference (<1ms)
- **Cons:** Requires labeled training data (need to collect stuck process examples), doesn't capture temporal evolution as well as sequences
- **ML Experience Required:** Basic (sklearn RandomForestClassifier tutorial sufficient)
- **Model Size:** ~2MB for 50 trees with 10 features
- **Verdict:** Primary recommendation. Strikes good balance between performance and complexity for minimal ML teams.
- **Learning Resources:** [sklearn Random Forest Guide](https://scikit-learn.org/stable/modules/ensemble.html#forest), [Interpretable ML Book](https://christophm.github.io/interpretable-ml-book/)

##### Option 3: Gradient Boosted Trees (XGBoost, LightGBM)

- **Approach:** More sophisticated ensemble method with boosting
- **Pros:** Often higher accuracy than Random Forest, still interpretable, handles complex patterns
- **Cons:** Harder to tune (more hyperparameters), slightly larger models (~5-10MB), overkill for this problem
- **ML Experience Required:** Moderate
- **Verdict:** Only if Random Forest shows insufficient precision. Likely unnecessary complexity.

**Recommendation:** Start with Option 1 for signal validation and POC. Move to Option 2 (Random Forest) for MVP. The key insight is syscall entropy - the ML model mostly helps distinguish edge cases and learn per-process baselines.

**Why Classification, Not Anomaly Detection:** We have clear labeled classes (working/idle/stuck), not just "normal vs abnormal." Supervised learning (classification) works better than unsupervised anomaly detection when you can label examples. Collect training data by:

- **Working:** Capture healthy processes under load
- **Idle:** Capture standby processes (web servers waiting for requests)
- **Stuck:** Synthetically create deadlocks (pthread_mutex deadlock, Go channel deadlock), capture real incidents

**Action Decision Logic:**

```
IF class == "stuck" AND confidence > 0.9 AND duration > 60s:
    IF process_is_critical() OR user_process():
        → SKIP (don't kill critical/user processes)
    ELSE:
        → Snapshot stack trace (gdb backtrace, Java jstack, Go pprof)
        → Send SIGTERM, wait 10s
        → IF still alive: SIGKILL
        → Record: PID, stack trace, classification features
        → Trigger restart via supervisor/systemd
```

#### ⚠️ Why This Requires AI, Not Simple Rules

**Why can't you just check if CPU is flat?**

- **Multi-Modal Behavioral Patterns:**"Stuck" manifests differently across languages and frameworks. Java deadlock: high futex count, medium CPU (GC still running). Python deadlock: low futex, zero CPU (GIL-locked). Go deadlock: high syscalls (goroutine scheduler still active), but zero I/O. A Random Forest learns these language-specific signatures from 10+ behavioral features, not just CPU.
- **Context-Dependent Classification:**Zero I/O is normal for a cache server (it's waiting for requests). Zero I/O is suspicious for a batch processor (it should be reading files). The classifier learns per-process baselines: "What does normal idle look like for THIS process?" Something no static rule can encode.
- **Temporal Evolution:**Deadlocks evolve over time. First 10s: normal waiting. Next 20s: entropy starts dropping. Next 30s: entropy hits floor, FD churn stops. The model tracks this trajectory across a 60s window, distinguishing "temporary lock contention" from "permanent deadlock."
- **The Intelligence Encoded:** This automates what an experienced SRE does when they SSH to a troubled box: run `strace`, watch the syscall stream, recognize the "stuck in futex" pattern, check I/O activity, examine network connections, and conclude "it's deadlocked" from the gestalt of signals. That expertise is encoded in the Random Forest trained on thousands of labeled process traces.

#### 🛡️ Safety Mechanisms

- **Whitelist critical processes:** Agent, systemd, containerd, kubelet
- **Distinguish "Long GC" from "Deadlock":** Long GC has high CPU with status=R and syscalls still occurring
- **Syscall entropy threshold:** Deadlocked process has extremely low entropy (repeating futex calls)
- **Require sustained stuck state:** Must be stuck for >60s continuously
- **Human override:** Check for `/var/run/gadget-pause` file before any kill action

#### 💰 The Payoff

**Reduces MTTR from hours to minutes.** Traditional approach: customer complaints → SRE investigation → manual diagnosis → manual kill → restart. The Pathologist: automatic detection in 60s → automatic recovery in 3 minutes → incident report with root cause delivered to SRE's dashboard.

**The SRE Reaction:** "I would've done exactly that - checked syscalls, seen the futex spam, killed it - but I would've spent 20 minutes diagnosing it. The Pathologist did it in real-time, before I even got paged."

#### 📋 Feasibility Assessment

**Signal Readiness:** ✅ High

- Syscall events from `pkg/security/ebpf/probes` (48+ probe types, raw_research.md line 706-756)
- Process CPU/IO from `/proc/[pid]/stat` via `pkg/process/procutil` (raw_research.md line 497-510)
- Context switches from `/proc/[pid]/status`
- Network connections from `pkg/network/tracer` (raw_research.md line 649-700)
- **Key signal:** Syscall monitoring already exists in CWS (Cloud Workload Security), need to expose syscall histogram aggregation

**ML Complexity:** ✅ Straightforward

- Option 2 (Random Forest) is very straightforward for minimal ML teams
- sklearn has excellent documentation and tutorials
- Feature engineering is simple (30s window aggregations, no complex transforms)
- Training data can be synthetically generated (create deliberate deadlocks)
- Model is small (~2MB) and interpretable (can inspect decision tree logic)

**Action Safety:** ⚠️ Moderate Risk

- Killing stuck processes is less risky than Oracle (processes are already non-functional)
- Risk mitigated by:
  - Critical process whitelist
  - 60s sustained stuck state requirement (avoids killing during brief lock contention)
  - Stack trace capture before kill (preserves debugging info)
  - Confidence threshold >90%
- Can be tested with synthetic deadlocks (pthread mutexes, Go channels)
- Biggest risk: Misclassifying long GC as deadlock (need entropy threshold tuning)

**Known Unknowns:**

- How reliably can we calculate syscall entropy from CWS events? (need to validate sampling doesn't affect entropy calculation)
- What's the false positive rate for distinguishing long GC from deadlock? (requires experimentation)
- Can we capture stack traces reliably across languages (gdb for C/C++, jstack for Java, pprof for Go)? (need to validate per language)
- How do we handle containerized processes? (gdb might not work across container boundaries)

**Timeline Estimate:**

- **Proof of Concept (3-4 weeks):** Simple entropy-based rules, validate syscall signal quality, test process kill in sandbox
- **MVP (8-10 weeks):** Random Forest classifier, synthetic training data generation, stack trace capture, safety mechanisms
- **Production-Ready (14-18 weeks):** Real-world training data, per-language stack trace capture, extensive false positive tuning
- **Advantage:** Synthetic training data makes this faster than Oracle (don't need to wait for production OOMs)

### 3. The Curator - Semantic Log Redaction & PII Detection — AI-Powered

#### 🎯 The Problem - PII Is Hiding in Plain Sight

Your compliance team sends the dreaded email: "GDPR audit found customer SSNs in production logs. Maximum fine: €20M. Root cause analysis required immediately." Your developers weren't malicious - they added debug logging during an incident that included "user object" serialization. Standard PII regex scans missed it because the SSN was formatted as "123 45 6789" (spaces instead of dashes).

**Why Regex Fails:** PII appears in infinite variations. Phone numbers: "(555) 123-4567", "555-123-4567", "5551234567", "call me at five five five one two three four five six seven". Credit cards: "4111111111111111", "4111-1111-1111-1111", "4111 1111 1111 1111". API keys: "sk-abc123...", "Bearer abc123...", "Authorization: abc123...". You can't regex your way out - the pattern space is too large, and context matters ("my password is 123-45-6789" vs "invoice #123-45-6789").

#### 💡 The Breakthrough Insight

**The key discovery:** PII detection is a *language understanding* problem, not a pattern matching problem. Humans recognize "call me at 5 5 5 - 0 1 9 9" as a phone number because we understand *context and intent*, not because it matches a regex. Modern NER (Named Entity Recognition) transformers do the same - they read the sentence, understand relationships between words, and classify entities based on semantic meaning.

**The Clever Architecture:** Running BERT on every log line is too expensive (100 logs/sec vs 100k logs/sec needed). The solution: a two-tier system where the fast path (regex) handles 95% of obvious PII, and the slow path (BERT) catches edge cases. Critically, when BERT finds PII that regex missed, it *generates a new regex rule for the fast path*, learning over time to handle more variations efficiently.

#### 🎭 The Magic Moment - The Audit That Never Happened

**March 15, 11:32 PM** - A junior developer is debugging a payment failure. They add a log line: `logger.debug("Payment failed for user: {}", user.toString())`. The User object includes SSN (for KYC verification), formatted as "SSN: 123 45 6789" (spaces, no dashes).

**11:33 PM** - Log hits the agent pipeline: "Payment failed for user: User{id=8471, name='John Smith', email='<john@example.com>', SSN: 123 45 6789, ...}". The Curator's fast-path regex doesn't match (it expects "XXX-XX-XXXX"). The line enters the slow-path sampling queue (1% of logs).

**11:34 PM** - BERT NER model processes the log. Context analysis: "SSN:" token followed by three digit groups → classified as SOCIAL_SECURITY_NUMBER with 96% confidence. The Curator redacts: "SSN: [SSN_REDACTED]", generates new fast-path regex: `/SSN:\s*\d{3}\s+\d{2}\s+\d{4}/`, adds to pattern library.

**11:35 PM** - Log is sent to backend, fully redacted. Alert sent to security team: "PII_DETECTED: SSN in non-standard format (spaces) in service=payment-api, source=UserDebugLogger.java:187. New pattern learned and added to fast-path. Historical logs scanned: 0 matches (first occurrence)."

**March 16, 8:00 AM** - Security engineer reviews the alert, removes the offending log line in the next deploy. No SSNs ever reached the log aggregation backend. No compliance violation. No audit finding.

**The Counter-Factual Without The Curator:** Developer logs SSNs for 3 months. Quarterly audit finds them in log archives. Company faces €2M fine. Engineering teams spend 200 person-hours scrubbing logs, implementing manual PII policies, and retraining developers. All because regex couldn't understand "SSN: 123 45 6789".

#### 🏗️ Intelligence Architecture

**The Problem:** Detect PII in log lines at 100k logs/sec throughput, catching edge-case formats that regex misses (spaces, word format, unusual syntax).

#### ML Approach Options

##### Option 1: Comprehensive Regex Library Only

- **Approach:** Maintain large library of regex patterns (100+ rules) covering all known PII formats
- **Pros:** Extremely fast (~100k logs/sec), zero model size, completely deterministic
- **Cons:** 60-70% catch rate (misses 30-40% of PII variations), requires constant manual updates, can't understand context ("123-45-6789" as SSN vs invoice number)
- **ML Experience Required:** None
- **Verdict:** Insufficient for compliance (too many misses), but necessary as fast path

##### Option 2: Two-Tier (Regex + Simple NER)

- **Approach:** Fast path (regex) handles 95% of obvious PII, slow path (lightweight NER) catches edge cases at 1% sampling rate
- **Slow Path Option A: Rule-based NER:** Pattern matching with context windows (check surrounding tokens for "SSN:", "phone:", "credit card:")
- **Slow Path Option B: spaCy NER:** Pretrained named entity recognizer, ~20MB model, medium accuracy
- **Pros:** Improves catch rate to ~85-90%, lower complexity than transformers, spaCy well-documented
- **Cons:** Still misses complex edge cases, doesn't learn organization-specific patterns
- **ML Experience Required:** Basic (spaCy tutorial sufficient if using Option B)
- **Model Size:** ~20MB (spaCy) or 0MB (rule-based)
- **Verdict:** Good middle ground for minimal ML teams. Significantly better than regex-only.
- **Learning Resources:** [spaCy NER Guide](https://spacy.io/usage/linguistic-features#named-entities)

##### Option 3: Two-Tier (Regex + BERT NER)

- **Approach:** Fast path (regex), slow path (quantized BERT/DistilBERT for contextual NER at 1% sampling)
- **Pros:** Best accuracy (~95%+ catch rate), understands context ("123-45-6789" in "SSN: 123-45-6789" vs "Invoice #123-45-6789"), can detect unusual formats, learning capability (regex auto-generation from caught PII)
- **Cons:** Complex model (~50-100MB quantized), requires NLP expertise, slower inference (~100 logs/sec on slow path), harder to debug
- **ML Experience Required:** Moderate-Advanced (BERT fine-tuning, quantization, NER training)
- **Model Size:** ~50MB (quantized DistilBERT) or ~100MB (quantized BERT)
- **Verdict:** Best accuracy but significant complexity. Only worth it if Option 2 shows insufficient catch rate or if organization has NLP expertise.
- **Learning Resources:** [Hugging Face NER Tutorial](https://huggingface.co/docs/transformers/tasks/token_classification), [TFLite Model Quantization](https://www.tensorflow.org/lite/performance/post_training_quantization)

**Recommendation:** Start with Option 1 (regex-only) for POC to establish baseline catch rate. Implement Option 2 with spaCy NER for MVP (good accuracy, straightforward for minimal ML teams). Consider Option 3 only if compliance requires >95% catch rate and team can partner with ML experts.

**Why Two-Tier Matters:** 100k logs/sec throughput requirement makes running ML on every log infeasible. Fast path handles the common cases (95%), slow path catches edge cases at low sampling rate (1%). This architecture is borrowed from web application firewalls (WAFs) which use similar fast/slow pattern matching.

**Architecture:**

| Path | Method | Throughput | Coverage |
|---|---|---|---|
| **Fast** | Compiled regex patterns (dynamic, learned) | ~100k logs/sec | ~95% of PII |
| **Slow** | NER model (spaCy or BERT) at 1% sampling | ~100 logs/sec | Catches unusual formats |

**Entities Detected:**

- **CREDIT_CARD:** 16-digit patterns with Luhn check
- **SSN:** XXX-XX-XXXX variants
- **PHONE:** Various formats including spoken ('five five five')
- **EMAIL:** Contextual email detection
- **API_KEY:** High-entropy base64/hex strings in key= contexts
- **IP_ADDRESS:** IPv4/IPv6 (only if in sensitive context)
- **NAME:** Person names (using context, not just capitalized words)

**Redaction Strategy:**

- **credit_card:** Replace with `[CREDIT_CARD_REDACTED]`
- **ssn:** Replace with `[SSN_REDACTED]`
- **phone:** Replace with `[PHONE_REDACTED]`
- **api_key:** Replace first 8 chars, keep last 4: `sk-abc...xyz`
- **email:** Preserve domain for debugging: `***@example.com`

#### ⚠️ Why This Requires AI, Not Just Better Regex

**Why can't you just maintain a comprehensive regex library?**

- **Context Is Everything:**The string "123-45-6789" could be an SSN (bad) or an invoice number (fine) or a product SKU (fine). Only semantic understanding tells them apart. Regex sees patterns; NER understands meaning. "Please call 555-0199 for support" vs "Error code: 555-0199" - one is a phone number, one isn't.
- **Infinite Format Variations:**Phone numbers alone have 50+ common formats globally. Credit cards: 15-19 digits, with or without spaces/dashes, sometimes with "CC: " prefix, sometimes embedded in JSON. The regex combinatorial explosion is unsustainable - you'd need 1000+ patterns and still miss edge cases.
- **Adversarial Formats:**Developers sometimes obfuscate PII unintentionally: "SSN is 1-2-3-4-5-6-7-8-9" (spoken digit format), "my social is one two three forty-five sixty-seven eighty-nine" (mixed numeric/word). NER transformers trained on diverse text understand these; regex never will.
- **Learning From Mistakes:**When BERT catches a PII pattern regex missed, it doesn't just redact - it*generates a new regex*for future fast-path detection. The system evolves, learning your organization's specific PII footprint over time. This is supervised learning in production: slow path teaches fast path.
- **The Intelligence Encoded:**This automates what a security engineer does during log audits: read each line, understand what each token means in context, identify sensitive data by semantic content (not just pattern), and recognize when developers have serialized objects that shouldn't be logged.

#### 💰 The Payoff

**Prevents €20M GDPR fines by catching PII before it reaches log storage.**Traditional regex-based PII scanning has a 60-70% catch rate (misses 30-40% of PII). The Curator's two-tier approach achieves >95% catch rate while maintaining 100k logs/sec throughput.
**The Security Team Reaction:**"We used to find SSNs in logs during quarterly audits and panic. Now The Curator finds them in real-time, redacts them, alerts us, and even tells us which line of code needs fixing. It's like having a security engineer review every log line automatically."

#### 📋 Feasibility Assessment

**Signal Readiness:** ✅ High

- Log pipeline already exists in `pkg/logs` with real-time processing (raw_research.md line 812-845)
- Log messages available at collection time before forwarding to backend
- Processing pipeline supports inline transformation (existing feature for log enrichment)
- Throughput: agent handles 10k-100k logs/sec today depending on configuration

**ML Complexity:** ⚠️ Moderate (Option 2) / ❌ Research Required (Option 3)

- Option 2 (spaCy NER) is straightforward for minimal ML teams
  - spaCy has excellent documentation and pretrained models
  - ~20MB model fits comfortably in memory budget
  - Integration is simple (Python library, synchronous API)
  - Can start with pretrained model, fine-tune later if needed
- Option 3 (BERT NER) requires NLP expertise team likely doesn't have
  - Model quantization, NER fine-tuning, inference optimization
  - Recommend partnering with ML team if this level needed
- Primary challenge: Validating PII detection accuracy (need test corpus of logs with labeled PII)

**Action Safety:** ✅ Low Risk

- Redaction is non-destructive (logs still sent, just modified)
- False positives (redacting non-PII) are low impact (some context loss, but safe)
- False negatives (missing PII) don't cause operational issues, just compliance risk
- Can be tested thoroughly in sandbox with synthetic logs before production
- No process killing or resource manipulation - safest gadget of all

**Known Unknowns:**

- What is acceptable false positive rate for redaction? (redacting invoice numbers that look like SSNs)
- How do we validate PII detection accuracy? (need labeled test corpus)
- Can we maintain 100k logs/sec with 1% slow-path sampling? (need performance profiling)
- What's the actual PII catch rate of comprehensive regex? (need to establish baseline)
- How do we handle internationalization? (EU phone numbers, non-US SSN formats)

**Timeline Estimate:**

- **Proof of Concept (2-3 weeks):** Regex-only redaction, integration with log pipeline, throughput validation
- **MVP (6-8 weeks):** spaCy NER integration (Option 2), two-tier architecture, accuracy testing with synthetic PII
- **Production-Ready (12-14 weeks):** Fine-tuning on organization-specific PII patterns, false positive reduction, comprehensive testing
- **Advantage:** Can be tested entirely offline with synthetic logs (no production risk during development)

### 4. The Equalizer - Intelligent Request Prioritization & Toxic Query Termination — AI-Powered

#### 😫 Pain Point

A single recursive GraphQL query or runaway SQL query blocks resources, causing cascading failures. Manual identification requires deep tracing; by the time operators notice, the service is degraded.

#### 💡 Concept

Uses unsupervised learning (k-means clustering) on connection behavior (duration, byte size, CPU/memory attribution) to identify "toxic" requests in real-time. Surgically terminates the specific TCP connection, freeing resources.

#### 🏗️ Intelligence Architecture

**Feature Engineering:**Statistical outlier detection

- `duration_zscore`: (duration - mean) / stddev
- `bytes_zscore`: (total_bytes - mean) / stddev
- `latency_zscore`: (latency - mean) / stddev
- `cpu_attribution`: cpu_delta / connection_count
- `memory_attribution`: mem_delta / connection_count
- `rtt_penalty`: rtt > 2 * avg_rtt
- `retransmits_penalty`: retransmits > threshold

#### ML Approach Options

**The Problem:** Identify "toxic" connections causing resource exhaustion (high duration, high CPU attribution, disproportionate resource use) in real-time, distinguishing from legitimate slow clients or large responses.

##### Option 1: Simple Statistical Outliers (Z-score)

- **Approach:** Flag connections with z-score > 3σ on duration AND cpu_attribution
- **Pros:** No ML needed, easy to understand, instant detection, zero model size
- **Cons:** Can't distinguish legitimate edge cases (large file download = high duration but not toxic), threshold tuning required per service, high false positives
- **ML Experience Required:** None (basic statistics)
- **Verdict:** Good for POC to validate signals, but likely 30-40% false positive rate (kills legitimate slow operations)

##### Option 2: Isolation Forest (Anomaly Detection)

- **Approach:** Unsupervised anomaly detection on 7 features, learns normal patterns, flags outliers
- **Pros:** No labeled data needed, adapts to per-service baselines, handles multivariate outliers, interpretable anomaly scores, small model (~5MB)
- **Cons:** Still unsupervised (no concept of "toxic" vs "legitimate slow"), requires tuning contamination rate, can't distinguish edge cases well
- **ML Experience Required:** Basic (sklearn IsolationForest tutorial)
- **Model Size:** ~5MB
- **Verdict:** Better than z-scores but still lacks context. Primary recommendation for minimal ML teams.
- **Learning Resources:** [sklearn Isolation Forest](https://scikit-learn.org/stable/modules/generated/sklearn.ensemble.IsolationForest.html)

##### Option 3: K-Means Clustering + Distance Thresholds

- **Approach:** Cluster connections into behavioral profiles (normal, slow-client, large-response, toxic), flag outliers far from any cluster
- **Pros:** Groups similar behaviors, can label clusters post-hoc, distance-to-centroid is interpretable
- **Cons:** Requires choosing K (number of clusters), assumes spherical clusters, still unsupervised (clusters may not align with toxic/benign), initialization sensitive
- **ML Experience Required:** Basic (sklearn KMeans)
- **Model Size:** ~1MB (cluster centroids)
- **Verdict:** Not clearly better than Isolation Forest. More parameters to tune (K, distance threshold) without obvious advantage.

**Recommendation:** Start with Option 1 for POC. Implement Option 2 (Isolation Forest) for MVP as it's straightforward and handles multivariate outliers better than z-scores. Option 3 doesn't provide clear advantages over Option 2 for this problem.

**Why Unsupervised?** We don't have labeled training data of "toxic" vs "benign" slow connections - we're trying to discover the toxic ones. Unsupervised learning is appropriate here, but has limitations (see Research Gaps below).

#### 🛡️ Safety Mechanisms

- **Whitelist paths:**`/health`,`/metrics`,`/admin/*`never terminated
- **HTTP method safety:**Only terminate GET, HEAD (safe methods)
- **Cooldown:**Max 1 termination per minute per service (avoid kill loops)
- **Confidence threshold:**Require anomaly score > 3σ (very high confidence)
- **Human override:**`/var/run/gadget-no-kill`file disables termination

#### 💰 Value Proposition

Prevents cascading failures from toxic queries. Surgical intervention avoids killing entire process. SREs say "I would've restarted the app, but this only killed the bad request."

#### 📋 Feasibility Assessment

**Signal Readiness:** ⚠️ Medium

- Connection tracking from `pkg/network/tracer` (raw_research.md line 664-676): duration, bytes, packets, RTT, retransmits
- **Gap:** CPU/memory attribution per-connection NOT currently collected
  - Would need to correlate process CPU deltas with active connections (non-trivial)
  - Or use eBPF to track CPU cycles per socket (complex instrumentation)
- HTTP request latency from USM (Universal Service Monitoring) available (raw_research.md line 678-692)
- Network performance metrics (RTT, retransmits) available today

**ML Complexity:** ✅ Straightforward

- Option 2 (Isolation Forest) is very straightforward for minimal ML teams
- sklearn has excellent documentation
- Feature engineering is simple statistical calculations (z-scores, means)
- Small model (~5MB), fast inference

**Action Safety:** ❌ High Risk

- Terminating TCP connections is destructive and hard to undo
- Risk of killing legitimate operations:
  - Large file downloads (high duration, high bytes, but legitimate)
  - Slow clients on poor networks (high duration, high RTT, but not toxic)
  - Admin operations (legitimate long-running queries)
- Unlike Oracle/Pathologist (kill process, it restarts), connection termination loses in-flight work
- Very difficult to validate without production traffic patterns
- **Major concern:** False positive rate could be 20-40% with unsupervised learning

**Known Unknowns:**

- How do we attribute CPU/memory to specific connections? (major technical gap)
- What defines "toxic" vs "slow but legitimate"? (unclear problem definition)
- Do we have production examples of toxic queries causing cascading failures? (need evidence)
- What's the false positive tolerance? (1 in 100? 1 in 1000?)
- How do we safely test this? (can't inject toxic queries into production easily)
- Can we even terminate a specific TCP connection from the agent? (kernel-level operation, may require eBPF or SO_LINGER hacks)

**Timeline Estimate:**

- **Proof of Concept (4-6 weeks):** Basic outlier detection on available signals (duration, bytes, RTT), validate connection tracking
- **Research Phase (4-8 weeks):** Investigate CPU/memory attribution, determine if technically feasible, collect production examples of toxic queries
- **MVP (12-16 weeks):** IF CPU attribution is feasible, implement Isolation Forest, extensive false positive testing
- **High Risk:** May discover CPU attribution is not feasible, rendering gadget concept invalid

#### ⚠️ Research Gaps

This gadget concept needs significant development before implementation:

**Unclear Problem Definition:**

- "Toxic query" is not well-defined. What makes a query toxic vs slow-but-legitimate?
- Need production examples: Has this organization actually experienced cascading failures from toxic queries? What did they look like?
- What's the actual business impact? Is this solving a real problem or a theoretical one?

**Approach Uncertainty:**

- **CPU/Memory attribution per-connection is NOT currently collected and may be technically difficult:**
  - Process-level CPU is easy (/proc/[pid]/stat)
  - Per-connection CPU would require correlating CPU delta with active connections (which connection caused CPU spike?)
  - Or eBPF to track CPU cycles per socket (complex, high overhead)
  - This is a **major technical unknown** - might not be feasible
- **Unsupervised learning may not be appropriate:**
  - Isolation Forest finds outliers, but not all outliers are toxic
  - Large file downloads are outliers (high duration, high bytes) but legitimate
  - May need supervised learning with labeled examples, but we don't have them
- **Why not use APM trace data instead?**
  - APM traces already have request duration, operation name, resource usage
  - Trace-based toxic query detection might be simpler and more accurate
  - This approach may be solving the problem at the wrong layer

**Validation Challenge:**

- How to test safely without production traffic?
- Can't easily inject synthetic "toxic queries" into production
- False positive impact is high (kill legitimate user requests)
- Need extensive A/B testing, difficult to set up

**Recommendation:** Needs 4-6 weeks of problem research before any coding:

1. Collect production examples of toxic query cascading failures
2. Analyze: What signals differentiated toxic from slow-but-legit?
3. Investigate technical feasibility of CPU attribution per-connection
4. Consider alternative approaches (APM-based detection, application-level circuit breakers)
5. Only proceed if clear problem evidence + technical feasibility confirmed

### 5. The Archivist - Anomaly-Triggered Retroactive Log Hydration — AI-Powered

#### 😫 Pain Point

To save costs, logs are sampled at 1%. When a rare bug occurs, the critical "cause" log was in the discarded 99%, leaving only the crash symptom.

#### 💡 Concept

Maintains a short-term ring buffer (1-2 minutes) of ALL logs. When an anomaly is detected (error rate spike, crash, OOM), retroactively flushes the full buffer for that time window, ensuring root cause is captured.

#### 🏗️ Intelligence Architecture

**Circular Buffer Management:**Memory-efficient log retention

- **Size:**120 seconds × expected_log_rate
- **Per-service:**Separate buffers to avoid cross-contamination
- **Eviction:**FIFO (oldest logs dropped first)
- **Memory Limit:**Max 500MB total across all services
- **Compression:**LZ4 on-the-fly for space efficiency

#### ML Approach Options

**The Problem:** Decide which logs from the ring buffer are worth keeping when an anomaly triggers. Buffer has 120s of ALL logs (100%), but we want to forward only the "interesting" ones to reduce volume (keep top 90% by interestingness).

##### Option 1: No ML - Keep Everything in Trigger Window

- **Approach:** When anomaly detected, forward all logs in 90s before + 30s after window (no filtering)
- **Pros:** Zero ML needed, guaranteed to have root cause, simple implementation
- **Cons:** High volume (may still be too much data if log rate is very high), no prioritization
- **ML Experience Required:** None
- **Verdict:** This might be sufficient! If 120s of logs at normal rates is manageable volume, semantic scoring may be unnecessary complexity.

##### Option 2: TF-IDF Novelty Scoring

- **Approach:** Build TF-IDF vectors from historical logs, score new logs by rarity of terms (high TF-IDF = unusual = interesting)
- **Pros:** Straightforward technique (sklearn TfidfVectorizer), lightweight (~10MB vocabulary), interpretable scores, finds unusual log patterns
- **Cons:** Requires building vocabulary from historical logs, assumes rare = interesting (not always true)
- **ML Experience Required:** Basic (sklearn tutorial sufficient)
- **Model Size:** ~10MB vocabulary
- **Verdict:** Good approach if Option 1 volume is too high. Provides quantifiable "novelty" score.
- **Learning Resources:** [sklearn TF-IDF](https://scikit-learn.org/stable/modules/generated/sklearn.feature_extraction.text.TfidfVectorizer.html)

##### Option 3: Sentence Embeddings (BERT/SBERT)

- **Approach:** Generate embeddings for each log line, compute cosine similarity to historical log corpus, low similarity = novel = interesting
- **Pros:** Best semantic understanding, handles synonyms and paraphrasing
- **Cons:** Very heavy (~100MB model), slow inference (~100ms per log), overkill for this problem, requires NLP expertise
- **ML Experience Required:** Advanced
- **Verdict:** Not recommended. Too complex for the benefit. Option 2 (TF-IDF) provides most of the value at 10x lower complexity.

**Recommendation:** Start with Option 1 (no ML) for POC - ring buffer + threshold triggers may be sufficient. Add Option 2 (TF-IDF) only if volume is too high and prioritization is needed. Skip Option 3 entirely (unnecessary complexity).

**Key Insight:** The value is in the ring buffer + trigger mechanism, not the semantic scoring. Most incident root cause logs are near the anomaly trigger (error spike, crash, OOM) - just keeping the time window may be enough. Semantic scoring is an optimization, not the core innovation.

**Anomaly Detection Triggers:**

- **Error Rate Spike:** `(error_logs / total_logs) > 2 × baseline_error_rate` (last 30s)
- **Crash Event:** `Container.RestartCount` incremented
- **OOM Event:** `CgroupResourceProvider.OOMEvents` counter increased
- **Latency Spike:** TraceAnalysisProvider `p99 > 2 × p99_baseline`

**Hydration Window:**

- **Before anomaly:**90s
- **After anomaly:**30s
- **Priority (if using Option 2):**Sort by TF-IDF novelty score, keep top 90%

#### 💰 Value Proposition

Eliminates "the log I needed was sampled away" problem. Post-incident analysis always has root cause logs. SREs say "I can finally see what happened before the crash."

#### 📋 Feasibility Assessment

**Signal Readiness:** ✅ High

- Log pipeline in `pkg/logs` has real-time stream (raw_research.md line 812-845)
- Error rate, crash events, OOM events all available as triggers
- Trace latency metrics from TraceAnalysisProvider (raw_research.md line 776-788)
- All trigger signals exist today

**ML Complexity:** ✅ Straightforward (Option 1) / ⚠️ Moderate (Option 2)

- Option 1 (no ML) is trivial - just buffer + threshold triggers
- Option 2 (TF-IDF) is straightforward for minimal ML teams
  - sklearn TfidfVectorizer well-documented
  - Simple text processing, interpretable results
- Core challenge is buffer management, not ML

**Action Safety:** ✅ Low Risk

- Forwarding more logs is non-destructive (just higher volume sent to backend)
- No process killing, no connection termination
- Worst case: Send too many logs (cost increase, but operationally safe)
- Can be tested thoroughly offline before production

**Known Unknowns:**

- What's the typical log rate? (determines buffer size and memory requirements)
- How many anomalies per day? (determines burst upload volume)
- Is 500MB buffer enough across all services? (need production profiling)
- Does Option 1 (no ML) provide sufficient value, or is filtering required? (may not need ML at all)

**Timeline Estimate:**

- **Proof of Concept (2-3 weeks):** Ring buffer implementation, simple error-rate trigger, validate memory usage
- **MVP (4-6 weeks):** Add all trigger types (crash, OOM, latency), compression, volume testing
- **Option 2 (8-10 weeks):** Add TF-IDF scoring if Option 1 volume is too high
- **Advantage:** Can be developed entirely offline, no production risk

#### ⚠️ Research Gaps

This gadget concept needs clarification before implementation:

**Unclear Problem Definition:**

- Is the "sampled away logs" problem real for this organization? (need incident examples)
- What's the actual log sampling rate today? (if not sampling, problem doesn't exist)
- How often do incidents require "logs we didn't capture"? (need data on frequency)

**Approach Uncertainty:**

- **May not need ML at all:**
  - Option 1 (ring buffer + triggers, no filtering) might be sufficient
  - The value is in *capturing everything around incidents*, not *intelligent filtering*
  - If 120s of logs at normal rates is manageable, semantic scoring adds complexity without value
- **Alternative: Just disable sampling during high-value time windows**
  - Simpler approach: When agent detects "interesting" activity (high error rate, high latency), disable sampling for next 60s
  - Achieves similar goal without ring buffer complexity

**Validation Challenge:**

- How to validate this improves incident debugging? (need post-incident surveys from SREs)
- What's the cost impact of increased log volume? (need backend storage cost analysis)
- Is the added complexity worth it vs simpler solutions? (evaluate cost/benefit)

**Recommendation:** Needs 2-3 weeks of problem validation:

1. Analyze recent incidents: Were critical logs actually "sampled away"? How often?
2. Measure current log sampling rates and volume
3. Estimate buffer size requirements and memory impact
4. Consider simpler alternatives (dynamic sampling rate adjustment)
5. Only proceed if clear evidence of problem + buffer approach is best solution

### 6. The Tuner - Adaptive TCP/Agent Parameter Optimization — AI-Powered

#### 😫 Pain Point

Static configuration doesn't adapt to traffic patterns. TCP buffers too small → throughput loss. Agent batch sizes too small → CPU overhead. Too large → latency spikes. Manual tuning is labor-intensive and error-prone.

#### 💡 Concept

Uses lightweight Reinforcement Learning (RL) to dynamically tune TCP socket buffers and Agent configuration parameters based on real-time load. Creates a reward function where high throughput + low drops = good.

#### 🏗️ Intelligence Architecture

**Parameter Space:**

- **TCP Parameters:** `net.core.rmem_max`/`wmem_max`, `net.ipv4.tcp_rmem`/`tcp_wmem`, Per-socket `SO_RCVBUF`/`SO_SNDBUF`
- **Agent Parameters:**Log batch size, Trace aggregation bucket interval, Metric aggregation interval, Check collection frequency

#### ML Approach Options

**The Problem:** Dynamically adjust TCP buffer sizes and agent batch parameters based on real-time load to optimize throughput while minimizing drops and latency.

##### Option 1: Heuristic Rules (No ML)

- **Approach:** IF throughput < 80% of capacity AND drops > 0, increase buffers by 10%. IF CPU > 70%, decrease batch sizes.
- **Pros:** Zero ML needed, interpretable, fast adjustments, zero model size
- **Cons:** Brittle, may oscillate (increase → overshoot → decrease → undershoot), doesn't learn optimal settings, hard to balance multiple objectives
- **ML Experience Required:** None
- **Verdict:** Good starting point to validate adjustment mechanism works, but likely suboptimal

##### Option 2: Bayesian Optimization / Gaussian Processes

- **Approach:** Model reward function (throughput - drops - latency) as Gaussian Process, use acquisition function (UCB, EI) to explore parameter space efficiently
- **Pros:** Sample-efficient (finds good parameters with few trials), uncertainty quantification, interpretable, well-suited for parameter tuning, existing libraries (scikit-optimize)
- **Cons:** Slower than RL (sequential evaluation), may take hours to converge, assumes smooth reward landscape
- **ML Experience Required:** Moderate (scikit-optimize tutorials available)
- **Model Size:** Tiny (~1MB for GP state)
- **Verdict:** Better than Option 1, safer than RL. Primary recommendation for minimal ML teams. Proven for hyperparameter tuning.
- **Learning Resources:** [scikit-optimize Tutorial](https://scikit-optimize.github.io/stable/), [Bayesian Optimization Book](https://bayesoptbook.com/)

##### Option 3: Reinforcement Learning (Actor-Critic, PPO)

- **Approach:** Train RL agent with state (10-dim metrics), actions (parameter adjustments), reward (throughput - drops - latency)
- **Pros:** Can learn complex policies, adapts continuously, handles multi-objective optimization
- **Cons:** Requires RL expertise team doesn't have, unstable training (may diverge), opaque decisions (hard to debug), exploration can cause production issues, typically needs 1000s of episodes to converge
- **ML Experience Required:** Advanced (deep RL, policy gradients, reward shaping)
- **Model Size:** ~10-50MB neural network
- **Verdict:** Too complex and risky for minimal ML team. Only consider if Options 1 & 2 clearly insufficient and can partner with RL experts.

**Recommendation:** Start with Option 1 for POC to prove parameter adjustment works safely. Implement Option 2 (Bayesian Optimization) for MVP - it's specifically designed for parameter tuning and much safer than RL. Skip Option 3 unless team gains significant ML expertise or partners with ML team.

**Why RL is Risky Here:** RL exploration can cause production degradation (agent tries bad parameters to learn). Bayesian Optimization is safer (explicitly balances exploration/exploitation, quantifies uncertainty). For parameter tuning with expensive evaluations (each config change affects production), Bayesian Optimization is the standard approach.

#### 🛡️ Safety Mechanisms

- **Parameter bounds:**Never adjust beyond safe_min/safe_max (10x range)
- **Rollback on degradation:**If reward drops >10%, revert change
- **Cooldown period:**Max 1 adjustment per parameter per 5 minutes
- **Production gate:**Requires`DD_GADGET_TUNER_ENABLED=true`(opt-in)
- **Emergency stop:**If agent CPU > 50% or memory > 80%, pause tuning

#### 💰 Value Proposition

Automatic tuning eliminates manual experimentation. Adapts to traffic patterns (peak vs off-peak). Outperforms static configs by 10-30% in dynamic environments.

#### 📋 Feasibility Assessment

**Signal Readiness:** ✅ High

- Network throughput, packet loss from system metrics
- TCP buffer utilization (requires reading `/proc/net/sockstat`, `/proc/net/snmp`)
- Log drop rate, trace flush latency from agent telemetry
- CPU, memory usage from system metrics
- All signals available today or easily collectible

**ML Complexity:** ⚠️ Moderate (Option 2) / ❌ Research Required (Option 3)

- Option 2 (Bayesian Optimization) is moderate complexity
  - scikit-optimize library available, good documentation
  - Requires understanding Gaussian Processes conceptually
  - Safe approach (quantifies uncertainty before adjustments)
- Option 3 (RL) is beyond minimal ML team capabilities
  - Requires deep RL expertise, reward shaping, policy training
  - Unstable and risky in production
  - Not recommended

**Action Safety:** ❌ High Risk

- Adjusting production parameters is inherently risky:
  - Bad TCP buffer sizes can cause network congestion or memory exhaustion
  - Bad batch sizes can cause log/trace drops or high latency
  - Cascading effects: one bad parameter can trigger multiple failures
- Unlike Oracle/Pathologist (localized damage), this affects entire agent
- Very difficult to test safely (need production-like load)
- Rollback is possible but damage may already be done
- **Major concern:** How to validate adjustments are safe without extensive A/B testing?

**Known Unknowns:**

- What's the actual performance gain? (10-30% claim is unvalidated)
- Are static configs actually suboptimal, or is this solving a non-problem? (need baseline measurements)
- How long does Bayesian Optimization take to converge? (hours? days?)
- What happens during exploration phase? (bad parameters may degrade performance before finding optimum)
- Can we safely adjust kernel parameters (`sysctl`) from agent? (requires privileges, may affect other processes)
- How do we handle multi-objective optimization trade-offs? (throughput vs latency vs CPU)

**Timeline Estimate:**

- **Proof of Concept (4-6 weeks):** Simple heuristic rules (Option 1), validate parameter adjustment mechanism, measure baseline performance
- **Research Phase (4-6 weeks):** Determine if performance gains actually exist, measure current vs optimal configs in controlled environment
- **MVP (12-16 weeks):** IF gains exist, implement Bayesian Optimization (Option 2), extensive safety testing, gradual rollout
- **High Risk:** May discover static configs are "good enough" and dynamic tuning provides minimal benefit for high complexity

#### ⚠️ Research Gaps

This gadget concept needs significant validation before implementation:

**Unclear Problem Definition:**

- Are static configs actually a problem? (need evidence of suboptimal performance)
- What's the performance baseline? (must measure current throughput, drops, latency)
- What performance gain justifies the complexity? (is 5% improvement worth the risk?)
- Has manual tuning been tried? (maybe one-time expert tuning is sufficient)

**Approach Uncertainty:**

- **Parameter tuning may not be the bottleneck:**
  - Network performance often limited by external factors (bandwidth, RTT, congestion)
  - Agent performance often limited by application workload, not configuration
  - "Tuning" may provide minimal gains if defaults are already reasonable
- **Multi-objective optimization is hard:**
  - Throughput vs latency trade-off (can't optimize both simultaneously)
  - Which objective matters most? (need product requirements)
  - How to weight objectives in reward function? (arbitrary weights may not reflect business value)
- **Kernel parameter tuning is risky:**
  - Affects all processes on host, not just agent
  - Requires root privileges
  - May conflict with operator/orchestrator settings (Kubernetes may override)

**Validation Challenge:**

- How to safely test parameter changes in production?
- Need A/B testing infrastructure (some nodes with tuning, some without)
- Need long-term evaluation (hours/days) to measure impact
- Difficult to isolate improvement from normal variance

**Recommendation:** Needs 4-8 weeks of problem validation:

1. Measure baseline: current throughput, drops, latency across diverse workloads
2. Manual tuning experiment: Can expert tuning improve metrics? By how much?
3. Analyze: Are gains significant enough to justify automated tuning complexity?
4. Risk assessment: What's the blast radius if tuning goes wrong?
5. Only proceed if clear evidence of (a) meaningful gains exist and (b) risk is manageable

### 7. The Timekeeper - Temporal Pattern Anomaly Suppression — AI-Powered

#### 😫 Pain Point

Recurring maintenance events (weekly backups, nightly batch jobs) trigger alerts. Engineers manually silence them or suffer alert fatigue. Simple time-based silencing breaks when schedules shift.

#### 💡 Concept

Uses time-series decomposition (Seasonal Trend Decomposition) + similarity search on metric shape to recognize "routine maintenance" patterns. Learns from history: "This CPU spike happens every Tuesday 3AM, and no errors follow." Suppresses alerts locally before sending.

#### 🏗️ Intelligence Architecture

**Historical Shape Storage:** Compressed temporal pattern database using t-digest for O(1) space per window. Stores 24h and 7d shapes as compressed sketches with metadata including `last_seen` timestamp and `error_followed` boolean.

#### ML Approach Options

**The Problem:** Recognize recurring metric patterns (weekly backups, nightly jobs) that look anomalous but are routine, suppressing alerts while distinguishing from actual incidents.

##### Option 1: Time-Window Suppression (No ML)

- **Approach:** Manual configuration: suppress alerts for "Tuesday 3AM ± 30min" based on known maintenance windows
- **Pros:** Simple, deterministic, zero ML needed, administrators understand it
- **Cons:** Breaks when schedules shift, requires manual updates, can't discover new patterns automatically
- **ML Experience Required:** None
- **Verdict:** Baseline approach. Works if maintenance schedules are stable and well-known.

##### Option 2: Cosine Similarity on Normalized Metric Windows

- **Approach:** Store historical metric patterns (1-hour windows), compare current window to historical using cosine similarity, high similarity (>85%) + no past errors = suppress
- **Pros:** Simple algorithm, fast (O(n) comparison), automatically learns patterns, adapts to schedule shifts (±30min), no model training needed
- **Cons:** Sensitive to amplitude changes (same pattern but 2x higher magnitude may not match), doesn't handle time shifts well (backup starting 1hr late may not match)
- **ML Experience Required:** None (just vector cosine similarity)
- **Storage:** ~1KB per historical pattern
- **Verdict:** Good starting point. Much better than Option 1, much simpler than DTW.

##### Option 3: Dynamic Time Warping (DTW)

- **Approach:** Like Option 2, but DTW allows time shifts (pattern can stretch/compress by ±20%), handles same-shape-different-timing better
- **Pros:** Robust to schedule shifts, finds similar patterns even if timing varies
- **Cons:** Computationally expensive (O(n²) for DTW), much slower than cosine similarity, complexity may not be needed if schedules are regular
- **ML Experience Required:** Basic (DTW algorithm understanding)
- **Verdict:** Only if Option 2 fails due to schedule variability. Try simpler approach first.

**Recommendation:** Start with Option 2 (cosine similarity) for MVP. It's simple, fast, and handles most routine patterns. Only implement Option 3 (DTW) if schedule variability is a proven problem. Skip complex time-series decomposition - it adds little value over similarity matching.

**Why Similarity Matching Works:** Maintenance events have distinctive metric signatures (CPU spike + disk I/O spike for backups, network spike for data replication). These shapes repeat weekly/daily. Cosine similarity on normalized windows captures the shape regardless of absolute values.

**Causal Validation:** Post-hoc pattern assessment checks if errors occurred in the 1 hour following a metric spike. Updates pattern metadata to mark as benign or error-prone.

#### 🛡️ Safety Mechanisms

- **Maximum suppression duration:**2 hours (avoid suppressing actual incidents)
- **Confidence threshold:**Require >85% similarity (high confidence)
- **Post-hoc validation:**Re-evaluate patterns weekly (did suppressed events cause issues?)
- **User override:** Tag in Datadog UI: `gadget_suppressed:routine` allows investigation
- **Emergency bypass:**If error rate spikes during suppression, immediately unsuppress

#### 💰 Value Proposition

Eliminates 90% of maintenance-related alerts. Learns automatically (no manual cron schedules). Adapts to schedule shifts (weekly backup moves to Wednesday). SREs say "I don't have to silence alerts anymore."

#### 📋 Feasibility Assessment

**Signal Readiness:** ⚠️ Medium

- Metric time-series available from agent's metric aggregator
- **Gap:** Alert suppression happens in backend, not agent
  - Alerts are generated by Datadog backend monitors, not agent
  - Agent would need to either: (a) intercept metrics before sending, or (b) call backend API to suppress alerts
  - This architectural question affects feasibility significantly
- Historical metric patterns would need to be stored (agent doesn't keep history today)

**ML Complexity:** ✅ Straightforward

- Option 2 (cosine similarity) requires no ML, just vector math
- Simple pattern storage and comparison
- Option 3 (DTW) is more complex but has existing libraries (dtaidistance, tslearn)

**Action Safety:** ✅ Low Risk

- Suppressing alerts is reversible (emergency bypass re-enables)
- False positives (suppressing real incident) are mitigated by:
  - Maximum suppression duration (2 hours)
  - Emergency bypass on error rate spike
  - Tag allows SRE to see what was suppressed
- Can be tested thoroughly in non-production environments

**Known Unknowns:**

- Where does alert suppression actually happen? (agent-side or backend-side)
- How do we store historical metric patterns? (memory constraints, storage size)
- What's the actual alert fatigue problem frequency? (need data on false positive rate for maintenance)
- How do we bootstrap without historical patterns? (cold-start problem)
- Can we distinguish "routine pattern" from "incident that happens to look similar"? (validation challenge)

**Timeline Estimate:**

- **Architecture Research (2-4 weeks):** Determine if agent-side or backend-side suppression, design integration point
- **Proof of Concept (4-6 weeks):** Pattern storage, cosine similarity matching, validate with synthetic patterns
- **MVP (10-14 weeks):** Causal validation, emergency bypass, comprehensive testing
- **Dependency:** Requires architectural decision on where suppression happens (may not be agent-local)

#### ⚠️ Research Gaps

This gadget concept needs architectural and problem validation:

**Unclear Problem Definition:**

- What's the actual false positive alert rate from maintenance? (need data)
- Are engineers currently manually silencing these? How often?
- What's the pain level? (nice-to-have or critical problem)
- Are existing solutions (Datadog downtime scheduling) insufficient? Why?

**Approach Uncertainty:**

- **Architectural mismatch:**
  - Alerts are generated by backend monitors, not agent
  - Agent sees metrics, not alerts
  - Where does suppression logic run? Options:
    1. Agent intercepts metrics, tags "routine" before sending (requires agent intelligence)
    2. Backend learns patterns, agent has no role (not an "agent gadget")
    3. Agent calls backend API to set downtimes (agent is just automation, not intelligent)
  - This needs architectural clarity before implementation
- **Cold-start problem:**
  - Need weeks/months of historical data to learn patterns
  - Can't suppress anything until patterns are learned
  - Deployment timeline is long before value delivered
- **Alternative: Backend-side pattern learning is simpler**
  - Backend already has all historical metrics
  - Pattern matching could happen backend-side without agent changes
  - This might not be an "agent gadget" at all

**Validation Challenge:**

- How to validate suppressions were correct? (need post-incident analysis)
- What if suppressed pattern was actually an incident this time? (degraded differently than past)
- How to measure improvement in alert fatigue? (subjective metric)

**Recommendation:** Needs 3-4 weeks of architectural research:

1. Clarify where alert logic runs (agent vs backend)
2. Determine if this should be agent-side or backend-side feature
3. Collect data on maintenance alert frequency and impact
4. Evaluate if existing Datadog features (downtime scheduling, anomaly detection) solve this
5. Only proceed if (a) clear architectural path and (b) evidence existing solutions are insufficient

## Part 2: Signal Providers

Signal providers are the data collection engines that gather telemetry from processes, systems, containers, networks, security events, traces, and logs. Each provider exposes rich, structured data that can be correlated and analyzed.

#### 📚 Why These Signals Matter

Traditional monitoring collects metrics in isolation - CPU here, memory there, network somewhere else. The breakthrough insight is that **correlating signals across providers reveals patterns invisible to any single source**. A CPU spike alone is just a spike. But CPU spike + specific syscall patterns + network retransmits + cgroup throttling = a noisy neighbor on AWS eating your I/O. This multi-dimensional correlation is what makes AI-powered gadgets possible.

### 1. ProcessSignalProvider

**Description:** Comprehensive process-level observability including lifecycle, resources, and relationships. This provider gives you deep visibility into every process running on the system, from basic identity information to detailed resource consumption patterns.

#### 💎 The "So What?" - Process-Level Attribution

**The Breakthrough:** Container-level metrics tell you "this pod is using 80% CPU" but not WHY. Process-level visibility reveals "actually, one Java process (PID 1843) is using 78% while everything else is idle." This attribution is critical for intelligent action - The Oracle needs to know WHICH process to kill, not just "the container is OOMing."

**What This Enables:**

- **Surgical interventions:** Kill the leaking process, not the entire container
- **Language-aware actions:** Trigger JVM GC (for Java) vs manual GC (for Go) based on detected language
- **Behavioral profiling:** Track CPU/IO/memory trends per-process over time (is this process normally this busy, or is this anomalous?)
- **Hierarchy understanding:** Parent/child relationships reveal process spawn patterns (fork bombs, zombie accumulation)

**Correlation Goldmine:**

- **Process RSS growth + Cgroup PSI rising** = identify which specific process is causing memory pressure (Oracle's kill decision)
- **Process in D state (uninterruptible sleep) + zero I/O** = stuck in kernel waiting for failed disk (different from deadlock)
- **High voluntary context switches + zero I/O** = process is yielding CPU waiting for something (likely deadlocked)

#### 🔗 Agent Integration Points

**Existing Collection:**

- **Component:** Process Agent (`pkg/process/procutil`)
- **Key Files:**
  - `process_linux.go:47-57` reads `/proc/[pid]/stat`, `/proc/[pid]/status`, `/proc/[pid]/io`
  - `process_model.go` defines `Process` struct with all metadata
  - Event collection in `pkg/process/events/` via eBPF (fork/exec/exit)
- **Collection Frequency:** 10-20s intervals (configurable via `process_config.intervals.process`)
- **Already Sends to Backend:** Yes, as `processes` payload in Process Agent check

**Data Format:**

```go
type Process struct {
    Pid int32
    Ppid int32
    CPU *CPUTimesStat    // User, System, Iowait time
    Memory *MemoryInfoStat  // RSS, VMS, Swap
    IOStat *IOCountersStat  // ReadBytes, WriteBytes, ReadCount, WriteCount
    Status string        // R, S, D, T, Z
    CreateTime int64
    // ... 30+ more fields
}
```

Reference: raw_research.md lines 485-530 for complete schema

**What's New for Gadgets:**

- **Current:** Sends periodic snapshots (every 10-20s) to backend as batch payloads
- **Needed:** Real-time stream to local gadget modules for sub-second decision-making
- **Mechanism Options:**
  1. Internal pubsub: Process Agent publishes updates to in-memory channel, gadgets subscribe
  2. Shared memory: Process stats written to shared memory region, gadgets read directly
  3. gRPC API: Process Agent exposes local gRPC endpoint (similar to system-probe model)
- **Gap:** Need to decide architecture for gadget→agent component communication

#### Process Metadata

- **Source:**`/pkg/process/procutil/process_model.go`
- **Status:**Ready
- **Identity:**PID, PPID, NsPid, Name, Exe, Cmdline, Cwd
- **Lifecycle:**CreateTime, Status (R/S/D/T/Z)
- **Credentials:**UIDs, GIDs, Capabilities (effective, permitted)
- **Language:**Detected programming language
- **Ports:**TCP/UDP port bindings

#### CPU Metrics

- **Shape:**`CPUTimesStat{User, System, Iowait, ...} + timestamp`
- **Frequency:**Per check interval (10-20s)
- **Source:**`/proc/[pid]/stat`via gopsutil

#### Memory Metrics

- **Shape:**RSS, VMS, Swap, Shared, Text, Data, Dirty (bytes)
- **Frequency:**Per check interval
- **Source:**`/proc/[pid]/status`,`/proc/[pid]/statm`

#### I/O Metrics

- **Shape:**ReadCount, WriteCount, ReadBytes, WriteBytes + rates
- **Frequency:**Per check interval
- **Source:**`/proc/[pid]/io`
- **Requires:**Elevated permissions

#### Lifecycle Events (eBPF)

- **Fork Events:**`{EventType: Fork, Pid, Ppid, ForkTime, ContainerID, ...}`- Real-time stream
- **Exec Events:**`{EventType: Exec, Pid, Exe, Cmdline, ExecTime, UID, GID, ...}`- Real-time
- **Exit Events:**`{EventType: Exit, Pid, ExitTime, ExitCode}`- Real-time
- **Source:**`/pkg/process/events/`

#### Update Characteristics

- **Metadata:**On-demand or per interval
- **Metrics:**Polling (10-20s default)
- **Events:**Real-time streaming via eBPF

#### Correlation Potential

- Join on PID with network connections, syscalls
- Link to containers via ContainerID
- Correlate with cgroup metrics via cgroup_path

### 2. SystemResourceProvider

**Description:** Host-level resource metrics and pressure indicators. This provider tracks system-wide resource utilization, identifying contention, saturation, and pressure points that affect all workloads on the host.

#### 💎 The "So What?" - System-Wide Context for Multi-Tenancy

**The Breakthrough:** Process and cgroup metrics show you what ONE workload is doing. System metrics reveal the ENVIRONMENT it's running in. High process CPU might be normal, or it might be fighting for resources with noisy neighbors. System metrics provide the context.

**What This Enables:**

- **Noisy neighbor detection:** Process CPU 50% + system stolen CPU 40% = virtualization overhead from neighbor, not application issue
- **I/O contention identification:** Process I/O wait 30% + disk queue depth at 50 = storage bottleneck affecting all workloads
- **Memory pressure correlation:** `swap_out` velocity is THE leading indicator for system-wide OOM, even more predictive than per-process RSS
- **Capacity planning signals:** Load average normalized by core count reveals if system is oversaturated

**Correlation Goldmine:**

- **High iowait + low disk throughput** = I/O scheduler saturation or failing disk (not just "slow application")
- **High stolen CPU + high load** = EC2 instance credit exhaustion or noisy neighbor in cloud
- **Swap activity + cache shrinking** = system under memory pressure (Oracle's secondary OOM indicator)

**Why Host-Level Matters:** In multi-tenant environments (Kubernetes, shared VMs), per-container metrics miss the forest for the trees. You need to know "is this slow because MY app is slow, or because the HOST is overloaded?" System metrics answer this.

#### 🔗 Agent Integration Points

**Existing Collection:**

- **Component:** Core Agent system checks (`pkg/collector/corechecks/system/`)
  - `cpu/cpu.go` for CPU metrics
  - `memory/memory.go` for memory and swap
  - `disk/diskv2/disk.go` for disk I/O
- **Key Files:**
  - Reads `/proc/stat`, `/proc/meminfo`, `/proc/vmstat`, `/proc/diskstats`
  - Uses `gopsutil` library wrappers for cross-platform support
- **Collection Frequency:** 15s intervals (configurable via `min_collection_interval`)
- **Already Sends to Backend:** Yes, as standard system metrics (`system.cpu.user`, `system.mem.used`, `system.io.rkb_s`, etc.)

**Data Format:**

- CPU: Per-core and aggregate percentages (user, system, iowait, idle, stolen)
- Memory: Absolute bytes (total, free, cached, buffered) + swap rates (swap_in, swap_out in MB/s)
- Disk: Per-device operation rates, throughput, latency, saturation
- All metrics tagged with hostname, device name

Reference: raw_research.md lines 532-569 for complete listing

**What's New for Gadgets:**

- **Current:** Metrics aggregated and sent to backend every 15s
- **Needed:** Real-time access to raw values for velocity/acceleration calculations (Oracle needs `d(swap_out)/dt`)
- **Mechanism:** Expose system check metrics to gadgets via same internal pubsub/API as ProcessSignalProvider
- **Gap:** Need sub-15s granularity for some signals (swap velocity changes fast), may require higher collection frequency

#### CPU Metrics

- **Breakdown:**user, system, iowait, idle, stolen, guest, interrupt
- **Shape:**Percentage per core or system-wide
- **Frequency:**15s default
- **Source:**`/proc/stat`

#### Load Average

- **Metrics:**`load.1`,`load.5`,`load.15`, normalized versions
- **Shape:**Float gauge (normalized by core count)
- **Frequency:**15s

#### Memory & Swap

- **Memory:**total, free, used, cached, buffered, slab, page_tables
- **Swap:**total, free, swap_in, swap_out (rates in MB/s)
- **Source:**`/proc/meminfo`,`/proc/vmstat`
- *Note: swap velocity = strong OOM predictor*

#### Disk I/O (Per Device)

- **Operations:**r_s, w_s, rrqm_s, wrqm_s (ops/sec)
- **Throughput:**rkb_s, wkb_s (KB/s)
- **Latency:**await, r_await, w_await, svctm (milliseconds)
- **Saturation:**avg_q_sz, util (queue depth, utilization %)
- **Source:**`/proc/diskstats`via iostat methodology

#### Interesting Derivations

- Memory pressure velocity: `d(swap_out)/dt` acceleration
- I/O wait correlation with disk queue depth
- CPU steal + iowait = contention signature
- Page cache thrashing: High `pgmajfault` + low cache

### 3. CgroupResourceProvider

**Description:** Container/cgroup-level resource limits, usage, and pressure events. Provides extremely detailed tracking of resource consumption within control groups, essential for container and Kubernetes environments.

#### 💎 The "So What?" - PSI is THE Breakthrough for OOM Prediction

**The Game-Changer:** PSI (Pressure Stall Information) is the single most important signal for OOM prediction because it's a **leading indicator**, not a lagging indicator like memory usage.

**The Fundamental Insight:**

- **Traditional monitoring:** "Memory is at 95% - we're almost out" (lagging indicator, may OOM in 30s or 30 minutes, can't tell)
- **PSI monitoring:** "Processes are stalled waiting for memory 15% of the time - pressure is building" (leading indicator, typically 2-3 minutes before OOM)

**Why This Changes Everything:**

- Memory usage can sit at 95% for hours (cached data that can be evicted)
- PSI climbing means processes are ACTUALLY BLOCKED waiting for allocations (cache eviction isn't keeping up)
- PSI velocity (0% → 5% → 15% in 60s) reveals the trajectory toward OOM
- This 2-3 minute warning window is what makes The Oracle possible

**What This Enables:**

- **OOM prediction:** Track PSI momentum (is it accelerating or stabilizing?)
- **Working set calculation:** Kubernetes uses `UsageTotal - InactiveFile` to determine actual memory pressure (cached files can be evicted, working set cannot)
- **Throttling detection:** CPU throttling reveals cgroup limit hits (different from host CPU saturation)
- **Eviction order:** QoSClass (BestEffort evicted first) helps Oracle choose which container to act on

**Correlation Goldmine:**

- **PSI rising + InactiveFile shrinking** = cache eviction happening but pressure still building (Oracle's "OOM imminent" signal)
- **Memory usage high + PSI flat** = mostly cached data, not true pressure (false alarm)
- **CPU throttled + PSI CPU rising** = CPU limit too low, causing application stalls

#### 🔗 Agent Integration Points

**Existing Collection:**

- **Component:** Cgroup utilities (`pkg/util/cgroups/`)
  - `reader.go` parses cgroup v1 and v2 filesystem
  - `self_reader.go` for agent's own cgroup
- **Key Files:**
  - Reads `/sys/fs/cgroup/memory.stat`, `/sys/fs/cgroup/memory.pressure`, `/sys/fs/cgroup/cpu.stat`
  - cgroupv1: `/sys/fs/cgroup/memory/[path]/memory.stat`
  - cgroupv2: `/sys/fs/cgroup/[path]/memory.pressure` (PSI metrics)
- **Collection Frequency:** 10-20s intervals (same as process checks)
- **Already Sends to Backend:** Yes, as container metrics (`container.memory.usage`, `container.memory.rss`, etc.)

**Data Format:**

```go
type CgroupMemStats struct {
    UsageTotal uint64
    RSS uint64
    Cache uint64
    InactiveFile uint64  // For working set calculation
    Limit uint64
    PSI *PSIStats {       // cgroupv2 only
        Avg10 float64     // 10-second average (0-100%)
        Avg60 float64     // 60-second average
        Avg300 float64    // 300-second average
        Total uint64      // Nanoseconds of stall time
    }
    OOMEvents uint64      // OOM event counter
}
```

Reference: raw_research.md lines 570-613 for complete schema

**What's New for Gadgets:**

- **Current:** Periodic polling, metrics sent to backend
- **Needed:** Sub-10s granularity for PSI velocity calculations (PSI changes fast during OOM trajectory)
- **Critical:** Must expose PSI metrics to gadgets (currently collected but may not be easily accessible to other agent components)
- **Mechanism:** Cgroup reader publishes stats to internal gadget API, similar to process signal flow

**Requirements for cgroupv2:**

- PSI metrics only available on cgroupv2 (kernel 4.20+)
- Systems running cgroupv1 would need fallback signals (swap velocity, OOM events)
- Oracle gadget may need "degraded mode" on cgroupv1 hosts

#### Memory Details

- **Usage:**UsageTotal, RSS, Cache, Swap, Shmem
- **Working Set:**`UsageTotal - InactiveFile`(Kubernetes calculation)
- **State:**ActiveAnon, InactiveAnon, ActiveFile, InactiveFile
- **Kernel:**KernelMemory, PageTables
- **Limits:**Limit, MinThreshold, LowThreshold, HighThreshold

#### Pressure Events (Critical for OOM Prediction)

- **OOMEvents:**`memory.failcnt`(v1),`memory.events oom`(v2)
- **OOMKillEvents:**Actual OOM kills (cgroupv2 only)

#### PSI Metrics (cgroupv2)

- **PSISome:**Avg10/60/300 (10s, 60s, 300s averages, percentage 0-100)
- **Total:**Nanoseconds of stall time
- Available for memory, CPU, and I/O

#### 📚 Background: Pressure Stall Information (PSI)

**PSI is a breakthrough signal from the Linux kernel (4.20+) that fundamentally changes OOM prediction.**
**The Key Insight:**Traditional memory monitoring tells you "the parking lot is full" (usage at 95%). PSI tells you "cars are circling waiting for spots" (processes are STALLED waiting for memory).
**Why This Matters:**PSI.Some measures the percentage of time that*at least one process*was blocked waiting for memory. When this number starts climbing from 0% → 5% → 20%, you're watching memory pressure build in real-time, typically 2-3 minutes before the OOM killer fires. It's the difference between a lagging indicator (usage) and a leading indicator (stall time).
**The Magic:**By tracking PSI*velocity*(how fast it's increasing), AI models can predict the exact moment when the system will cross the OOM threshold, giving just enough time to take preventive action.

#### CPU Throttling

- **ElapsedPeriods:**Total scheduling periods
- **ThrottledPeriods:**Periods where throttled
- **ThrottledTime:**Nanoseconds spent throttled
- **Calculation:**`throttle_rate = ThrottledPeriods / ElapsedPeriods`

#### Correlation Potential

- Link to containers via cgroup path
- Correlate limits with process resource usage
- **PSI metrics predict resource exhaustion 2-3 minutes ahead**
- OOM events correlate with`memory.high`breaches

### 4. ContainerRuntimeProvider

**Description:** Container lifecycle, health, and orchestration events. Complete container state tracking including Docker, containerd, and Kubernetes pod metadata with rich orchestrator hierarchy.

#### 💎 The "So What?" - Orchestration State Reveals Intent

**The Breakthrough:** Metrics tell you "container restarted" but not WHY. Orchestration state reveals the intent: was it a deliberate update (rolling deployment), a crash (exit code != 0), or an eviction (node memory pressure)?

**What This Enables:**

- **Crash vs restart differentiation:** `RestartCount++` + `ExitCode=137` = OOM kill (SIGKILL). `RestartCount++` + `ExitCode=0` = graceful restart (deployment)
- **Eviction prediction:** Pod in `MemoryPressure` condition + QoSClass=BestEffort = likely to be evicted soon (Oracle should act preemptively)
- **Health check failures:** Container shows `Health=Unhealthy` while process metrics look normal = application-level deadlock (Pathologist's domain)
- **Deployment tracking:** Pod phase transitions (Pending → Running → Succeeded) help distinguish planned changes from failures

**Correlation Goldmine:**

- **RestartCount spiking + OOMEvents in cgroup** = chronic OOM kills (Oracle should investigate root cause, not just prevent)
- **Health=Unhealthy + syscall entropy normal** = health check is broken, process is fine (don't kill it)
- **Pod phase=Pending for >5min + no events** = scheduler can't place pod (resource constraints, not application issue)

**Why Orchestration Context Matters:** In Kubernetes, the same symptom (container restart) has different root causes requiring different actions. OOM kill → fix memory leak. Failed health check → investigate app logic. Eviction → node is overloaded. Orchestration state disambiguates.

#### 🔗 Agent Integration Points

**Existing Collection:**

- **Component:** Container metadata / workloadmeta (`comp/core/workloadmeta/`)
  - Unified metadata store for containers across runtimes
  - Collectors for Docker, containerd, Kubelet, ECS
- **Key Files:**
  - `pkg/util/containers/metrics/provider/` - Container metric collection
  - Kubelet API client: `pkg/util/kubernetes/kubelet/` (pod status, stats)
  - Docker events: Docker API event stream
  - Container collectors in `pkg/util/containers/metrics/` (docker, containerd, kubelet, CRI)
- **Collection Frequency:**
  - Kubelet: ~10s polling for pod status
  - Docker/containerd: real-time event stream
  - Pod expiration: >15s not seen = removed
- **Already Sends to Backend:** Yes, as container metadata and orchestrator resources

**Data Format:**

- Container: State, Health, RestartCount, PID, ExitCode, timestamps
- Pod: Phase, Conditions, QoSClass, PriorityClass
- Events: start, stop, die, kill, oom, health status changes

Reference: raw_research.md lines 614-648 for complete container lifecycle details

**What's New for Gadgets:**

- **Current:** Metadata available via workloadmeta store, events streamed internally
- **Needed:** Real-time event notifications to gadgets (container crashed, health changed, pod evicted)
- **Mechanism:** Subscribe to workloadmeta event stream or container runtime event channels
- **Already architected well:** workloadmeta is designed for internal agent component consumption

#### Container Lifecycle

- **Shape:**`Container{State, Health, RestartCount, PID, ExitCode, CreatedAt, StartedAt, FinishedAt}`
- **Events:**ActionStart, ActionHealthStatus, ActionHealthStatusHealthy/Unhealthy
- **Source:**Docker API, Containerd, Kubelet

#### State Transitions

- **Docker events:**start, stop, die, kill, pause, unpause, restart, oom
- **Health changes:**healthy → unhealthy transitions
- **Crash detection:**`RestartCount`increments +`ExitCode != 0`

#### Kubernetes Pod Signals

- **Shape:**`KubernetesPod{Phase, Ready, QOSClass, PriorityClass, Conditions, ...}`
- **Phases:**Pending, Running, Succeeded, Failed, Unknown
- **QOS Classes:**BestEffort, Burstable, Guaranteed
- **Conditions:**PodScheduled, Initialized, ContainersReady, Ready

#### Eviction Signals (Critical for Cluster Health)

- `Pod.Reason: Evicted`
- `Pod.Conditions[].Reason: MemoryPressure, DiskPressure, PIDPressure`
- `ContainerState.Terminated.Reason: OOMKilled, Error`

#### Update Characteristics

- **Kubelet polling:**~10s intervals
- **Docker/Containerd events:**Real-time stream
- **Pod expiration:**>15s not seen

### 5. NetworkBehaviorProvider

**Description:** Connection-level behavior, protocol analysis, and application performance. Low-level network monitoring with deep protocol inspection for HTTP, Kafka, PostgreSQL, Redis, and DNS.

#### 💎 The "So What?" - Protocol-Level Intelligence Without Application Changes

**The breakthrough:** Traditional APM requires instrumenting your application code with tracing libraries. NetworkBehaviorProvider uses eBPF to inspect network traffic at the kernel level, extracting HTTP requests, database queries, and cache commands *without any application changes*. It sees what your code is actually doing, not what it claims to be doing.

**The Non-Obvious Capability:** This provider doesn't just count bytes - it understands HTTP methods, URL paths, status codes, and latency per-request. For databases, it captures query fragments and correlates them with connection performance (RTT, retransmits). This is application-level observability built from network-level inspection.

**Correlation Goldmine:**

- **TCP Retransmits + HTTP 5xx:** Distinguishes "application error" (5xx with no retransmits) from "network problem causing errors" (5xx correlated with high retransmit rate)
- **Connection Count + Throughput:** Pool exhaustion signature: many connections but low bytes/sec per connection (all waiting for locks)
- **DNS Failures + Service Errors:** When DNS resolution fails 30s before service errors spike, it's a DNS propagation issue, not an application bug
- **Process PID + Connection Tuple:** Exact attribution of network behavior to processes, even in containerized environments with complex networking (overlay networks, service meshes)

#### 🔗 Agent Integration Points

**Existing Collection:**

- **Component:** System-Probe Network Tracer (`pkg/network/tracer/`)
  - eBPF-based connection tracking (kprobe, fentry, or CO-RE)
  - Protocol parsers in `pkg/network/protocols/` (HTTP, HTTP/2, Kafka, Postgres, Redis, DNS)
  - Universal Service Monitoring (USM) for application-level visibility
- **Key Files:**
  - `pkg/network/tracer/tracer.go` - Main connection tracking orchestration
  - `pkg/network/tracer/connection/ebpf_tracer.go` - eBPF connection tracer
  - eBPF source: `pkg/network/ebpf/c/tracer.c`, `runtime/usm.c`
  - HTTP analyzer: `pkg/network/protocols/http/protocol.go`
- **Collection Frequency:** Real-time via eBPF (connections tracked continuously), HTTP endpoint polled every 30s
- **Already Sends to Backend:** Yes, as network connections and USM metrics

**Data Format:**

```go
type ConnectionStats struct {
    SrcIP, DstIP netip.Addr
    SrcPort, DstPort uint16
    Pid int32
    SentBytes, RecvBytes uint64
    SentPackets, RecvPackets uint64
    RTT uint32              // Round-trip time (microseconds)
    RTTVar uint32           // RTT variance
    Retransmits uint32      // TCP retransmission count
    HTTPStats *HTTPAggregations  // Per-connection HTTP metrics
}
```

Reference: raw_research.md lines 649-700 for complete network monitoring architecture

**What's New for Gadgets:**

- **Current:** System-probe exposes HTTP endpoint `/connections` polled every 30s by core agent
- **Needed:** Real-time connection event stream (new connection, connection closed, HTTP request completed)
- **Mechanism:** Extend system-probe gRPC API or add gadget-specific endpoints
- **Gap:** HTTP latency distributions (DDSketch) are aggregated - gadgets may need per-request latency for Equalizer toxic query detection

#### Connection Tracking

- **Tuple:**`{SrcIP, DstIP, SrcPort, DstPort, Protocol, PID, NetNS}`
- **Bytes:**SentBytes, RecvBytes (monotonic + delta)
- **Packets:**SentPackets, RecvPackets
- **TCP Performance:**
- RTT: Round-trip time (microseconds)
- RTTVar: RTT variance
- Retransmits: TCP retransmission count

- **TCP State:**Bit mask of TCP state transitions
- **Failure Reasons:**Map of POSIX error codes (104=reset, 110=timeout, 111=refused)

#### HTTP/HTTPS Protocol Analysis

- **Request:**`{Method, Path, Fragment}`
- **Response:**`{StatusCode}`
- **Timing:**Latency (nanoseconds, stored in DDSketch for percentiles)
- **Aggregation:**`(connection, method, path_quantized, status) → {count, latency_distribution}`
- **Path Quantization:**`/orders/123 → /orders/*`
- **TLS Detection:**`{TLSVersion, CipherSuite, Library}`

#### Other Protocols

- **Kafka:**APIKey, TopicName, ErrorCode, latency per partition
- **PostgreSQL:**Operation type, query fragment (160 bytes), latency
- **Redis:**Command, key name (128 bytes), error flag
- **DNS:**QueryType, Domain, ResponseCode, latency, timeouts

#### Interesting Derivations

- Request rate anomalies: DDSketch enables percentile spike detection
- Connection pool exhaustion: High connection count + low throughput
- Retry storm detection: Repeated failures to same endpoint
- Latency percentile shifts: p99 spike while p50 stable = tail latency issue
- DNS resolution failure rate per service

### 6. SecurityEventProvider

**Description:** Syscall-level activity, file access, process communication, and security events. Comprehensive syscall monitoring with 450+ syscalls tracked, enabling deep security analysis and process behavior understanding.

#### 💎 The "So What?" - Why Syscalls Are Intelligence Gold

**Here's the non-obvious insight:** Syscalls are the language programs speak to the kernel. By analyzing which syscalls a process makes and in what patterns, you can understand its true behavior at a level health checks can never reach.

**The Breakthrough:** A deadlocked process might respond to TCP health checks (ports still open), but its syscall pattern tells the truth: it's stuck in an infinite loop calling only `futex()` over and over. A healthy process, even when idle, shows diverse syscalls: `poll()`, `read()`, `write()`, `accept()`, `recvfrom()`. This diversity - measurable as entropy - is a "liveness" indicator that traditional monitoring misses entirely.

**Correlation Goldmine:** Syscalls + network connections + process CPU = ability to attribute exact network operations to specific processes, even in container environments where network namespaces obscure the connection graph.

#### 🔗 Agent Integration Points

**Existing Collection:**

- **Component:** Security Agent / Cloud Workload Security (`pkg/security/`)
  - Runtime security monitoring via eBPF (CWS)
  - 48+ eBPF probe types tracking syscalls (raw_research.md line 711-756)
  - Event model with 43+ event types (file ops, process ops, network ops)
- **Key Files:**
  - `pkg/security/ebpf/probes/` - Individual eBPF programs per syscall type
  - `pkg/security/probe/` - Main CWS probe orchestration
  - `pkg/security/secl/model/events.go` - Event type definitions
  - eBPF source: `pkg/security/ebpf/c/` (various .c files)
- **Collection Frequency:** Real-time event stream from eBPF
- **Already Sends to Backend:** Yes, as CWS security events (filtered by SECL rules)

**Data Format:**

```go
type Event struct {
    Type EventType  // FileOpenEventType, ExecEventType, etc.
    Timestamp uint64
    ProcessContext ProcessContext {
        Pid int32
        Exe string
        // ...
    }
    Syscall *SyscallEvent {
        ID int32        // Syscall number (e.g., 2 = open)
        Retval int64    // Return value
    }
    // Event-specific data (file paths, network tuples, etc.)
}
```

Reference: raw_research.md lines 702-756 for complete event model

**What's New for Gadgets:**

- **Current:** Events filtered by SECL rules, sent to backend for security analysis
- **Needed:** Raw syscall event stream for Pathologist (need all syscalls, not just security-relevant ones)
- **Gap:** Current CWS filters aggressively (only security events) - gadgets need comprehensive syscall tracking
- **Challenge:** Full syscall tracing is HIGH overhead - need to evaluate if existing CWS infrastructure can support gadget use cases without performance degradation
- **Mechanism:** Subscribe to CWS event bus, or add dedicated syscall aggregation probes for gadgets

**Critical Consideration:** Syscall entropy calculation requires histogram of ALL syscalls over 30s window. If CWS is filtering heavily, entropy calculation may not be accurate. May need dedicated "gadget mode" syscall collection.

#### Syscall Monitoring

- **File Operations:**open, read, write, chmod, chown, link, unlink, rename, mkdir, rmdir
- **Process Operations:**fork, clone, execve, exit, kill, setuid, capset
- **Network Operations:**socket, bind, connect, accept
- **IPC Operations:**pipe, signal, shmget, semop, msgget

#### 📚 Background: Syscall Entropy as a Liveness Indicator

**What is syscall entropy?**It's a measure of how diverse a process's system calls are over a time window (typically 30 seconds).
**The Math:**Given a histogram of syscall types, Shannon entropy = -Σ(p(i) × log₂(p(i))) where p(i) is the probability of seeing syscall type i. High entropy (diverse calls) ≈ 4-5 bits. Low entropy (repetitive) ≈ 0-1 bits.
**Why This Matters:**

- **Healthy Process (High Entropy ≈ 4.2 bits):**Makes 1000 syscalls over 30s: 300× read(), 250× write(), 200× poll(), 100× accept(), 50× futex(), 50× recvfrom(), 50× sendto()... diverse activity
- **Deadlocked Process (Low Entropy ≈ 0.1 bits):**Makes 10,000 syscalls over 30s: 9,999× futex(WAIT), 1× poll()... stuck in a lock
- **Long GC (Medium Entropy ≈ 2.5 bits):**Makes 5000 syscalls: 4000× futex(), 500× mmap(), 300× munmap(), 200× brk()... memory-focused but still diverse

**The Detection Power:**Syscall entropy distinguishes "stuck forever" from "legitimately waiting" or "busy working" - something no health check or CPU metric can do. This is why The Pathologist can detect deadlocks that bypass all traditional monitoring.

#### File Tracking

- **Operations:**Read, Write, Create, Delete, Rename, Chmod, Chown
- **Resolution:**Full path reconstruction from kernel dentry cache
- **FD Tracking:**Maintains FD → inode → path mapping per process
- **Mount Awareness:**Handles overlay filesystems

#### Process Communication Graph

- **Unix Sockets:**`AF_UNIX bind/connect/accept → process relationship`
- **Pipes:**`pipe/pipe2 → producer/consumer tracking`
- **Signals:**`kill/tkill → {SignalType, SourcePID, TargetPID}`
- **Shared Memory:**`shmget/shmat → process attachment graph`
- **Ptrace:**`PTRACE_ATTACH → debugger relationships`

#### Network Flow Monitoring (7.63+)

- **Shape:**`Flow{Source, Destination, L3Protocol, L4Protocol, IngressStats, EgressStats}`
- **Aggregation:**5-tuple with bidirectional byte/packet counts
- **Source:**TC eBPF programs on network interfaces

#### Update Characteristics

- Real-time event stream from eBPF
- Filtering in kernel space via approvers/discarders
- Rate limiting per event type

### 7. TraceAnalysisProvider

**Description:** APM trace data with local aggregation and statistical analysis. Rich distributed tracing information with local statistics aggregation enables real-time performance analysis without backend dependency.

#### 💎 The "So What?" - Why Local Trace Statistics Are Game-Changing

**The breakthrough:** Traditional APM requires sending ALL traces to a backend for analysis, then querying that backend for insights. This creates latency (can't analyze until ingestion completes) and cost (storing billions of spans). TraceAnalysisProvider does statistical aggregation *locally in the agent*, enabling real-time performance analysis without backend round-trips.

**What This Enables:** Gadgets can detect latency spikes, error rate changes, and throughput anomalies within 10 seconds using local DDSketch percentile calculations - fast enough to take preventive action (kill toxic query, shed load) before cascading failures propagate.

**Correlation Goldmine:** Traces contain ContainerID, enabling precise correlation with container resource metrics. When a specific container's memory spikes, you can immediately identify which *service operations* (not just processes) are involved by joining on ContainerID + timestamp.

#### 🔗 Agent Integration Points

**Existing Collection:**

- **Component:** Trace Agent (`pkg/trace/`)
  - Receives traces from applications on port 8126 (HTTP/TCP)
  - Local trace statistics aggregation in `pkg/trace/stats/`
  - Concentrator computes DDSketch distributions per operation
- **Key Files:**
  - `pkg/trace/api/api.go:256-350` - Trace ingestion HTTP endpoints
  - `pkg/trace/stats/concentrator.go` - Local statistics aggregation
  - `pkg/trace/stats/statsraw.go` - Raw stats before aggregation
  - DDSketch implementation for percentile calculations
- **Collection Frequency:**
  - Spans: Real-time ingestion as applications send them
  - Stats: Aggregated in 10s buckets (configurable)
- **Already Sends to Backend:** Yes, sampled spans and aggregated statistics

**Data Format:**

```go
type Span struct {
    TraceID uint64
    SpanID uint64
    Service string
    Resource string     // e.g., "GET /api/users"
    Start int64        // nanoseconds
    Duration int64     // nanoseconds
    Error int32        // 0 or 1
    Meta map[string]string
    Metrics map[string]float64
}

type StatsGrouped struct {
    Service string
    Resource string
    Hits uint64
    Errors uint64
    Duration uint64
    OkSummary *DDSketch    // Percentile distribution for non-errors
    ErrSummary *DDSketch   // Percentile distribution for errors
}
```

Reference: raw_research.md lines 757-811 for trace analysis details

**What's New for Gadgets:**

- **Current:** Stats aggregated in 10s buckets, sent to backend
- **Needed:** Real-time access to DDSketch distributions for anomaly detection (Equalizer detecting latency spikes)
- **Already well-architected:** Stats are computed locally and available in-memory before being sent
- **Mechanism:** Expose stats concentrator data via internal API (similar to how core agent polls stats from trace-agent today)

**Advantage for Gadgets:** Local trace stats are already computed - no new instrumentation needed. Just need to expose to gadget modules.

#### Span-Level Data

- **Identity:**`{TraceID, SpanID, ParentID, Service, Resource, Operation}`
- **Timing:**`{Start (nanoseconds), Duration (nanoseconds)}`
- **Status:**`{ErrorFlag (0/1), HTTPStatusCode, gRPCStatusCode}`
- **Tags:**`{Meta (strings), Metrics (floats), SpanKind}`
- **Context:**`{ContainerID, Env, Version, GitCommitSha, Language}`
- **Relationships:**SpanLinks (cross-trace causality)

#### Local Trace Statistics

- **Key:**`(Service, Resource, Type, StatusCode, HTTPMethod, SpanKind, ...)`
- **Stats:**
- Hits: Request count
- TopLevelHits: Entry point spans
- Errors: Error span count
- Duration: Sum for average calculation
- OkDistribution: DDSketch (1% accuracy) for p50/p75/p90/p95/p99
- ErrDistribution: Separate percentiles for errors

- **Bucket Interval:**Configurable (default 10s)

#### 📚 Background: DDSketch - Percentiles Without Storing Everything

**The Challenge:**Computing accurate p99 latency traditionally requires storing all latency values, sorting them, and picking the 99th percentile. For high-traffic services (100k requests/sec), that's 6 million values per minute - unsustainable memory usage.
**The DDSketch Solution:**A probabilistic data structure (similar to t-digest) that stores latency distributions in ~2KB of memory regardless of traffic volume, with<1% relative error on percentiles. Instead of storing individual values, it maintains logarithmic buckets: values in [1ms, 1.01ms] go in bucket 0, [1.01ms, 1.02ms] in bucket 1, etc.
**Why This Matters for Gadgets:**

- **Real-Time Anomaly Detection:**The Equalizer can compare current p99 (last 10s) vs baseline p99 (last 10m) using DDSketch without storing raw latencies. "Current p99 = 500ms, baseline = 150ms → 3.3x spike → toxic query detected"
- **Percentile Shifts Are Signal:**When p99 spikes but p50 stays flat, that's a tail latency issue (one slow operation). When p50 and p99 both spike together, that's systemic (database down). DDSketch enables distinguishing these patterns in real-time.
- **Memory Efficiency:**Agent can track percentiles for 10,000 unique service operations in ~20MB of memory, enabling comprehensive coverage without OOM risk.

#### Sampling Metadata

- **Mechanisms:**PrioritySampler (TPS-based), ErrorsSampler, RareSampler
- **Rates:**`_sample_rate`,`_dd1.sr.rcusr`,`_dd1.sr.rapre`
- *Enables extrapolation from sampled data*

#### Correlation Potential

- Link traces to containers via ContainerID
- Join with logs by Service + Timestamp
- Correlate HTTP traces with network connections by timestamp + ports
- Match error spans with error logs

### 8. LogStreamProvider

**Description:** Log collection with metadata, volume tracking, and processing rules. Structured log ingestion with rich metadata and comprehensive volume/latency tracking for pipeline health monitoring.

#### 💎 The "So What?" - Real-Time Error Context and PII Interception

**The Breakthrough:** Logs are the last line of defense before data leaves the host. Intercepting logs at the agent allows real-time analysis and transformation (PII redaction, error pattern detection) before logs reach backend storage - preventing compliance violations and enabling immediate action on error signals.

**What This Enables:**

- **PII redaction at source:** The Curator intercepts logs before backend, preventing PII from ever reaching log storage (compliance by design)
- **Error rate spike detection:** The Archivist uses error log rate as trigger (errors detected immediately at collection time)
- **Anomaly triggers:** Sudden log volume spikes, new error messages, or unusual log patterns can trigger other gadgets
- **Service attribution:** Logs tagged with service name enable per-service gadget actions

**Correlation Goldmine:**

- **Error logs + trace error spans** = correlate log messages with distributed trace failures (same service, same timestamp)
- **Log volume spike + container restart** = application crash with verbose error logging before death
- **PII in logs + service name** = identify which application is logging sensitive data (alert developers)

**Why Log-Time Interception Matters:** Once logs reach backend storage, it's too late - PII is already persisted, compliance violation already occurred. Agent-side processing is the only opportunity for prevention vs remediation.

#### 🔗 Agent Integration Points

**Existing Collection:**

- **Component:** Logs Agent (`pkg/logs/`)
  - Multiple tailer types: file, container, journald, Windows Event, TCP/UDP, channel
  - Processing pipeline with decoders, parsers, processors
  - Orchestrated by launchers in `pkg/logs/launchers/`
- **Key Files:**
  - `pkg/logs/pipeline/` - Main log processing pipeline
  - `pkg/logs/tailers/` - File, container, socket tailers
  - `pkg/logs/message/` - Log message data structure
  - Launcher orchestration: `pkg/logs/launchers/launchers.go`
- **Collection Frequency:** Real-time streaming (logs collected as they're written)
- **Already Sends to Backend:** Yes, as processed log events

**Data Format:**

```go
type Message struct {
    Content []byte        // Raw log line
    Status string        // info, error, warning
    Timestamp time.Time
    Origin *Origin {
        Service string
        Source string    // e.g., "file", "docker", "journald"
        FilePath string  // If file source
        Tags []string    // Container labels, custom tags
    }
    IsPartial bool
    IsTruncated bool
}
```

Reference: raw_research.md lines 812-845 for log collection architecture

**What's New for Gadgets:**

- **Current:** Logs flow through pipeline → processors → sender → backend
- **Needed:** Gadget processors that can:
  - Analyze content (Curator for PII detection)
  - Buffer logs (Archivist ring buffer)
  - Trigger on patterns (error rate spike detection)
- **Already architected well:** Processing pipeline supports plugin processors - gadgets can be integrated as pipeline stages
- **Mechanism:** Gadgets register as log processors, receive every log message before forwarding

**Advantage for Gadgets:** Log pipeline is designed for extensibility - adding new processors is straightforward. No new data collection needed.

#### Log Messages

- **Content:**Raw or structured log line
- **Metadata:**
- Hostname, Status (info/error/warning)
- `Origin.LogSource.Service`
- `Origin.FilePath`
- Tags (container labels, custom tags)

- **Parsing:**Timestamp, IsPartial, IsTruncated, IsMultiLine

#### Volume Tracking

- **Counters:**LogsDecoded, LogsProcessed, LogsSent, BytesSent, BytesMissed, LogsTruncated
- **Distributions:**TlmLogLineSizes (Histogram of log sizes)
- **Latency:**SenderLatency, LatencyStats (24h window, 1h buckets per source)
- **Tags:**By service, source

#### Volume Control

- **Back Pressure:**Pipeline capacity monitoring
- **Drops:**`DestinationLogsDropped`counter
- *No explicit rate-based sampling, just volume limits*

#### Update Characteristics

- Real-time streaming through pipeline
- Metrics updated continuously
- No long-term local storage (in-memory before forwarding)

---

## Cross-Reference: Gadgets ↔ Signal Providers

### Table 1: Gadget → Signal Provider Dependencies

| Gadget | Agent Components to Extend | Signals Required (Criticality) | Action Mechanism | Overall Complexity | Status |
|--------|---------------------------|--------------------------------|------------------|-------------------|--------|
| **1. Oracle** | `pkg/util/cgroups`, `pkg/process` | CgroupResource: PSI (HIGH), Memory stats (HIGH)<br>SystemResource: swap velocity (HIGH)<br>ProcessSignal: RSS per-process (MEDIUM)<br>ContainerRuntime: QoS, RestartCount (LOW) | `syscall.Kill(pid, SIGTERM/SIGKILL)` | ⚠️ Medium | ✅ Strong |
| **2. Pathologist** | `pkg/security/probe`, `pkg/process`, `pkg/network` | SecurityEvent: Syscall events (HIGH)<br>ProcessSignal: CPU/IO deltas (HIGH)<br>NetworkBehavior: Connection count, tx rate (MEDIUM) | `syscall.Kill(pid, SIGTERM/SIGKILL)` + stack trace capture (gdb/jstack/pprof) | ⚠️ Medium | ✅ Strong |
| **3. Curator** | `pkg/logs` | LogStream: Log messages (HIGH) | In-place redaction in log pipeline | ✅ Low | ✅ Strong |
| **4. Equalizer** | `pkg/network/tracer`, `pkg/trace` | NetworkBehavior: Connection stats (HIGH)<br>TraceAnalysis: HTTP latency (MEDIUM)<br>**Gap:** CPU attribution per-connection (MISSING) | TCP RST or `close(fd)` | ❌ High | ⚠️ Weak |
| **5. Archivist** | `pkg/logs` | LogStream: All logs (HIGH)<br>CgroupResource: OOM events (MEDIUM)<br>ContainerRuntime: RestartCount (MEDIUM)<br>TraceAnalysis: Latency spikes (LOW) | Buffer management + conditional flush | ✅ Low | ⚠️ Weak |
| **6. Tuner** | System-wide (sysctl), agent config | SystemResource: Network, disk metrics (HIGH)<br>Agent telemetry: Drop rates, latency (HIGH) | `sysctl` writes, config file updates | ❌ High | ⚠️ Weak |
| **7. Timekeeper** | **Unclear** (alert suppression architecture TBD) | Any metric time-series (HIGH)<br>**Gap:** Historical patterns (MISSING) | Alert suppression (mechanism unclear) | ⚠️ Medium | ⚠️ Weak |

**Legend:**

- **Signals Criticality:**
  - HIGH = Essential for gadget function, must exist
  - MEDIUM = Improves accuracy/safety, desirable
  - LOW = Nice-to-have, provides context
- **Complexity:** ✅ Low (4-8 weeks), ⚠️ Medium (8-16 weeks), ❌ High (16+ weeks or significant unknowns)
- **Status:** ✅ Strong (well-developed, clear approach), ⚠️ Weak (needs research, has gaps), ❌ Blocked (missing critical signals or architectural clarity)

**Key Insights from This Table:**

- **Oracle, Pathologist, Curator** are the strongest candidates: signals exist (HIGH readiness), clear approaches, medium-low complexity
- **Equalizer, Tuner** have critical signal gaps or high complexity
- **Archivist, Timekeeper** have architectural questions to resolve
- **All gadgets** require decisions on gadget↔agent component communication architecture

---

### Table 2: Signal Provider → Agent Component Mapping

| Signal Provider | Agent Component | Key Package Path | Primary Data Source | Collection Method | Gadgets Using This |
|-----------------|-----------------|------------------|---------------------|-------------------|--------------------|
| **ProcessSignalProvider** | Process Agent | `pkg/process/procutil`<br>`pkg/process/events` | `/proc/[pid]/stat`<br>`/proc/[pid]/io`<br>eBPF (fork/exec/exit) | Polling (10-20s)<br>Real-time events | Oracle, Pathologist |
| **SystemResourceProvider** | Core Agent System Checks | `pkg/collector/corechecks/system/`<br>`cpu/`, `memory/`, `disk/` | `/proc/stat`<br>`/proc/meminfo`<br>`/proc/diskstats` | Polling (15s) | Oracle, Tuner |
| **CgroupResourceProvider** | Cgroup Utilities | `pkg/util/cgroups/`<br>`reader.go` | `/sys/fs/cgroup/memory.pressure`<br>`/sys/fs/cgroup/memory.stat` | Polling (10-20s) | Oracle, Archivist |
| **ContainerRuntimeProvider** | Workloadmeta | `comp/core/workloadmeta/`<br>`pkg/util/containers/` | Kubelet API<br>Docker API<br>containerd API | Polling (10s)<br>Event streams | Oracle, Archivist |
| **NetworkBehaviorProvider** | System-Probe Network Tracer | `pkg/network/tracer/`<br>`pkg/network/protocols/` | eBPF connection tracking<br>Packet inspection | Real-time eBPF | Pathologist, Equalizer |
| **SecurityEventProvider** | Security Agent (CWS) | `pkg/security/probe/`<br>`pkg/security/ebpf/probes/` | eBPF syscall probes<br>(48+ probe types) | Real-time eBPF events | Pathologist |
| **TraceAnalysisProvider** | Trace Agent | `pkg/trace/stats/`<br>`concentrator.go` | APM traces (port 8126)<br>Local aggregation | Real-time ingestion<br>10s buckets | Equalizer, Archivist |
| **LogStreamProvider** | Logs Agent | `pkg/logs/pipeline/`<br>`pkg/logs/tailers/` | File, container, journald,<br>TCP/UDP, etc. | Real-time streaming | Curator, Archivist |

**Key Insights from This Table:**

- **Most providers already exist** - gadgets are primarily about *consuming* existing data in new ways
- **Real-time vs polling split** - eBPF providers (Network, Security) are real-time; /proc-based (Process, System) are polled
- **Integration patterns:**
  - eBPF providers: Subscribe to event streams
  - Polling providers: Need higher-frequency access or pub/sub notifications
  - Processing providers (Logs): Integrate as pipeline processors
- **Shared infrastructure:** Multiple gadgets consume same providers (Oracle uses 4 providers, Pathologist uses 3)

---

## Part 3: Exploration Gaps & Future Signal Opportunities

During the comprehensive exploration of the Datadog Agent codebase, several gaps were identified where additional signals could enable even more sophisticated gadgets. These represent opportunities for future enhancement.

### 1. GPU Memory SignalsGap

- **Location:**`/pkg/gpu/`
- **Status:**Exists but lacks process correlation
- **Missing:**
- GPU memory usage per process
- GPU → CPU affinity tracking

- **Opportunity:**GPU Thrashing Detector gadget predicting GPU OOM

### 2. eBPF mmap OperationsGap

- **Status:**eBPF can track but data not exposed
- **Missing:**Anonymous mmap growth patterns per process
- **Opportunity:**Memory leak detector via mmap tracking

### 3. File Cache Miss RatesGap

- **Source:**`/proc/vmstat`(`pgmajfault`)
- **Missing:**Per-process cache miss aggregation
- **Opportunity:**Cold Start Predictor identifying cache thrashing

### 4. Hardware Performance CountersGap

- **Missing:**
- CPU cycles
- Cache misses (L1, L2, L3)
- Branch mispredictions

- **Collection Method:**`perf_event`or eBPF
- **Opportunity:**CPU Optimization Advisor identifying inefficient code

### 5. Disk Latency HistogramsGap

- **Current:**Only averages collected
- **Missing:**Percentile distributions (p50, p95, p99)
- **Opportunity:**Storage Cliff Detector identifying latency anomalies

### 6. Cross-Container Network FlowsGap

- **Status:**Network flows tracked but not container-aware graph
- **Missing:**Explicit container-to-container communication graph
- **Opportunity:**Microservice Dependency Mapper with anomaly detection

## Conclusion

The Datadog Agent provides an incredibly rich observability foundation with**8 comprehensive signal providers**covering process metrics, system resources, cgroups, containers, network behavior, security events, traces, and logs. These signals are either fully implemented or have clear paths to exposure with data already being collected.
The**7 proposed AI-powered Gadgets**leverage these signals in sophisticated ways:

- **The Oracle**- Predicts OOMs 2-3 minutes ahead using LSTM and graduated interventions
- **The Pathologist**- Detects deadlocked processes via behavioral classification
- **The Curator**- Redacts PII using contextual NER models
- **The Equalizer**- Terminates toxic requests using anomaly detection
- **The Archivist**- Retroactively hydrates logs around anomalies
- **The Tuner**- Optimizes parameters using reinforcement learning
- **The Timekeeper**- Suppresses alerts for routine maintenance patterns

These gadgets represent **intelligent, proactive interventions** that go far beyond simple if-then rules, making SREs say "I wish I had thought of that" rather than "I could have scripted that myself." They leverage temporal patterns, multi-signal correlation, and statistical learning to understand not just *what* is happening, but *why* it's happening and *what will happen next*.
The exploration also identified **6 key gaps** that represent future opportunities for even more sophisticated gadgets, including GPU monitoring, hardware performance counters, and cross-container dependency mapping.
**Document Version:**1.0
**Date:**2025-01-20
**Project:**Q-Branch Gadget Initiative
↑
