// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// Package agent implements the api endpoints for the `/agent` prefix.
// This group of endpoints is meant to provide high-level functionalities
// at the agent level.

// +build jmx

package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util"

	yaml "gopkg.in/yaml.v2"
)

func getJMXConfigs(w http.ResponseWriter, r *http.Request) {
	var ts int
	queries := r.URL.Query()
	if timestamps, ok := queries["timestamp"]; ok {
		ts, _ = strconv.Atoi(timestamps[0])
	}

	if int64(ts) > embed.JMXConfigCache.GetModified() {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	log.Debugf("Getting latest JMX Configs as of: %#v", ts)

	j := map[string]interface{}{}
	configs := map[string]integration.JSONMap{}

	configItems := embed.JMXConfigCache.Items()
	for name, config := range configItems {
		m, ok := config.(map[string]interface{})
		if !ok {
			err := fmt.Errorf("wrong type in cache")
			log.Errorf("%s", err.Error())
			http.Error(w, err.Error(), 500)
			return
		}

		cfg, ok := m["config"].(integration.Config)
		if !ok {
			err := fmt.Errorf("wrong type for config")
			log.Errorf("%s", err.Error())
			http.Error(w, err.Error(), 500)
			return
		}

		var rawInitConfig integration.RawMap
		err := yaml.Unmarshal(cfg.InitConfig, &rawInitConfig)
		if err != nil {
			log.Errorf("unable to parse JMX configuration: %s", err)
			http.Error(w, err.Error(), 500)
			return
		}

		c := map[string]interface{}{}
		c["init_config"] = util.GetJSONSerializableMap(rawInitConfig)
		instances := []integration.JSONMap{}
		for _, instance := range cfg.Instances {
			var rawInstanceConfig integration.JSONMap
			err = yaml.Unmarshal(instance, &rawInstanceConfig)
			if err != nil {
				log.Errorf("unable to parse JMX configuration: %s", err)
				http.Error(w, err.Error(), 500)
				return
			}
			instances = append(instances, util.GetJSONSerializableMap(rawInstanceConfig).(integration.JSONMap))
		}

		c["instances"] = instances
		configs[name] = c

	}
	j["configs"] = configs
	j["timestamp"] = time.Now().Unix()
	jsonPayload, err := json.Marshal(util.GetJSONSerializableMap(j))
	if err != nil {
		log.Errorf("unable to parse JMX configuration: %s", err)
		http.Error(w, err.Error(), 500)
		return
	}
	w.Write(jsonPayload)
}

func setJMXStatus(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)

	var jmxStatus status.JMXStatus
	err := decoder.Decode(&jmxStatus)
	if err != nil {
		log.Errorf("unable to parse jmx status: %s", err)
		http.Error(w, err.Error(), 500)
	}

	status.SetJMXStatus(jmxStatus)
}
