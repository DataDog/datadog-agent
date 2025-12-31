# Gadget Ideation Project - Context for Claude

## Project Purpose

This is a **research planning document** for a 6-8 month project leading to a demo at Datadog's Dash conference. The document explores AI-powered "Gadgets" - autonomous modules in the Datadog Agent that take **production-safe actions** based on observability signals to predict, prevent, and remediate infrastructure issues.

**Critical Requirement:** These gadgets MUST take actions (kill processes, drop caches, terminate connections, etc.). Monitoring/alerting-only solutions are explicitly out of scope. The goal is autonomous intervention with extensive safety mechanisms.

## Primary Audience

**The document's primary reader is the project lead** (Scott):
- **Background:** 10 years engineering experience, 5 years in Linux systems engineering
- **Role:** Leading team of 4-5 SWEs for 6-8 months
- **Expertise:** Strong in systems concepts (PSI, eBPF, syscalls, cgroups), minimal ML/AI experience
- **Need:** Make informed decisions about which gadgets to build, identify research gaps, understand ML trade-offs

**Secondary audiences:** The project team (similar systems background, minimal ML) and Datadog stakeholders reviewing progress.

## Document Goals

This document should help the reader:

1. **Identify strongest gadget candidates** - Which have solid signal foundations and clear approaches?
2. **Flag research gaps** - Which gadgets need more problem definition, approach validation, or technical investigation?
3. **Understand ML trade-offs** - Provide intuition for choosing between ML approaches (not prescriptions)
4. **Ground in existing agent code** - Show what signals are collectible TODAY from the Datadog Agent
5. **Assess feasibility** - What's achievable in 6-8 months with the team's skills and constraints?

**Important:** Uneven development across gadgets is USEFUL signal. Shorter, less developed sections explicitly flag "this needs more work" rather than hiding weakness.

## Project Constraints

### Resource Budget
- **CPU Overhead:** <20% per gadget (generous for demo/prototype environment)
- **Memory Overhead:** <200MB per gadget
- **Implication:** Can use moderately complex ML models (small LSTMs, Random Forests, quantized BERT). Not ultra-constrained.

### Team Skills
- **Strong:** Linux systems, eBPF, observability, agent architecture
- **Weak:** ML/AI algorithms, model training, inference optimization
- **Approach:** Use straightforward, well-documented ML techniques with existing libraries (sklearn, PyTorch tutorials). Avoid cutting-edge research or custom architectures.

### Timeline
- **Duration:** 6-8 months to Dash demo
- **Phases:** Proof of concept (signals accessible, actions safe) → ML integration → Production hardening
- **Risk:** Must ship working demo; can't rely on research breakthroughs

### Architecture
- **Deliberately ambiguous** - Not committing to agent-local vs backend intelligence yet
- **Focus:** What signals exist, what actions are possible, what intelligence is needed
- **Defer:** Where inference runs, where models are trained, how updates are deployed

## Core Philosophy: "AI Magic, Not Smart Scripting"

The goal is to make engineers say **"I wish I had thought of that"** not **"I could have scripted that myself."**

### What Makes a Good Gadget

✅ **Strong Gadget Characteristics:**
- **Non-obvious signal correlations** - Combines data sources in ways that aren't immediately apparent (PSI + memory velocity + cache eviction = OOM prediction)
- **Predictive, not just reactive** - Understands trajectories (velocity, acceleration) not just current state
- **Encodes expertise** - Automates what senior SREs do intuitively after years of experience
- **Handles ambiguity** - Distinguishes contexts that simple rules can't (deadlock vs long GC, burst vs leak)
- **Production-safe actions** - Takes autonomous action with extensive safety mechanisms

❌ **Weak Gadget Characteristics:**
- Just alerting/monitoring (doesn't meet action requirement)
- Simple threshold rules (could be scripted easily)
- Vague problem definition (can't validate if it works)
- Unclear signal requirements (can't verify signals exist)
- Overly complex ML for minimal team benefit

## Documentation Style Guide

### Writing Tone
- **Professional but compelling** - Not marketing hype, not dry academic prose
- **Concrete over abstract** - Scenarios and examples over generic descriptions
- **Educational without condescension** - Explain complex ideas clearly, assume intelligence
- **Explicit about gaps** - Flag unknowns and research needs openly

### Technical Depth: Systems vs ML

**For Systems Concepts (PSI, syscalls, eBPF, cgroups):**
- ✅ Information-dense, detailed explanations
- ✅ Prove technical viability with specifics
- ✅ Reference actual agent code when useful for disambiguation
- ✅ Connect to kernel interfaces (/proc, /sys, eBPF maps)
- ⚠️ File paths are useful but not required for every signal

**For ML/AI Concepts (models, algorithms, training):**
- ✅ Focus on **intuition** and **decision-making criteria**
- ✅ Present **options with trade-offs**, not prescriptions
- ✅ Explain **why** this approach (handles temporal patterns, learns baselines)
- ✅ Provide **learning resources** (PyTorch tutorials, sklearn docs)
- ❌ Don't prescribe specific architectures, hyperparameters, training procedures
- ❌ Don't assume reader knows ML terminology - explain or avoid jargon

## Gadget Documentation Structure

Each gadget must follow this structure:

1. **🎯 The Problem** - Visceral pain point, why traditional monitoring fails
2. **💡 The Breakthrough Insight** - The key discovery that makes the solution possible (e.g., "PSI is a leading indicator of OOM")
3. **🎭 The Magic Moment** - Concrete scenario showing it preventing/resolving an incident (narrative style)
4. **📊 Signal Requirements** - What data is needed, from which agent components
5. **🏗️ Intelligence Architecture** - Conceptual approach (not implementation details)
   - ML sections use decision framework approach (see template below)
6. **⚠️ Why AI is Required** - What makes this hard, why rules can't handle it
7. **🛡️ Safety Mechanisms** - How autonomous actions are made production-safe
8. **💰 Value Proposition** - MTTR reduction, cost savings, incidents prevented
9. **📋 Feasibility Assessment** - Structured assessment (see template below)
10. **🔍 Research Gaps** - Explicit about unknowns (see template below)

### Feasibility Assessment Structure

Every gadget must include this assessment:

```markdown
#### 📋 Feasibility Assessment

**Signal Readiness:** ✅ High / ⚠️ Medium / ❌ Low
- Do the required signals exist in the agent today?
- Reference specific agent components (pkg/*, raw_research.md)

**ML Complexity:** ✅ Straightforward / ⚠️ Moderate / ❌ Research Required
- Given minimal ML experience, how achievable is this?
- What straightforward approaches exist?

**Action Safety:** ✅ Low Risk / ⚠️ Moderate Risk / ❌ High Risk
- How dangerous are the autonomous actions?
- What safety mechanisms mitigate risk?

**Known Unknowns:**
- Specific gaps/questions that need research before implementation
- What needs validation before committing to this gadget?

**Timeline Estimate:**
- Rough estimate for proof of concept → MVP → production-safe
- Flags for long-lead-time items (training data collection, model tuning)
```

### ML Decision Framework Approach

ML sections must present options with trade-offs, not prescribe specific solutions. Use this structure:

```markdown
#### ML Approach Options

**The Problem:** [State the prediction/classification/detection problem clearly]

**Option 1: [Simplest/No ML]**
- Approach: [Simple rules or statistical methods]
- Pros: [Easy to implement, fast, interpretable]
- Cons: [Limitations]
- ML Experience Required: None
- Verdict: [When this is sufficient]

**Option 2: [Moderate Complexity]**
- Approach: [Classical ML or simple neural nets]
- Pros: [Better performance than Option 1]
- Cons: [More complex, requires training data]
- ML Experience Required: Basic (sklearn, simple PyTorch)
- Verdict: [Primary recommendation for minimal ML teams]

**Option 3: [Most Sophisticated]**
- Approach: [Deep learning, RL, transformers]
- Pros: [Best performance]
- Cons: [Complex, harder to debug, larger models]
- ML Experience Required: Advanced
- Verdict: [Fallback if Option 2 insufficient]

**Recommendation:** [Start with X, move to Y if needed]

**Learning Resources:**
- [Links to tutorials, papers, existing implementations]
```

**Rationale:** This approach helps teams with minimal ML experience make informed decisions rather than blindly following prescriptions. It shows trade-offs and provides learning paths.

### Research Gap Documentation

For gadgets with unclear approaches or insufficient development, include this section:

```markdown
#### ⚠️ Research Gaps

This gadget concept needs further development before implementation:

**Unclear Problem Definition:**
- [What's not well-defined about the problem space]
- [What production evidence is needed]

**Approach Uncertainty:**
- [Open questions about the technique]
- [Why is the proposed approach unclear or questionable]

**Validation Challenge:**
- [How to test this safely]
- [How to measure if it's working]

**Recommendation:** Needs [X weeks] of problem research and approach validation before coding starts.
```

**Rationale:** Explicit gap flagging helps project planning by identifying which gadgets need more research before implementation can begin.

## Signal Provider Documentation

### Required Components

Each signal provider section must include:

1. **💎 "So What?" Section** - What unique insights does this enable?
2. **🔗 Agent Integration Points** - Map to actual Datadog Agent components
3. **Correlation Goldmines** - Non-obvious signal combinations worth highlighting
4. **Educational Backgrounds** - Explain unfamiliar technical concepts accessibly

### Agent Integration Points Structure

For each signal provider, include this section:

```markdown
#### 🔗 Agent Integration Points

**Existing Collection:**
- Component: `pkg/[component]` ([Agent module name])
- Key Files: [Specific .go files] reads [kernel interfaces]
- Collection Frequency: [10s, 20s, real-time, etc.]
- Already Sends to Backend: [Yes/No, as what metric/event type]

**Data Format:**
- [Show actual structs or data shapes from agent code]
- [Reference raw_research.md line numbers if applicable]

**What's New for Gadgets:**
- Current: [What agent does today]
- Needed: [What gadgets require that's different]
- Mechanism: [How to expose to gadgets - ring buffer, pubsub, API, etc.]
```

**Source:** Use `raw_research.md` to ground each provider in actual agent code. Show what data collection exists TODAY.

## Key Themes to Emphasize

1. **Multi-dimensional correlation is the breakthrough** - Single signals are just noise; combined patterns tell stories
2. **Leading indicators over lagging** - PSI predicts OOM minutes ahead (stall time > usage)
3. **Behavioral fingerprints** - Syscall entropy reveals deadlocks, connection patterns reveal toxic queries
4. **Temporal context matters** - Velocity and acceleration, not just current values
5. **Context-dependent classification** - What's "normal" varies by process, time, workload
6. **Production-safe autonomy** - Actions with extensive safety mechanisms (cooldowns, whitelists, confidence thresholds, rollback)

## Root Cause Investigation Mindset

The project embodies: **"Favor root cause investigations over fixing end effects"**

When proposing gadgets:
- Don't just detect symptoms (high CPU) - understand causes (deadlock with futex spam)
- Don't just alert - predict and prevent (OOM forecasting with graduated intervention)
- Don't just react - learn patterns (PII detection that evolves with new formats)

## Explicit Gap Flagging Philosophy

**Critical:** Don't hide weaknesses or research gaps. Flag them explicitly so the project lead knows what needs more work.

Uneven development across gadgets is USEFUL signal - shorter, less developed sections indicate areas that need more research or validation before implementation.

## Documentation Value System

### ✅ DO Emphasize

- **Breakthrough insights** - The "aha!" moment that makes each idea possible (PSI as leading indicator, syscall entropy as liveness measure)
- **"So What?" factor** - Why this capability matters, what it enables that wasn't possible before
- **Compelling narratives** - "Magic moment" scenarios showing real-world impact (3AM crisis averted, audit prevented)
- **Multi-signal correlation** - The specific signal combinations that reveal invisible patterns
- **Why AI is required** - Clear articulation of why ML/AI is needed, not just if-then rules
- **Concrete value** - MTTR reduction, cost savings, incidents prevented
- **Educational backgrounds** - Explain complex concepts (PSI, syscall entropy, DDSketch) accessibly for systems engineers
- **Actual agent code** - Ground in what exists today (pkg/* references, raw_research.md)
- **Feasibility assessment** - Explicit about what's strong, what's weak, what's unknown
- **ML trade-offs** - Help reader choose between approaches, don't prescribe
- **Safety mechanisms** - How autonomous actions are made production-safe

### ❌ DON'T Include

- **Implementation minutiae** - Exact model architectures, hyperparameters, training procedures
- **Speculative performance numbers** - Metrics we haven't measured yet (unless clearly marked as targets)
- **Detailed configurations** - Parameters that will evolve through experimentation
- **Technology stack details** - "Should we use TensorFlow or PyTorch?" level decisions
- **ML prescriptions** - "Use LSTM with 128 hidden units" - instead explain why LSTMs handle sequences
- **False precision** - Don't overspecify if it's not validated yet
- **Hidden weaknesses** - If a gadget is underdeveloped, FLAG IT, don't pretend it's complete

## Validation Requirements

### Proof of Technical Viability

The document must demonstrate:

1. **Data Exists:** Required signals are collectible from the Datadog Agent today
   - Reference specific agent components (pkg/util/cgroups, pkg/security/probe)
   - Show collection mechanisms (/proc/[pid]/stat, eBPF maps, existing checks)
   - Use raw_research.md as evidence

2. **ML is Feasible:** AI/ML approaches can run in agent given resource constraints
   - Model size estimates (50-200MB within budget)
   - Inference latency targets (<100ms typically acceptable)
   - On-device ML viability (quantization, ONNX, TFLite)
   - Prove it's not "too expensive to run"

3. **Actions are Safe:** Autonomous interventions won't cause more harm than good
   - Safety mechanisms (whitelists, cooldowns, confidence thresholds)
   - Graduated responses (warn → gentle action → aggressive action)
   - Rollback strategies
   - Failure modes considered

4. **Value is Clear:** Compelling scenarios show impact
   - 3AM OOM crisis prevented
   - GDPR audit finding avoided
   - Deadlock detected in 3 minutes vs 30 minutes
   - Quantified improvements (MTTR reduction, incidents prevented)

## Cross-Reference Requirements

### Gadget → Signal Provider Mapping

Include a comprehensive table:

| Gadget | Agent Components to Extend | Signals Required | Action Mechanism | Complexity | Status |
|--------|---------------------------|------------------|------------------|------------|--------|
| Oracle | `pkg/util/cgroups` | PSI (HIGH), Memory stats (HIGH) | `syscall.Kill()` | Medium | Strong |
| Pathologist | `pkg/security/probe`, `pkg/process` | Syscall events (HIGH), CPU/IO (HIGH) | `syscall.Kill()` | Medium | Strong |
| ... | ... | ... | ... | ... | ... |

**Legend:**
- **Signals:** HIGH = exists today, MEDIUM = needs exposure work, LOW = might not exist
- **Complexity:** Low/Medium/High for 6-8 month timeline
- **Status:** Strong (well-developed) / Weak (needs research) / Gap (significant unknowns)

### Signal Provider → Agent Code Mapping

For each provider, show:
- Abstract name (e.g., "ProcessSignalProvider")
- Actual agent components (e.g., `pkg/process/procutil`)
- Key files and kernel interfaces
- What data is available TODAY

This grounds the document in verifiable reality.

## Practical Usage Notes

### For the Project Lead (Primary Reader)

**Use this document to:**
1. Quickly identify which gadgets have strongest technical foundation (Signal Readiness: ✅)
2. Spot research gaps that need investigation before coding (Research Gaps sections)
3. Make informed ML approach decisions (ML Decision Framework sections)
4. Understand what agent components the team needs to modify (Agent Integration Points)
5. Assess 6-8 month feasibility (Timeline Estimates, Complexity ratings)

**Red flags to watch for:**
- ML Complexity: ❌ Research Required → High risk for minimal ML team
- Signal Readiness: ❌ Low → Can't build without new instrumentation
- Action Safety: ❌ High Risk → Needs extensive safety validation
- Long Research Gaps section → Concept needs more problem definition

**Green flags to look for:**
- Signal Readiness: ✅ High + references to pkg/* code → Can start immediately
- ML Complexity: ✅ Straightforward with clear learning resources → Team can handle
- Action Safety: ✅ Low Risk or ⚠️ Moderate with clear safety mechanisms → Shippable
- Strong "Magic Moment" narratives + explicit value → Compelling demo

### For Document Editors

**When adding/revising gadgets:**
1. Present ML options with trade-offs, not prescriptions
2. Ground signals in actual agent code (reference raw_research.md)
3. Include Feasibility Assessment and Research Gaps sections
4. If something is unclear or uncertain - state it explicitly
5. Systems concepts: detailed and technical; ML concepts: intuitive and educational
6. Safety mechanisms are required (actions are the core requirement)

**When something is speculative:**
- Mark it clearly: "This assumes..." or "Requires validation that..."
- Include in Research Gaps section
- Don't present unvalidated ideas as established approaches

## Build System

- **Edit:** `content.md` (main document source)
- **Build:** `python3 build.py` generates standalone `index.html`
- **Technical:** Pandoc converts markdown to HTML with `--toc --toc-depth=3`, CSS from `style.css` embedded inline
- **Output:** Single self-contained HTML file ready for sharing

## Reference Materials

- **raw_research.md:** Comprehensive catalog of Datadog Agent data sources, compiled from actual codebase exploration. Use this to ground signal providers in reality.
- **Datadog Agent repo:** `/Users/scott.opell/dev/alt-datadog-agent/` - Primary source for what's possible today

## Tool Use Rules

- **Playwright Screenshot:** ALWAYS use jpeg as filetype, NEVER png
- **File References:** Use absolute paths when referencing agent code
- **Cross-references:** When mentioning agent components, use `pkg/*` notation consistently
