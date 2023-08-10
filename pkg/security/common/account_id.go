// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//revive:disable:var-naming

// Package common TODO comment
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
	name string
	// struct field accountIdName should be accountIDName
	accountIdName string
	callback      func(context.Context) bool
	// struct field accountIdCallback should be accountIDCallback
	accountIdCallback func(context.Context) (string, error)
}

// func queryAccountId should be queryAccountID
func queryAccountId(ctx context.Context) (string, string, error) {
	detectors := []cloudProviderDetector{
		{name: ec2.CloudProviderName, accountIdName: "account_id", callback: ec2.IsRunningOn, accountIdCallback: ec2.GetAccountID},
		{name: gce.CloudProviderName, accountIdName: "project_id", callback: gce.IsRunningOn, accountIdCallback: gce.GetProjectID},
		{name: azure.CloudProviderName, accountIdName: "subscription_id", callback: azure.IsRunningOn, accountIdCallback: azure.GetSubscriptionID},
	}

	for _, cloudDetector := range detectors {
		if cloudDetector.callback(ctx) {
			log.Infof("Cloud provider %s detected", cloudDetector.name)

			accountID, err := cloudDetector.accountIdCallback(ctx)
			if err != nil {
				return "", "", fmt.Errorf("could not detect cloud provider account ID: %w", err)
			}

			log.Infof("Detecting account id from %s cloud provider: %+q", cloudDetector.name, accountID)

			return cloudDetector.accountIdName, accountID, nil
		}
	}

	return "", "", fmt.Errorf("no cloud provider detected")
}

// var accountIdTagCache should be accountIDTagCache
var accountIdTagCache struct {
	sync.Once
	value string
}

// QueryAccountIdTag returns the account id tag matching the current deployment
// func QueryAccountIdTag should be QueryAccountIDTag
func QueryAccountIdTag() string {
	accountIdTagCache.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		tagName, tagValue, err := queryAccountId(ctx)
		if err != nil {
			log.Errorf("failed to query account id: %v", err)
			return
		}
		accountIdTagCache.value = fmt.Sprintf("%s:%s", tagName, tagValue)
	})

	return accountIdTagCache.value
}
