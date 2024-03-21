// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configsyncimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"time"

	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func (cs *configSync) runWithInterval(refreshInterval time.Duration) {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	cs.runWithChan(ticker.C)
}

func (cs *configSync) runWithChan(ch <-chan time.Time) {
	// whether we managed to contact the core-agent, used to avoid spamming logs
	connected := true
	url := cs.url.String()

	cs.Log.Infof("Starting to sync config with core agent at %s", cs.url)

	for {
		select {
		case <-cs.ctx.Done():
			return
		case <-ch:
			cfg, err := fetchConfig(cs.ctx, cs.client, cs.Authtoken.Get(), url)
			if err != nil {
				if connected {
					cs.Log.Warnf("Failed to fetch config from core agent: %v", err)
					connected = false
				} else {
					cs.Log.Debugf("Failed to fetch config from core agent: %v", err)
				}
				continue
			}

			if connected {
				cs.Log.Debug("Succeeded to fetch config from core agent")
			} else {
				cs.Log.Info("Succeeded to fetch config from core agent")
				connected = true
			}

			for key, value := range cfg {
				if updateConfig(cs.Config, key, value) {
					cs.Log.Debugf("Updating config key %s from core agent", key)
				}
			}
		}
	}
}

// fetchConfig contacts the url in configSync and parses the returned data
func fetchConfig(ctx context.Context, client *http.Client, authtoken, url string) (map[string]interface{}, error) {
	options := apiutil.ReqOptions{
		Ctx:       ctx,
		Conn:      apiutil.LeaveConnectionOpen,
		Authtoken: authtoken,
	}
	data, err := apiutil.DoGetWithOptions(client, url, &options)
	if err != nil {
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return config, nil
}

func updateConfig(cfg pkgconfigmodel.ReaderWriter, key string, value interface{}) bool {
	// check if the value changed to only log if it effectively changed the value
	oldvalue := cfg.Get(key)
	cfg.Set(key, value, pkgconfigmodel.SourceLocalConfigProcess)

	return !reflect.DeepEqual(oldvalue, cfg.Get(key))
}
