// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common holds common related files
package common

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type cloudProviderDetector struct {
	name              string
	accountIDName     string
	callback          func(context.Context) bool
	accountIDCallback func(context.Context) (string, error)
}

func queryAccountID(ctx context.Context) (string, string, error) {
	detectors := []cloudProviderDetector{
		{name: ec2.CloudProviderName, accountIDName: "account_id", callback: ec2.IsRunningOn, accountIDCallback: ec2.GetAccountID},
		{name: gce.CloudProviderName, accountIDName: "project_id", callback: gce.IsRunningOn, accountIDCallback: gce.GetProjectID},
		{name: azure.CloudProviderName, accountIDName: "subscription_id", callback: azure.IsRunningOn, accountIDCallback: azure.GetSubscriptionID},
	}

	for _, cloudDetector := range detectors {
		if cloudDetector.callback(ctx) {
			log.Infof("Cloud provider %s detected", cloudDetector.name)

			accountID, err := cloudDetector.accountIDCallback(ctx)
			if err != nil {
				return "", "", fmt.Errorf("could not detect cloud provider account ID: %w", err)
			}

			log.Infof("Detecting account id from %s cloud provider: %+q", cloudDetector.name, accountID)

			return cloudDetector.accountIDName, accountID, nil
		}
	}

	return "", "", fmt.Errorf("no cloud provider detected")
}

var accountIDTagCache struct {
	sync.Once
	value string
}

// QueryAccountIDTag returns the account id tag matching the current deployment
func QueryAccountIDTag() string {
	accountIDTagCache.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		tagName, tagValue, err := queryAccountID(ctx)
		if err != nil {
			log.Errorf("failed to query account id: %v", err)
			return
		}
		accountIDTagCache.value = fmt.Sprintf("%s:%s", tagName, tagValue)
	})

	return accountIDTagCache.value
}
