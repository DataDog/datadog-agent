// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package modules

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/logonduration"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(LogonDuration) }

// LogonDuration Factory
var LogonDuration = &module.Factory{
	Name:             config.LogonDurationModule,
	ConfigNamespaces: []string{"logon_duration"},
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		return &logonDurationModule{}, nil
	},
}

var _ module.Module = &logonDurationModule{}

type logonDurationModule struct {
}

func (m *logonDurationModule) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/check", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, _ *http.Request) {
		log.Infof("Got check request in logon_duration module")
		timestamps := logonduration.GetLoginTimestamps()
		utils.WriteAsJSON(w, timestamps, utils.CompactOutput)
	}))

	return nil
}

func (m *logonDurationModule) GetStats() map[string]interface{} {
	return map[string]interface{}{}
}

func (m *logonDurationModule) Close() {
}
