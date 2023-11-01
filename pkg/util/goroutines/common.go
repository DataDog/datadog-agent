// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package goroutines has functions for goroutines used in Agent.
package goroutines

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// GetGoRoutinesDump returns the stack trace of every Go routine of a running Agent.
func GetGoRoutinesDump(cfg pkgconfigmodel.Reader) (string, error) {
	ipcAddress, err := pkgconfigmodel.GetIPCAddress(cfg)
	if err != nil {
		return "", err
	}

	pprofURL := fmt.Sprintf("http://%v:%s/debug/pprof/goroutine?debug=2",
		ipcAddress, cfg.GetString("expvar_port"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client := http.Client{}
	req, err := http.NewRequest(http.MethodGet, pprofURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	return string(data), err
}
