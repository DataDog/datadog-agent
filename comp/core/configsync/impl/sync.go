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
	"strconv"
	"time"

	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func (cs *configSync) updater() {
	cfg, err := fetchConfig(cs.ctx, cs.client, cs.Authtoken.Get(), cs.url.String())
	if err != nil {
		if cs.connected {
			cs.Log.Warnf("Failed to fetch config from core agent: %v", err)
			cs.connected = false
		} else {
			cs.Log.Debugf("Failed to fetch config from core agent: %v", err)
		}
		return
	}

	if cs.connected {
		cs.Log.Debug("Succeeded to fetch config from core agent")
	} else {
		cs.Log.Info("Succeeded to fetch config from core agent")
		cs.connected = true
	}

	for key, value := range cfg {
		cs.Log.Debugf("Updating config key %s from core agent", key)
		if key == "logs_config.additional_endpoints" {
			valueMap, ok := value.(map[string]string)
			if !ok {
				// this would be unexpected - but deal with it
				updateConfig(cs.Config, key, value)
				continue
			}

			typedValues := map[string]interface{}{}
			for cfgkey, cfgval := range valueMap {
				if cfgkey == "is_reliable" {
					if b, err := strconv.ParseBool(cfgval); err == nil {
						typedValues[cfgkey] = b
					} else {
						typedValues[cfgkey] = cfgval
					}
				}
				updateConfig(cs.Config, key, typedValues)
			}

		} else {
			updateConfig(cs.Config, key, value)
		}
	}
}

func (cs *configSync) runWithInterval(refreshInterval time.Duration) {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	cs.runWithChan(ticker.C)
}

func (cs *configSync) runWithChan(ch <-chan time.Time) {

	cs.Log.Infof("Starting to sync config with core agent at %s", cs.url)

	for {
		select {
		case <-cs.ctx.Done():
			return
		case <-ch:
			cs.updater()
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
