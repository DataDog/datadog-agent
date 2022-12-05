// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks
// +build clusterchecks

package app

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func InitializeCCCache(ctx context.Context) error {
	pollInterval := time.Second * time.Duration(config.Datadog.GetInt("cloud_foundry_cc.poll_interval"))
	_, err := cloudfoundry.ConfigureGlobalCCCache(
		ctx,
		config.Datadog.GetString("cloud_foundry_cc.url"),
		config.Datadog.GetString("cloud_foundry_cc.client_id"),
		config.Datadog.GetString("cloud_foundry_cc.client_secret"),
		config.Datadog.GetBool("cloud_foundry_cc.skip_ssl_validation"),
		pollInterval,
		config.Datadog.GetInt("cloud_foundry_cc.apps_batch_size"),
		config.Datadog.GetBool("cluster_agent.refresh_on_cache_miss"),
		config.Datadog.GetBool("cluster_agent.serve_nozzle_data"),
		config.Datadog.GetBool("cluster_agent.sidecars_tags"),
		config.Datadog.GetBool("cluster_agent.isolation_segments_tags"),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize CC Cache: %v", err)
	}
	return nil
}

func InitializeBBSCache(ctx context.Context) error {
	pollInterval := time.Second * time.Duration(config.Datadog.GetInt("cloud_foundry_bbs.poll_interval"))
	// NOTE: we can't use GetPollInterval in ConfigureGlobalBBSCache, as that causes import cycle

	includeListString := config.Datadog.GetStringSlice("cloud_foundry_bbs.env_include")
	excludeListString := config.Datadog.GetStringSlice("cloud_foundry_bbs.env_exclude")

	includeList := make([]*regexp.Regexp, len(includeListString))
	excludeList := make([]*regexp.Regexp, len(excludeListString))

	for i, pattern := range includeListString {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("failed to compile cloud_foundry_bbs.env_include regex pattern %s: %s", pattern, err.Error())
		}
		includeList[i] = re
	}

	for i, pattern := range excludeListString {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("failed to compile cloud_foundry_bbs.env_exclude regex pattern %s: %s", pattern, err.Error())
		}
		excludeList[i] = re
	}

	bc, err := cloudfoundry.ConfigureGlobalBBSCache(
		ctx,
		config.Datadog.GetString("cloud_foundry_bbs.url"),
		config.Datadog.GetString("cloud_foundry_bbs.ca_file"),
		config.Datadog.GetString("cloud_foundry_bbs.cert_file"),
		config.Datadog.GetString("cloud_foundry_bbs.key_file"),
		pollInterval,
		includeList,
		excludeList,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize BBS Cache: %s", err.Error())
	}
	log.Info("Waiting for initial warmup of BBS Cache")
	ticker := time.NewTicker(time.Second)
	timer := time.NewTimer(pollInterval * 5)
	for {
		select {
		case <-ticker.C:
			if bc.LastUpdated().After(time.Time{}) {
				return nil
			}
		case <-timer.C:
			ticker.Stop()
			return fmt.Errorf("BBS Cache failed to warm up. Misconfiguration error? Inspect logs")
		}
	}
}

func SetupClusterCheck(ctx context.Context) (*clusterchecks.Handler, error) {
	handler, err := clusterchecks.NewHandler(common.AC)
	if err != nil {
		return nil, err
	}
	go handler.Run(ctx)

	log.Info("Started cluster check Autodiscovery")
	return handler, nil
}
