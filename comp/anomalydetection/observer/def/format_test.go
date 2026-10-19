// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observer

import "testing"

func TestFormatAnomaly(t *testing.T) {
	series := SeriesDescriptor{Name: "cpu.user", Aggregate: AggregateAverage}
	tests := []struct {
		name        string
		anomaly     Anomaly
		wantTitle   string
		wantDetails string
	}{
		{"scanmw up", Anomaly{Source: series, DetectorName: "scanmw", DebugInfo: &AnomalyDebugInfo{BaselineMedian: 10, CurrentValue: 25, PValue: 1e-8, EffectSize: 0.85, DeviationSigma: 5}},
			"ScanMW changepoint: cpu.user:avg", "cpu.user:avg increased (pre_median=10.0000, post_median=25.0000, p=1.00e-08, effect=0.85, 5.0 MADs)"},
		{"scanmw down", Anomaly{Source: series, DetectorName: "scanmw", DebugInfo: &AnomalyDebugInfo{BaselineMedian: 25, CurrentValue: 10, PValue: 2e-9, EffectSize: -0.7, DeviationSigma: 4}},
			"ScanMW changepoint: cpu.user:avg", "cpu.user:avg decreased (pre_median=25.0000, post_median=10.0000, p=2.00e-09, effect=-0.70, 4.0 MADs)"},
		{"scanwelch up", Anomaly{Source: series, DetectorName: "scanwelch", DebugInfo: &AnomalyDebugInfo{BaselineMedian: 10, CurrentValue: 25, TestStatistic: 7.2, PValue: 3e-9, EffectSize: 0.8, DeviationSigma: 4}},
			"ScanWelch changepoint: cpu.user:avg", "cpu.user:avg increased (pre_median=10.0000, post_median=25.0000, t=7.20, p=3.00e-09, effect=0.80, 4.0 MADs)"},
		{"scanwelch down", Anomaly{Source: series, DetectorName: "scanwelch", DebugInfo: &AnomalyDebugInfo{BaselineMedian: 25, CurrentValue: 10, TestStatistic: 8.4, PValue: 1e-8, EffectSize: -0.85, DeviationSigma: 5}},
			"ScanWelch changepoint: cpu.user:avg", "cpu.user:avg decreased (pre_median=25.0000, post_median=10.0000, t=8.40, p=1.00e-08, effect=-0.85, 5.0 MADs)"},
		{"bocpd change point", Anomaly{Source: series, DetectorName: "bocpd", DebugInfo: &AnomalyDebugInfo{Threshold: 0.5, BOCPDTrigger: BOCPDTriggerChangePointProbability, BOCPDChangePointProb: 0.6, BOCPDShortRunMass: 0.2, BOCPDShortRunLength: 3}},
			"BOCPD changepoint detected: cpu.user:avg", "cpu.user:avg changepoint probability 0.60 exceeded threshold 0.50 (cp=0.60, short-run<=3 mass=0.20)"},
		{"bocpd short run", Anomaly{Source: series, DetectorName: "bocpd", DebugInfo: &AnomalyDebugInfo{Threshold: 0.5, BOCPDTrigger: BOCPDTriggerShortRunMass, BOCPDChangePointProb: 0.2, BOCPDShortRunMass: 0.7, BOCPDShortRunLength: 3}},
			"BOCPD changepoint detected: cpu.user:avg", "cpu.user:avg short-run posterior mass 0.70 exceeded threshold 0.50 (cp=0.20, short-run<=3 mass=0.70)"},
		{"holt", Anomaly{Source: series, DetectorName: "holt_residual", DebugInfo: &AnomalyDebugInfo{CurrentValue: 25, Forecast: 12, Residual: 13, DeviationSigma: 4.2, HoltLevel: 11, HoltTrend: 1, ValueMADs: 5}},
			"Holt residual: cpu.user:avg", "cpu.user:avg deviated from forecast (observed=25.0000, forecast=12.0000, residual=13.0000, |z|=4.20, level=11.0000, trend=1.0000, 5.0 valueMADs)"},
		{"tukey down", Anomaly{Source: series, DetectorName: "tukey_biweight", DebugInfo: &AnomalyDebugInfo{BaselineMedian: 10, BaselineMAD: 0.5, TukeyBiweightZScore: -5, TukeyBiweightSampleCount: 80}},
			"Tukey biweight: cpu.user:avg", "below biweight baseline (z=-5.00, mu=10.0000, sigma=0.5000, n=80)"},
		{"tukey up no aggregate", Anomaly{Source: SeriesDescriptor{Name: "requests"}, DetectorName: "tukey_biweight", DebugInfo: &AnomalyDebugInfo{BaselineMedian: 10, BaselineMAD: 0.5, TukeyBiweightZScore: 5, TukeyBiweightSampleCount: 80}},
			"Tukey biweight: requests", "above biweight baseline (z=5.00, mu=10.0000, sigma=0.5000, n=80)"},
		{"missing debug", Anomaly{Source: series, DetectorName: "scanmw"},
			"Anomaly detected: scanmw: cpu.user:avg", ""},
		{"unknown trigger", Anomaly{Source: series, DetectorName: "bocpd", DebugInfo: &AnomalyDebugInfo{BOCPDTrigger: BOCPDTriggerUnknown}},
			"Anomaly detected: bocpd: cpu.user:avg", ""},
		{"unknown detector", Anomaly{Source: series, DetectorName: "custom", DebugInfo: &AnomalyDebugInfo{}},
			"Anomaly detected: custom: cpu.user:avg", ""},
		{"missing source", Anomaly{DetectorName: "scanmw", DebugInfo: &AnomalyDebugInfo{}},
			"Anomaly detected: scanmw", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			title, details := FormatAnomaly(tc.anomaly)
			if title != tc.wantTitle || details != tc.wantDetails {
				t.Fatalf("FormatAnomaly() = (%q, %q), want (%q, %q)", title, details, tc.wantTitle, tc.wantDetails)
			}
		})
	}
}
