// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build anomalydetectiontestbench

package observerimpl

import "os"

// Only the three preregistered research schedules are accepted. Production
// configuration is unaffected; the default preserves the preceding experiment.
type seasonalLearningPlan struct {
	FitCycles, ValidationCycles, CalibrationHours int
}

func (p seasonalLearningPlan) cycles() int { return p.FitCycles + p.ValidationCycles }

func seasonalLearningPlanFromEnvironment() seasonalLearningPlan {
	switch os.Getenv("OBSERVER_REAL_SEASONAL_LEARNING_PLAN") {
	case "", "6-2-2":
		return seasonalLearningPlan{6, 2, 2}
	case "4-2-1":
		return seasonalLearningPlan{4, 2, 1}
	case "3-2-1":
		return seasonalLearningPlan{3, 2, 1}
	default:
		panic("unsupported research learning plan")
	}
}

var realSeasonalPlan = seasonalLearningPlanFromEnvironment()
