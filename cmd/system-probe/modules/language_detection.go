// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"fmt"
	"net/http"

	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/languagedetection"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var LanguageDetectionModule = module.Factory{
	Name:             config.LanguageDetectionModule,
	ConfigNamespaces: []string{"language_detection"},
	Fn: func(cfg *config.Config) (module.Module, error) {
		return &languageDetectionModule{}, nil
	},
}

var _ module.Module = (*languageDetectionModule)(nil)

type languageDetectionModule struct{}

func (l *languageDetectionModule) GetStats() map[string]interface{} {
	return nil
}

func (l *languageDetectionModule) Register(router *module.Router) error {
	router.HandleFunc("/lang/detect", func(writer http.ResponseWriter, request *http.Request) {
		var b []byte
		if _, err := request.Body.Read(b); err != nil {
			handleError(writer, http.StatusInternalServerError, fmt.Errorf("failed to read body of request: %w", err))
			return
		}

		var m languagedetection.DetectLanguageRequest
		if err := proto.Unmarshal(b, &m); err != nil {
			handleError(writer, http.StatusInternalServerError, fmt.Errorf("failed to unmarshal request: %w", err))
			return
		}
	})
}

func handleError(writer http.ResponseWriter, status int, err error) {
	_ = log.Error("Failed to properly handle /lang/detect request:", err)
	writer.WriteHeader(status)
}

func (l *languageDetectionModule) Close() {
	//TODO implement me
	panic("implement me")
}
