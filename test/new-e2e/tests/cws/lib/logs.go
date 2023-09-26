// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"errors"
	"time"

	"github.com/DataDog/datadog-api-client-go/api/v2/datadog"
	"github.com/cenkalti/backoff"
)

// WaitAppLogs waits for the app log corresponding to the query
func WaitAppLogs(apiClient MyAPIClient, query string) (*datadog.LogAttributes, error) {
	var resp *datadog.LogAttributes
	err := backoff.Retry(func() error {
		tmpResp, err := apiClient.GetAppLog(query)
		if err != nil {
			return err
		}
		if len(tmpResp.Data) > 0 {
			resp = tmpResp.Data[0].Attributes
			return nil
		}
		return errors.New("no log found")
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 60))
	return resp, err
}

// WaitAppSignal waits for the signal corresponding to the query
func WaitAppSignal(apiClient MyAPIClient, query string) (*datadog.SecurityMonitoringSignalAttributes, error) {
	var resp *datadog.SecurityMonitoringSignalAttributes
	err := backoff.Retry(func() error {
		tmpResp, err := apiClient.GetAppSignal(query)
		if err != nil {
			return err
		}
		if len(tmpResp.Data) > 0 {
			resp = tmpResp.Data[0].Attributes
			return nil
		}
		return errors.New("no log found")
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 60))
	return resp, err
}
