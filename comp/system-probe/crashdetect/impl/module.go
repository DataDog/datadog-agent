// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package crashdetectimpl

import (
	"net/http"

	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect/probe"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type winCrashDetectModule struct {
	*probe.WinCrashProbe
}

func (wcdm *winCrashDetectModule) Register(httpMux types.SystemProbeRouter) error {
	// only ever allow one concurrent check of the blue screen file.
	httpMux.HandleFunc("/check", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, _ *http.Request) {
		log.Infof("Got check request in crashDetect")
		results := wcdm.WinCrashProbe.Get()
		utils.WriteAsJSON(w, results, utils.CompactOutput)
	}))

	return nil
}

func (wcdm *winCrashDetectModule) GetStats() map[string]interface{} {
	return map[string]interface{}{}
}

func (wcdm *winCrashDetectModule) Close() {

}
