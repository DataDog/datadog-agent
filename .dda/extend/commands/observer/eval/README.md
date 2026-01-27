# Observer Anomaly Detection Evaluation Framework

## Overview

The Observer Evaluation Framework provides tools for evaluating anomaly detection performance using synthetic and real-world data. It implements:

- **Peaks-Over-Threshold (POT)** thresholding using Extreme Value Theory
- **UCR Score** for top-1 anomaly ranking
- **Point-Adjusted F1** for incident detection accuracy
- **AUC-ROC** for overall discrimination capability

## Installation

The framework requires the following Python dependencies (auto-installed by dda):

```bash
pip install pandas numpy scipy scikit-learn pyarrow matplotlib
```

## Quick Start

### 1. Generate Synthetic Test Data

```bash
python .dda/extend/commands/observer/eval/tests/generate_synthetic.py \
  --output-dir .dda/extend/commands/observer/eval/tests/fixtures
```

This generates five test scenarios:
- `simple_incident` - Single incident with clear signal (good detector)
- `multi_metric` - Multiple correlated metrics
- `no_incident` - Baseline data without anomalies
- `multi_incident` - Two separate incidents
- `bad_detector` - Inverted scores (poor detector - validates framework can detect bad performance)

### 2. Run Evaluation

```bash
dda observer eval \
  --raw-metrics .dda/extend/commands/observer/eval/tests/fixtures/simple_incident_metrics.parquet \
  --findings .dda/extend/commands/observer/eval/tests/fixtures/simple_incident_findings.parquet \
  --output results.json \
  --plot results.png \
  --verbose
```

### 3. View Results

**JSON Output** (`results.json`):
```json
{
  "UCR_Score": 1,
  "Adjusted_F1": 0.95,
  "AUC_ROC": 0.98,
  "Computed_Threshold": 0.75,
  "Total_Anomalies_Found": 18,
  "Precision": 0.94,
  "Recall": 0.96
}
```

**Visualization** (`results.png`):
- Panel 1: Raw metric values with ground truth windows (green shading)
- Panel 2: Anomaly scores with POT threshold and predictions
- Panel 3: Binary comparison heatmap (ground truth vs predictions)

## Data Format

### Input: `raw_metrics.parquet`

Contains all metrics including the special `observer.incident` metric for ground truth:

| Column | Type | Description |
|--------|------|-------------|
| `timestamp` | int64 | Unix timestamp (seconds) |
| `metric_name` | string | Metric name (e.g., "heap.used_mb", "observer.incident") |
| `value` | float64 | Metric value |
| `tags` | string | JSON array of tags (e.g., '["host:demo"]') |

**Ground Truth Format**:
- The `observer.incident` metric marks incident boundaries:
  - `value=1.0` at incident **start** timestamp
  - `value=0.0` at incident **end** timestamp
  - Absent/null at non-incident timestamps

**Example**:
```
timestamp | metric_name         | value | tags
----------|---------------------|-------|------
1000      | heap.used_mb        | 512   | ["host:demo"]
1022      | observer.incident   | 1.0   | []          # INCIDENT START
1022      | heap.used_mb        | 900   | ["host:demo"]
1050      | observer.incident   | 0.0   | []          # INCIDENT END
1050      | heap.used_mb        | 512   | ["host:demo"]
```

### Input: `findings.parquet`

Contains anomaly detector outputs:

| Column | Type | Description |
|--------|------|-------------|
| `timestamp` | int64 | Unix timestamp (seconds) |
| `anomaly_score` | float64 | Anomaly score (0.0 to 1.0) |

The framework performs an inner join on `timestamp` to align metrics with findings.

## Command Reference

### `dda observer eval`

Evaluate anomaly detection performance.

**Required Arguments**:
- `--raw-metrics PATH` - Path to raw_metrics.parquet file
- `--findings PATH` - Path to findings.parquet file

**Optional Arguments**:
- `--metric-name NAME` - Metric to evaluate (default: "heap.used_mb")
- `--output PATH` - Save JSON results to file (default: stdout)
- `--plot PATH` - Save visualization plot to file
- `--q FLOAT` - POT probability parameter (default: 1e-4)
- `--initial-percentile FLOAT` - Initial threshold percentile (default: 98.0)
- `--verbose` - Print debug information

**Example**:
```bash
dda observer eval \
  --raw-metrics data/metrics.parquet \
  --findings data/findings.parquet \
  --metric-name heap.used_mb \
  --output results.json \
  --plot results.png \
  --q 1e-4 \
  --initial-percentile 98.0 \
  --verbose
```

## Evaluation Metrics

### UCR Score (Top-1 Accuracy)

**Definition**: `1` if the maximum anomaly score occurs within any ground truth window, `0` otherwise.

**Use Case**: Measures if the detector identifies the most anomalous point correctly.

**Interpretation**:
- `1.0` = Max score is in an incident window (good)
- `0.0` = Max score is outside all windows (bad)

### Adjusted F1 Score

**Definition**: F1 score after applying point-adjustment:
- If ANY point in a ground truth window is detected, mark the ENTIRE window as detected

**Use Case**: Measures incident detection accuracy with temporal tolerance.

**Interpretation**:
- `1.0` = All incidents detected with no false positives (perfect)
- `0.0` = No incidents detected or all predictions are false (poor)
- `0.5-0.8` = Partial detection with some errors (acceptable)

### AUC-ROC

**Definition**: Area Under the Receiver Operating Characteristic curve.

**Use Case**: Measures overall discrimination capability across all threshold values.

**Interpretation**:
- `1.0` = Perfect separation (ideal)
- `0.5` = Random guessing (useless)
- `< 0.5` = Worse than random (inverted)

### POT Threshold

**Method**: Peaks-Over-Threshold using Generalized Pareto Distribution (GPD).

**Equation**:
```
th = u + (sigma/xi) * ((n*q/n_u)^(-xi) - 1)

where:
  u     = initial threshold (percentile)
  sigma = GPD scale parameter
  xi    = GPD shape parameter
  n     = total data points
  n_u   = peaks above u
  q     = probability parameter (e.g., 1e-4)
```

**Use Case**: Automatically compute a statistically-principled threshold for anomaly detection.

## Algorithm Details

### Phase 1: Data Ingestion
1. Load `raw_metrics.parquet` and `findings.parquet`
2. Extract incident windows from `observer.incident` metric
3. Align metrics with findings via inner join on `timestamp`
4. Build binary ground truth vector (`y_true`)

### Phase 2: POT Thresholding
1. Compute initial threshold as the `initial_percentile`-th percentile (default: 98th)
2. Extract peaks (exceedances) above initial threshold
3. Fit Generalized Pareto Distribution (GPD) to exceedances
4. Compute POT threshold using Equation 2
5. Apply threshold to get binary predictions (`y_pred_raw`)

### Phase 3: Metrics Calculation
1. **UCR Score**: Check if max score is in any window
2. **Point-Adjustment**: If any point in a window is detected, mark entire window
3. **Adjusted F1**: Calculate F1 on adjusted predictions
4. **AUC-ROC**: Calculate area under ROC curve using raw scores

### Phase 4: Output & Visualization
1. Generate JSON summary with all metrics
2. Generate 3-panel plot (raw metric, scores, comparison)
3. Print human-readable summary to console

## Testing

### Generate Test Data

```bash
python .dda/extend/commands/observer/eval/tests/generate_synthetic.py
```

Generates five scenarios in `.dda/extend/commands/observer/eval/tests/fixtures/`:
- `simple_incident` - Basic test case (good detector)
- `multi_metric` - Multiple correlated metrics
- `no_incident` - Baseline without anomalies
- `multi_incident` - Two separate incidents
- `bad_detector` - Inverted scores (validates framework detects bad performance)

### Run Unit Tests

```bash
pytest .dda/extend/commands/observer/eval/tests/
```

Tests cover:
- Data loading and alignment
- Incident window extraction
- POT thresholding
- Metrics calculation
- Edge cases (no ground truth, no predictions, etc.)

### Integration Tests

```bash
# Test on simple incident (should get UCR=1, F1>0.9)
dda observer eval \
  --raw-metrics .dda/extend/commands/observer/eval/tests/fixtures/simple_incident_metrics.parquet \
  --findings .dda/extend/commands/observer/eval/tests/fixtures/simple_incident_findings.parquet \
  --verbose

# Test on no incident (should get UCR=0 if scores are low)
dda observer eval \
  --raw-metrics .dda/extend/commands/observer/eval/tests/fixtures/no_incident_metrics.parquet \
  --findings .dda/extend/commands/observer/eval/tests/fixtures/no_incident_findings.parquet \
  --verbose

# Test on bad detector (should get UCR=0, F1≈0, AUC≈0)
dda observer eval \
  --raw-metrics .dda/extend/commands/observer/eval/tests/fixtures/bad_detector_metrics.parquet \
  --findings .dda/extend/commands/observer/eval/tests/fixtures/bad_detector_findings.parquet \
  --verbose
```

### Validation: Good vs Bad Detector

The framework correctly distinguishes between good and bad detectors:

**Good Detector** (`simple_incident`):
```json
{
  "UCR_Score": 1.0,        // Max score in incident window
  "Adjusted_F1": 1.0,      // Perfect detection
  "AUC_ROC": 0.9795,       // Excellent discrimination
  "Precision": 1.0,
  "Recall": 1.0
}
```

**Bad Detector** (`bad_detector` - inverted scores):
```json
{
  "UCR_Score": 0.0,        // Max score outside window
  "Adjusted_F1": 0.0,      // No correct detections
  "AUC_ROC": 0.003,        // Worse than random
  "Precision": 0.0,
  "Recall": 0.0
}
```

The bad detector scenario uses **inverted anomaly scores** (high when normal, low during incident) to validate that the framework can detect poor performance. See `tests/BAD_DETECTOR_TEST.md` for detailed analysis.

## Troubleshooting

### Error: "No data found for metric 'X'"

**Cause**: The specified metric name doesn't exist in `raw_metrics.parquet`.

**Solution**: Check available metrics:
```python
import pandas as pd
df = pd.read_parquet("raw_metrics.parquet")
print(df['metric_name'].unique())
```

### Error: "No overlapping timestamps"

**Cause**: Timestamps in `raw_metrics` and `findings` don't align.

**Solution**: Ensure both files have matching timestamps (inner join is used).

### Warning: "No ground truth windows"

**Cause**: No `observer.incident` metric found in `raw_metrics.parquet`.

**Solution**:
- If evaluating real data without ground truth, UCR score will be 0
- If ground truth should exist, verify `observer.incident` metric is present

### Error: "anomaly_score contains NaN values"

**Cause**: Missing or invalid anomaly scores in `findings.parquet`.

**Solution**: Clean the data to ensure all `anomaly_score` values are valid floats.

### Warning: "GPD fitting failed"

**Cause**: Insufficient peaks or numerical issues in GPD fitting.

**Solution**: Framework automatically falls back to percentile threshold. Consider:
- Lowering `--initial-percentile` to get more peaks
- Checking if scores have sufficient variance

## Future Work (Phase 2)

Phase 2 will integrate with the Go backend:

1. **Add `observer.incident` metric to testbench demo generator**
   - Emit incident markers at scenario boundaries

2. **Create parquet export API in testbench**
   - `POST /api/export` endpoint
   - Exports metrics and findings to parquet files

3. **End-to-end workflow**
   ```bash
   # Run testbench with demo scenario
   ./observer-testbench --scenario heap_spike

   # Export data via API
   curl -X POST http://localhost:8080/api/export \
     -o metrics.parquet -o findings.parquet

   # Run evaluation
   dda observer eval \
     --raw-metrics metrics.parquet \
     --findings findings.parquet \
     --plot results.png
   ```

## References

- **POT/EVT**: Siffer et al., "Anomaly Detection in Streams with Extreme Value Theory" (KDD 2017)
- **UCR Benchmark**: Yeh et al., "Matrix Profile I" (ICDM 2016)
- **Point-Adjustment**: Huet et al., "Local Evaluation of Time Series Anomaly Detection Algorithms" (KDD 2022)

## Support

For issues or questions:
- File an issue in the repository
- Contact the Observer team
- Check `.dda/extend/commands/observer/eval/tests/` for examples
