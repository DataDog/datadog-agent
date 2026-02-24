# Observer Evaluation Framework

This directory contains tools and documentation for evaluating and tuning observer anomaly detection and correlation algorithms.

## Overview

The evaluation framework enables systematic testing of algorithm configurations against labeled scenarios, using an LLM (GPT-4) to generate diagnoses and grade them against ground truth.

## Quick Start

### Prerequisites

1. **Build the observer binary:**
   ```bash
   go build -o bin/observer-demo-v2 ./cmd/observer-demo-v2
   ```

2. **Set up environment:**
   ```bash
   export OPENAI_API_KEY=your_key_here
   ```

3. **Get test data:**
   Parquet files with labeled scenarios should be placed in a `data/` directory.

### Running a Single Evaluation

```bash
# Run observer on a scenario
./bin/observer-demo-v2 \
    --parquet data/<scenario>.parquet \
    --output /tmp/output.json \
    --<detector-flag> \
    --<correlator-flag> \
    --rca \
    --all

# Generate LLM diagnosis
python3 comp/observer/eval/analyze_with_llm.py /tmp/output.json > /tmp/diagnosis.txt

# Grade the diagnosis
python3 comp/observer/eval/evaluate_diagnosis.py /tmp/diagnosis.txt --scenario <scenario-name>
```

### Running Bayesian Optimization (Optuna)

```bash
python3 comp/observer/eval/harness.py \
    --scenario <scenario-name> \
    --detector <detector> \
    --correlator <correlator> \
    --trials 10
```

## Files

| File | Purpose |
|------|---------|
| `analyze_with_llm.py` | Sends observer output to GPT-4 for root cause diagnosis |
| `evaluate_diagnosis.py` | Grades diagnosis against ground truth (0-100 score) |
| `harness.py` | Optuna-based Bayesian optimization for parameter tuning |

`output.json` keeps existing fields (`sample_anomalies`, `correlations`, `edges`) and may include additive `rca` evidence when `--rca` is enabled.

### Documentation

| File | Purpose |
|------|---------|
| `ITERATION_PLAN.md` | Generic methodology for iterative algorithm development |
| `TUNING_PLAN.md` | Generic parameter tuning methodology |
| `CORRELATION_PLAN.md` | Algorithm design guide and patterns |
| `EXAMPLE_CONTEXT.md` | Specific implementations, ground truths, and results |

## Evaluation Pipeline

```
+-----------------+     +------------------+     +-----------------+
|  Parquet Data   |---->|  observer-demo-v2 |---->|  output.json    |
|  (scenarios)    |     |  (algorithms)     |     |  (anomalies)    |
+-----------------+     +------------------+     +--------+--------+
                                                          |
                        +------------------+              |
                        |  GPT-4 Grader    |<-------------+
                        |  (0-100 score)   |
                        +--------+---------+
                                 |
                        +--------v---------+
                        |  evaluate_       |
                        |  diagnosis.py    |
                        +------------------+
```

## Workflow for AI Assistants

When asked to evaluate or tune observer algorithms:

1. **Read `EXAMPLE_CONTEXT.md`** for scenario ground truths and current implementations
2. **Follow `ITERATION_PLAN.md`** for the iteration methodology
3. **Use `TUNING_PLAN.md`** for parameter tuning approach
4. **Reference `CORRELATION_PLAN.md`** when designing new algorithms

### Key Principles

1. **Train/test split is sacred** - Never tune on test set
2. **Run complete matrices** - Test all configs on all scenarios
3. **Document all results** - Even negative results are valuable
4. **Evidence over decisions** - Correlators surface evidence, LLM interprets

## Adding New Scenarios

1. Define ground truth in `EXAMPLE_CONTEXT.md`
2. Add to `evaluate_diagnosis.py`:
   ```python
   PROBLEM_TYPES["new-scenario"] = "problem type"
   GROUND_TRUTHS["new-scenario"] = {...}
   ```
3. Place parquet file in data directory
4. Run evaluation to establish baseline

## Adding New Algorithms

1. Implement in `comp/observer/impl/`
2. Add CLI flags to `cmd/observer-demo-v2/main.go`
3. Wire up in `demo_main_v2.go`
4. Document parameters in `EXAMPLE_CONTEXT.md`
5. Run evaluation matrix
