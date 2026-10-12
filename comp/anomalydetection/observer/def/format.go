// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observer

import "fmt"

// FormatAnomaly constructs detector text from the source descriptor and the
// evidence captured when the anomaly was emitted. Call it only for output.
func FormatAnomaly(a Anomaly) (title, description string) {
	source := a.Source.String()
	debug := a.DebugInfo
	if source == "" || debug == nil {
		return fallbackAnomalyTitle(a, source), ""
	}

	switch a.DetectorName {
	case "scanmw":
		direction := "increased"
		if debug.CurrentValue < debug.BaselineMedian {
			direction = "decreased"
		}
		return "ScanMW changepoint: " + source,
			fmt.Sprintf("%s %s (pre_median=%.4f, post_median=%.4f, p=%.2e, effect=%.2f, %.1f MADs)",
				source, direction, debug.BaselineMedian, debug.CurrentValue, debug.PValue, debug.EffectSize, debug.DeviationSigma)
	case "scanwelch":
		direction := "increased"
		if debug.CurrentValue < debug.BaselineMedian {
			direction = "decreased"
		}
		return "ScanWelch changepoint: " + source,
			fmt.Sprintf("%s %s (pre_median=%.4f, post_median=%.4f, t=%.2f, p=%.2e, effect=%.2f, %.1f MADs)",
				source, direction, debug.BaselineMedian, debug.CurrentValue, debug.TestStatistic, debug.PValue, debug.EffectSize, debug.DeviationSigma)
	case "bocpd":
		var triggerType string
		var triggerValue float64
		switch debug.BOCPDTrigger {
		case BOCPDTriggerChangePointProbability:
			triggerType = "changepoint probability"
			triggerValue = debug.BOCPDChangePointProb
		case BOCPDTriggerShortRunMass:
			triggerType = "short-run posterior mass"
			triggerValue = debug.BOCPDShortRunMass
		default:
			return fallbackAnomalyTitle(a, source), ""
		}
		return "BOCPD changepoint detected: " + source,
			fmt.Sprintf("%s %s %.2f exceeded threshold %.2f (cp=%.2f, short-run<=%d mass=%.2f)",
				source, triggerType, triggerValue, debug.Threshold, debug.BOCPDChangePointProb, debug.BOCPDShortRunLength, debug.BOCPDShortRunMass)
	case "holt_residual":
		return "Holt residual: " + source,
			fmt.Sprintf("%s deviated from forecast (observed=%.4f, forecast=%.4f, residual=%.4f, |z|=%.2f, level=%.4f, trend=%.4f, %.1f valueMADs)",
				source, debug.CurrentValue, debug.Forecast, debug.Residual, debug.DeviationSigma, debug.HoltLevel, debug.HoltTrend, debug.ValueMADs)
	case "tukey_biweight":
		direction := "above"
		if debug.TukeyBiweightZScore < 0 {
			direction = "below"
		}
		return "Tukey biweight: " + source,
			fmt.Sprintf("%s biweight baseline (z=%.2f, mu=%.4f, sigma=%.4f, n=%d)",
				direction, debug.TukeyBiweightZScore, debug.BaselineMedian, debug.BaselineMAD, debug.TukeyBiweightSampleCount)
	default:
		return fallbackAnomalyTitle(a, source), ""
	}
}

func fallbackAnomalyTitle(a Anomaly, source string) string {
	if source == "" {
		return "Anomaly detected: " + a.DetectorName
	}
	if a.DetectorName == "" {
		return "Anomaly detected: " + source
	}
	return "Anomaly detected: " + a.DetectorName + ": " + source
}
