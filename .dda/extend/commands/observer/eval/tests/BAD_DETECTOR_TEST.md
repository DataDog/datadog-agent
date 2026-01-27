# Bad Detector Test Case

## Purpose

This test case validates that the evaluation framework correctly identifies **poor anomaly detection performance**. It uses inverted anomaly scores to simulate a badly calibrated or malfunctioning detector.

## Scenario Description

**Test Case**: `bad_detector`

**Setup**:
- 100 timestamps (t=1000 to t=1099)
- Metric: `heap.used_mb`
- Ground truth incident: t=1040 to t=1060 (20 seconds)
- **Detector behavior**: INVERTED scores
  - **High scores (0.6-0.9)** during NORMAL periods (t=1000-1039, t=1061-1099)
  - **Low scores (0.0-0.3)** during INCIDENT period (t=1040-1060)

This simulates a detector that:
- Produces false alarms when everything is normal
- Fails to detect the actual incident
- Has fundamentally broken logic

## Expected Results

A bad detector should produce:
- **UCR Score = 0**: Max score should be outside the incident window
- **Adjusted F1 ≈ 0**: No correct detections
- **AUC-ROC ≈ 0**: Poor discrimination (worse than random)
- **Precision = 0**: All detections are false positives
- **Recall = 0**: Misses all true anomalies

## Actual Results

### Test Output
```json
{
  "UCR_Score": 0.0,              ✅ EXPECTED: Max score outside window
  "Adjusted_F1": 0.0,            ✅ EXPECTED: No correct detections
  "AUC_ROC": 0.003,              ✅ EXPECTED: Terrible discrimination (0.003 ≈ 0)
  "Computed_Threshold": 0.898182,
  "Total_Anomalies_Found": 1,
  "Precision": 0.0,              ✅ EXPECTED: False positive only
  "Recall": 0.0,                 ✅ EXPECTED: Missed all true anomalies
  "True_Positives": 0,
  "False_Positives": 1,          ✅ EXPECTED: One false alarm
  "False_Negatives": 21,         ✅ EXPECTED: Missed all 21 incident points
  "Max_Score_Timestamp": 1061,   ✅ EXPECTED: Right after incident ended (t=1060)
  "Max_Score_In_Window": false   ✅ EXPECTED: Max not in window
}
```

### Interpretation

The framework correctly identifies this as a **completely broken detector**:

1. **UCR Score = 0.0**
   - The maximum anomaly score (0.9) occurred at t=1061 (right after the incident ended)
   - This is OUTSIDE the incident window [1040, 1060]
   - Detector failed the most basic test: identifying the most anomalous point

2. **AUC-ROC = 0.003**
   - Area under ROC curve is essentially 0 (would be 0.5 for random guessing, 1.0 for perfect)
   - This is **worse than random** - the detector has inverted logic
   - Confirms the detector is fundamentally broken

3. **Adjusted F1 = 0.0**
   - No true positives after point-adjustment
   - The one detection (at t=1061) was a false positive
   - Zero recall means it missed the entire incident

4. **Precision = 0.0, Recall = 0.0**
   - All 21 points in the incident window were missed (false negatives)
   - The single detection was a false positive (outside the window)
   - No useful signal from this detector

## Comparison: Good vs Bad Detector

### Good Detector (`simple_incident`)
```json
{
  "UCR_Score": 1.0,        // ✅ Max score in window
  "Adjusted_F1": 1.0,      // ✅ Perfect detection
  "AUC_ROC": 0.9795,       // ✅ Excellent discrimination
  "Precision": 1.0,        // ✅ No false positives
  "Recall": 1.0            // ✅ Found all anomalies
}
```

### Bad Detector (`bad_detector`)
```json
{
  "UCR_Score": 0.0,        // ❌ Max score outside window
  "Adjusted_F1": 0.0,      // ❌ No correct detections
  "AUC_ROC": 0.003,        // ❌ Worse than random
  "Precision": 0.0,        // ❌ All false positives
  "Recall": 0.0            // ❌ Missed all anomalies
}
```

### Visual Difference

The generated plot (`/tmp/observer_eval_bad.png`) shows:
- **Panel 1 (Raw Metric)**: Same as good detector - clear spike during incident window (green shaded)
- **Panel 2 (Anomaly Scores)**: INVERTED pattern
  - Low scores during the incident (green window)
  - High scores before and after (false alarms)
  - Threshold line shows only one point exceeds it (outside the window)
- **Panel 3 (Binary Comparison)**: Mismatch
  - Ground truth shows incident period
  - Predictions show single false positive outside the window
  - Clear visual indication of detection failure

## Usage

### Generate the test data:
```bash
dda observer generate-test-data
```

### Run evaluation:
```bash
dda observer eval \
  --raw-metrics .dda/extend/commands/observer/eval/tests/fixtures/bad_detector_metrics.parquet \
  --findings .dda/extend/commands/observer/eval/tests/fixtures/bad_detector_findings.parquet \
  --plot bad_detector_results.png \
  --output bad_detector_results.json \
  --verbose
```

### Verify results:
```bash
cat bad_detector_results.json
# Expected: UCR_Score = 0.0, Adjusted_F1 = 0.0, AUC_ROC ≈ 0.0
```

## Why This Test Matters

1. **Validation**: Confirms the framework doesn't give false confidence to bad detectors
2. **Sensitivity**: Shows the metrics can distinguish good from bad performance
3. **Debugging**: Helps identify when a detector has inverted logic or broken calibration
4. **Baseline**: Provides a worst-case comparison point (scores should be > 0)
5. **CI Testing**: Can be used in automated tests to ensure framework integrity

## Integration Testing

This test case is essential for CI pipelines:

```yaml
# .gitlab-ci.yml example
observer-eval-validation:
  script:
    # Generate test data
    - dda observer generate-test-data

    # Test good detector (should pass)
    - dda observer eval --raw-metrics fixtures/simple_incident_metrics.parquet
        --findings fixtures/simple_incident_findings.parquet --output good.json
    - python validate_metrics.py good.json --expect-ucr 1.0 --expect-f1-gt 0.9

    # Test bad detector (should fail gracefully)
    - dda observer eval --raw-metrics fixtures/bad_detector_metrics.parquet
        --findings fixtures/bad_detector_findings.parquet --output bad.json
    - python validate_metrics.py bad.json --expect-ucr 0.0 --expect-f1-lt 0.1
```

## Conclusion

✅ **Test PASSED**: The evaluation framework correctly identifies the bad detector with:
- All metrics at or near zero
- Clear indication of detection failure
- Accurate diagnosis of the problem (inverted scores)

This validates that the framework can:
- Distinguish good detectors from bad detectors
- Provide reliable performance metrics
- Be trusted for evaluating real anomaly detection systems
