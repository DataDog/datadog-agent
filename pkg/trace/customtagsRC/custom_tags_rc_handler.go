// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package customtagsRC

import (
	"encoding/json"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/davecgh/go-spew/spew"
)

type CustomTagsConfig struct {
	// array of fields - "span name: tag name"
	CustomTagsArr *[]string `json:"custom_tags"`
}

// RemoteConfigHandler holds pointers to samplers that need to be updated when APM remote config changes
type RemoteConfigHandler struct {
	remoteClient config.RemoteClient
	customTags   map[string][]string
	agentConfig  *config.AgentConfig
}

func New(conf *config.AgentConfig, customTags map[string][]string) *RemoteConfigHandler {
	if conf.RemoteCustomTagsClient == nil {
		return nil
	}

	return &RemoteConfigHandler{
		remoteClient: conf.RemoteCustomTagsClient,
		customTags:   customTags,
		agentConfig:  conf,
	}
}

func (h *RemoteConfigHandler) Start() {
	if h == nil {
		return
	}

	h.remoteClient.Start()
	h.remoteClient.Subscribe(state.ProductAgentConfig, h.onUpdate)
}

func (h *RemoteConfigHandler) onUpdate(update map[string]state.RawConfig) {
	if len(update) == 0 {
		log.Debugf("No updates to remote config for custom tags payload")
		return
	}

	if len(update) > 1 {
		log.Errorf("Custom tags remote config payload contains %v configurations, but it should contain at most one", len(update))
		return
	}

	var customTagsConfigPayload CustomTagsConfig
	for _, v := range update {
		err := json.Unmarshal(v.Config, &customTagsConfigPayload)
		if err != nil {
			log.Error(err)
			return
		}
	}

	log.Debugf("updating custom tags with remote configuration: %v", spew.Sdump(customTagsConfigPayload))
	h.updateCustomTags(customTagsConfigPayload)
}

func (h *RemoteConfigHandler) updateCustomTags(config CustomTagsConfig) {
	var customTagsConf *CustomTagsConfig

	if customTagsConf != nil && customTagsConf.CustomTagsArr != nil {

		var customTagsList = *config.CustomTagsArr
		for tag := range customTagsList {
			tagName, tagValue, _ := strings.Cut(customTagsList[tag], ":")
			h.agentConfig.CustomTags[tagName] = []string{tagValue}
		}

	}
}
