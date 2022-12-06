// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package stress

import (
	"math/rand"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const spinCheckName = "spin"

// Use CheckBase fields only
type SpinCheck struct {
	core.CheckBase
}

// Run executes the check
func (c *SpinCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	for i := 0; i < 10.000; i++ {
		logLevel := rand.Intn(6)
		logLength := rand.Intn(16) + 1

		logAtLevel(logLevel, logLength)
	}

	sender.Count("stress.spin.executed", 1, "", nil)
	return nil
}

func logAtLevel(logLevel, logLength int) {
	logLine := ""
	for i := 0; i < logLength; i++ {
		logLine += " "
		logLine += "Random log string"
	}

	switch logLevel {
	case 0:
		pkglog.Trace(logLine)
	case 1:
		pkglog.Debug(logLine)
	case 2:
		pkglog.Info(logLine)
	case 3:
		pkglog.Warn(logLine)
	case 4:
		pkglog.Error(logLine)
	case 5:
		pkglog.Critical(logLine)
	}
}

func loadFactory() check.Check {
	return &SpinCheck{
		CheckBase: core.NewCheckBase(spinCheckName),
	}
}

func init() {
	core.RegisterCheck(spinCheckName, loadFactory)
}
