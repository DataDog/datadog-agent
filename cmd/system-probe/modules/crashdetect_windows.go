// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package modules

import (
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect/probe"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"google.golang.org/grpc"
)

// WinCrashProbe Factory
var WinCrashProbe = module.Factory{
	Name:             config.WindowsCrashDetectModule,
	ConfigNamespaces: []string{"windows_crash_detection"},
	Fn: func(cfg *config.Config) (module.Module, error) {
		log.Infof("Starting the WinCrashProbe probe")
		cp, err := probe.NewWinCrashProbe(cfg)
		if err != nil {
			return nil, fmt.Errorf("unable to start the Windows Crash Detection probe: %w", err)
		}
		return &winCrashDetectModule{
			WinCrashProbe: cp,
		}, nil
	},
}

var _ module.Module = &winCrashDetectModule{}

type winCrashDetectModule struct {
	*probe.WinCrashProbe
}

func (wcdm *winCrashDetectModule) Register(httpMux *module.Router) error {
	// only ever allow one concurrent check of the blue screen file.
	httpMux.HandleFunc("/check", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, req *http.Request) {
		log.Infof("Got check request in crashDetect")
		results := wcdm.WinCrashProbe.Get()
		utils.WriteAsJSON(w, results)
	}))

	return nil
}

func (wcdm *winCrashDetectModule) RegisterGRPC(_ grpc.ServiceRegistrar) error {
	return nil
}

func (wcdm *winCrashDetectModule) GetStats() map[string]interface{} {
	return map[string]interface{}{}
}

func (wcdm *winCrashDetectModule) Close() {

}
