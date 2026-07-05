// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet || !systemd

package logssourceimpl

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// registerKubeletJournaldSource is a no-op stub; journald tailing requires
// both kubelet and systemd build tags. Returns nil (no source created).
func registerKubeletJournaldSource(_ *sources.LogSources, logger log.Component) *sources.LogSource {
	logger.Debugf("[observer/logssource] kubelet journald source not registered: requires kubelet+systemd build tags")
	return nil
}
