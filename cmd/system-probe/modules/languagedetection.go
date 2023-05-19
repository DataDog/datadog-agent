// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"net/http"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	pkgLanguageDetection "github.com/DataDog/datadog-agent/pkg/languagedetection"
)

var _ module.Module = (*languageDetection)(nil)

type languageDetection struct {
}

// NetworkTracer is a factory for NPM's tracer
var LanguageDetection = module.Factory{
	Name: config.NetworkTracerModule,
	Fn: func(cfg *config.Config) (module.Module, error) {
		return &languageDetection{}, nil
	},
}

func (l languageDetection) GetStats() map[string]interface{} {
	debug := map[string]interface{}{}
	return debug
}

func (l languageDetection) Register(router *module.Router) error {
	router.HandleFunc("/language", func(writer http.ResponseWriter, request *http.Request) {
		request.Body.Close()
		pkgLanguageDetection.DetectLanguageWithPrivileges()
	})
}

func (l languageDetection) Close() {
	//TODO implement me
	panic("implement me")
}
