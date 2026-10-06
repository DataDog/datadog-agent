// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package productcomposition

import (
	"go.uber.org/fx"

	pythonruntimefx "github.com/DataDog/datadog-agent/comp/collector/pythonruntime/fx-python"
	pythondiscoveryfx "github.com/DataDog/datadog-agent/comp/core/autodiscovery/discoverer/fx-python"
	pythonchecksfx "github.com/DataDog/datadog-agent/comp/core/gui/impl/pythonchecks/fx-python"
	pythoninfofx "github.com/DataDog/datadog-agent/comp/metadata/host/impl/pythoninfo/fx-python"
	collectorpython "github.com/DataDog/datadog-agent/pkg/collector/python"
)

func autodiscoveryOptions() []fx.Option {
	return []fx.Option{
		pythondiscoveryfx.Module(),
	}
}

func hostMetadataOptions() []fx.Option {
	return []fx.Option{
		pythoninfofx.Module(),
	}
}

func pythonCollectorOptions() []fx.Option {
	return []fx.Option{
		pythonruntimefx.Module(),
	}
}

func guiOptions() []fx.Option {
	return []fx.Option{
		pythonchecksfx.Module(),
	}
}

func pythonCheckOptions() []fx.Option {
	return []fx.Option{
		pythonruntimefx.Module(),
	}
}

var pythonVersionGetFunc = collectorpython.GetPythonVersion
