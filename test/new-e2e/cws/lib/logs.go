// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-api-client-go/api/v2/datadog"
	"github.com/cenkalti/backoff"
)

func WaitAgentLogs(vm *client.VM, agentName string, pattern string) error {
	err := backoff.Retry(func() error {
		output, err := vm.ExecuteWithError(fmt.Sprintf("cat /var/log/datadog/%s.log", agentName))
		if err != nil {
			return err
		}
		if strings.Contains(output, pattern) {
			return nil
		}
		return errors.New("no log found")
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 60))
	return err
}

func WaitAppLogs(apiClient MyApiClient, query string) (*datadog.LogAttributes, error) {
	var resp *datadog.LogAttributes
	err := backoff.Retry(func() error {
		tmpResp, err := apiClient.GetAppLog(query)
		if err != nil {
			return err
		}
		if len(tmpResp.Data) > 0 {
			resp = tmpResp.Data[0].Attributes
			json.Marshal(resp)
			return nil
		}
		return errors.New("no log found")
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 60))
	return resp, err
}

func WaitAppSignal(apiClient MyApiClient, query string) (*datadog.SecurityMonitoringSignalAttributes, error) {
	var resp *datadog.SecurityMonitoringSignalAttributes
	err := backoff.Retry(func() error {
		tmpResp, err := apiClient.GetAppSignal(query)
		if err != nil {
			return err
		}
		if len(tmpResp.Data) > 0 {
			resp = tmpResp.Data[0].Attributes
			json.Marshal(resp)
			return nil
		}
		return errors.New("no log found")
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(500*time.Millisecond), 60))
	return resp, err
}
