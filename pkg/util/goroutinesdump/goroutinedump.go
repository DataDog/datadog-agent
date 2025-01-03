// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package goroutinesdump provides functions to get the stack trace of every Go routine of a running Agent.
package goroutinesdump

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// Get returns the stack trace of every Go routine of a running Agent.
func Get() (string, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return "", err
	}

	pprofURL := fmt.Sprintf("http://%v:%s/debug/pprof/goroutine?debug=2",
		ipcAddress, pkgconfigsetup.Datadog().GetString("expvar_port"))
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
