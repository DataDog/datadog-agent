// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && systemd

package logssourceimpl

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// registerKubeletJournaldSource registers a journald source that tails the
// kubelet.service unit. Returns the created LogSource for caller-managed teardown.
// Only built when kubelet support is compiled in and the systemd build tag is set.
func registerKubeletJournaldSource(logSources *sources.LogSources, logger log.Component) *sources.LogSource {
	src := sources.NewLogSource("kubelet", &logsconfig.LogsConfig{
		Type:               logsconfig.JournaldType,
		ConfigID:           "kubelet",
		Source:             "kubelet", // enables msg.Origin.Source() for log filter matching
		IncludeSystemUnits: logsconfig.StringSliceField{"kubelet.service"},
		Tags:               logsconfig.StringSliceField{"source:kubelet"},
	})
	logSources.AddSource(src)
	logger.Infof("[observer/logssource] registered kubelet journald source: config_id=%q include_units=%v tags=%v",
		src.Config.ConfigID, src.Config.IncludeSystemUnits, src.Config.Tags)
	return src
}
