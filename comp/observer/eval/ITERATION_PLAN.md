# Observer: Agentic Iteration Plan Template

## Goal
Implement and evaluate detection/correlation algorithms, test across scenarios, and iterate to improve diagnosis accuracy.

---

## Ground Truths

Ground truths define what the LLM diagnosis should identify for each scenario. Each entry contains:
- **Problem type:** Classification of the failure mode
- **Fault:** What was injected/broken
- **Mechanism:** How the fault manifests in the system
- **Root cause:** The actual cause an LLM should identify
- **Correct keywords:** Terms that indicate correct diagnosis
- **Partial keywords:** Terms that show partial understanding

### Adding New Scenarios

Add entries to `evaluate_diagnosis.py` with:
```python
PROBLEM_TYPES["scenario-name"] = "problem type"
GROUND_TRUTHS["scenario-name"] = {
    "correct_keywords": [...],
    "partial_keywords": [...],
    "root_cause": "description"
}
```

---

## Train/Test Split (Avoid Overfitting)

Split scenarios into:
- **TRAIN SET:** Develop and tune algorithms on these
- **TEST SET:** Final evaluation only (never tune on these)

Recommended split: 60% train, 40% test.

---

## Agentic Iteration Loop

```
+---------------------------------------------------------------------+
|                     AGENTIC ITERATION LOOP                          |
+---------------------------------------------------------------------+
|                                                                     |
|  STEP 1: IMPLEMENT                                                  |
|    - Write new detector/correlator in comp/observer/impl/           |
|    - Add CLI flags to cmd/observer-demo-v2/main.go                  |
|    - Wire up in demo_main_v2.go                                     |
|                                                                     |
|  STEP 2: BUILD                                                      |
|    - go build -o bin/observer-demo-v2 ./cmd/observer-demo-v2        |
|                                                                     |
|  STEP 3: TEST ON ONE SCENARIO                                       |
|    - ./bin/observer-demo-v2 --parquet <train_scenario> \            |
|         --output test.json --<detector> --<correlator> --all        |
|    - python3 analyze_with_llm.py test.json                          |
|    - python3 evaluate_diagnosis.py diagnosis.txt --scenario <name>  |
|                                                                     |
|  STEP 4: ANALYZE FAILURE                                            |
|    - If score < 50: Read diagnosis, identify missing evidence       |
|    - Check: What signals exist? What correlations found?            |
|    - Determine: What evidence would help LLM diagnose?              |
|                                                                     |
|  STEP 5: IMPROVE ALGORITHM                                          |
|    - Adjust algorithm logic based on failure analysis               |
|    - Go back to STEP 1                                              |
|                                                                     |
|  STEP 6: CROSS-VALIDATE (when single scenario works)                |
|    - Test on ALL train scenarios                                    |
|    - Compute average score                                          |
|    - If avg < 50: identify failing scenarios, go to STEP 4          |
|                                                                     |
|  STEP 7: FINAL EVAL (when train avg > target)                       |
|    - Test on held-out TEST scenarios                                |
|    - Report generalization performance                              |
|                                                                     |
+---------------------------------------------------------------------+
```

---

## Algorithm Types

### Detectors (Layer 1)
Identify anomalies in individual time series.

Approaches:
- Change-point detection (e.g., CUSUM)
- Statistical outlier detection (e.g., ESD, Z-score)
- Bayesian methods (e.g., edge frequency learning)

### Correlators (Layer 2)
Group/relate anomalies to provide context for diagnosis.

Approaches:
- Temporal clustering (group by time proximity)
- Causal/lead-lag detection (A leads B by N seconds)
- Co-occurrence analysis (lift/support metrics)
- Graph-based methods (edge frequency learning)

### Pre-processing
- Deduplication (reduce volume before correlation)
- Filtering (remove noise, low-confidence anomalies)

---

## Evaluation Commands

### Single Scenario Test
```bash
# Build
go build -o bin/observer-demo-v2 ./cmd/observer-demo-v2

# Run with chosen detector + correlator
./bin/observer-demo-v2 \
    --parquet <scenario>.parquet \
    --output /tmp/test.json \
    --<detector-flag> \
    --<correlator-flag> \
    --all

# Diagnose
python3 comp/observer/eval/analyze_with_llm.py /tmp/test.json > /tmp/diagnosis.txt

# Grade
python3 comp/observer/eval/evaluate_diagnosis.py /tmp/diagnosis.txt --scenario <scenario-name>
```

### Multi-Scenario Batch
```bash
for scenario in <train-scenarios>; do
    ./bin/observer-demo-v2 --parquet "${scenario}.parquet" \
        --output "/tmp/${scenario}.json" --<flags> --all
    python3 analyze_with_llm.py "/tmp/${scenario}.json" > "/tmp/${scenario}_diag.txt"
    echo "=== $scenario ==="
    python3 evaluate_diagnosis.py "/tmp/${scenario}_diag.txt" --scenario "$scenario"
done
```

---

## Success Criteria

### Per-Scenario Targets
Define minimum acceptable scores for each scenario based on:
- Difficulty (some scenarios have less observable signal)
- Baseline performance (what untuned algorithms achieve)

### Generalization Target
Test set average should be within 20% of train set average. Large gaps indicate overfitting.

---

## Experimental Best Practices

### Evaluation Matrix Rules

1. **Run complete evaluation matrices** - Always test all configs on all scenarios. Never leave cells empty.

2. **No selective/sparse evaluation** - Don't skip running a config because it "seems unlikely to work."

3. **Ask before skipping** - If something truly seems fruitless, raise the concern first.

4. **Document all results** - Even negative results (low scores) are valuable data.

5. **Tuning must be exhaustive** - If tuning one config, tune all configs for fair comparison.

### Data Integrity

6. **Test set is sacred** - Never tune on test set. Only final evaluation.

7. **Train/test split is fixed** - Don't cherry-pick scenarios between sets based on results.

8. **Preserve all artifacts** - Keep JSON outputs and diagnosis files for reproducibility.

### Communication

9. **Report uncertainty** - Flag anomalous results and investigate before moving on.

10. **No silent omissions** - Report explicitly if you can't run something.

---

## Files Summary

### Files to CREATE (New Algorithms)
| File Pattern | Purpose |
|--------------|---------|
| `comp/observer/impl/anomaly_<name>.go` | New detector implementation |
| `comp/observer/impl/anomaly_processor_<name>.go` | New correlator implementation |

### Files to MODIFY (Integration)
| File | Changes |
|------|---------|
| `cmd/observer-demo-v2/main.go` | Add CLI flags for new algorithms |
| `comp/observer/impl/demo_main_v2.go` | Add config fields, instantiate algorithms |
| `evaluate_diagnosis.py` | Add new ground truth definitions |

### Files NOT to Touch
Existing working algorithms should not be modified when adding new ones.
