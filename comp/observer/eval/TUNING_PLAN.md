# Observer: Algorithm Tuning Methodology

## Goal

Systematically tune algorithm parameters to optimize diagnosis accuracy across scenarios.

---

## Tuning Cycle

```
+-----------------------------------------------------------+
|  tune params -> run replay -> LLM diagnosis -> LLM score  |
+-----------------------------------------------------------+
```

Each iteration:
1. Sample parameters from search space
2. Run observer with those parameters
3. Generate LLM diagnosis
4. Grade diagnosis against ground truth
5. Update parameter search based on score

---

## Tuning Infrastructure

### Config Structure

Define tunable parameters in a config struct:

```go
type TuningConfig struct {
    // Detector parameters
    DetectorParam1 float64
    DetectorParam2 int

    // Correlator parameters
    CorrelatorParam1 float64
    CorrelatorParam2 int

    // Pre-processing parameters
    PreprocessParam1 int
}
```

### CLI Flags

Add parameter override flags to the demo CLI:

```go
flag.Float64Var(&config.DetectorParam1, "detector-param1", 0, "override default")
```

### Tuning Harness (Optuna)

```python
def objective(trial: optuna.Trial) -> float:
    # 1. Sample parameters
    params = {
        "detector_param1": trial.suggest_float("param1", min, max),
        "detector_param2": trial.suggest_int("param2", min, max),
    }

    # 2. Run observer with params
    output_json = run_observer(params)

    # 3. Generate diagnosis
    diagnosis = call_llm(output_json)

    # 4. Grade diagnosis
    score = grade_diagnosis(diagnosis, scenario)

    return score
```

---

## Parameter Search Spaces

### Defining Search Ranges

For each tunable parameter, define:

| Parameter | Default | Range | Scale | Notes |
|-----------|---------|-------|-------|-------|
| param_name | default_value | [min, max] | linear/log | explanation |

**Scale guidance:**
- Use `linear` for parameters where small changes matter equally throughout range
- Use `log` for parameters spanning orders of magnitude (e.g., 0.001 to 0.1)

### Trial Budget

Recommended trials per configuration:
- Quick exploration: 10-20 trials
- Thorough tuning: 30-50 trials
- Exhaustive search: 100+ trials

---

## Experimental Design

### Single-Scenario Tuning

Tune on one scenario to find good parameter regions:

```bash
python3 harness.py \
    --scenario <scenario-name> \
    --detector <detector> \
    --correlator <correlator> \
    --trials 30
```

### Cross-Scenario Validation

After single-scenario tuning, validate on all train scenarios:

```bash
for scenario in <all-train-scenarios>; do
    python3 harness.py --scenario $scenario --trials 10
done
```

### Aggregate Optimization

Optimize for average score across all train scenarios:

```python
def multi_scenario_objective(trial):
    scores = []
    for scenario in train_scenarios:
        score = single_scenario_objective(trial, scenario)
        scores.append(score)
    return sum(scores) / len(scores)
```

---

## Results Analysis

### Per-Configuration Matrix

Record results in a matrix:

| Config | Scenario1 | Scenario2 | ... | Average |
|--------|-----------|-----------|-----|---------|
| Config_A | score | score | ... | avg |
| Config_B | score | score | ... | avg |

### Parameter Importance

Use Optuna's importance analysis:

```python
importance = optuna.importance.get_param_importances(study)
```

### Visualization

Plot optimization history:

```python
optuna.visualization.plot_optimization_history(study)
optuna.visualization.plot_param_importances(study)
```

---

## Common Issues

### Overfitting to Train Set

**Symptom:** High train scores, low test scores

**Solutions:**
- Use cross-validation within train set
- Regularize toward default parameter values
- Early stopping based on validation performance

### Parameter Interactions

**Symptom:** Single-parameter sweeps don't improve, but combinations do

**Solutions:**
- Use multi-dimensional search (Optuna handles this)
- Look for correlated parameters
- Consider joint parameter constraints

### Unstable Scores

**Symptom:** Same config gives different scores on re-runs

**Causes:**
- LLM non-determinism (temperature > 0)
- Random components in algorithm

**Solutions:**
- Average over multiple runs
- Set LLM temperature to 0
- Fix random seeds for reproducibility

---

## Output Artifacts

### Per-Tuning-Run

- `params.json` - Parameter values used
- `output.json` - Observer output
- `diagnosis.txt` - LLM diagnosis
- `score.txt` - Graded score

### Per-Study

- `study.db` - Optuna study database
- `results.csv` - All trials with parameters and scores
- `best_params.json` - Best parameter configuration found

---

## Tuning Workflow Summary

1. **Baseline:** Run with default parameters, record scores
2. **Single-scenario:** Tune on hardest scenario first
3. **Cross-validate:** Check tuned params on all train scenarios
4. **Multi-scenario:** Optimize for average if needed
5. **Final eval:** Test on held-out test set
6. **Document:** Record best params and rationale
