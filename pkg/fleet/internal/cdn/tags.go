// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/hosttags"
	detectenv "github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/yaml.v2"
)

type hostTagsGetter struct {
	config model.Config
}

func newHostTagsGetter() hostTagsGetter {
	config := pkgconfigsetup.Datadog()
	detectenv.DetectFeatures(config) // For host tags to work
	err := populateTags(config)
	if err != nil {
		log.Warnf("Failed to populate tags from datadog.yaml: %v", err)
	}
	return hostTagsGetter{
		config: config,
	}
}

type tagsConfigFields struct {
	Tags      []string `yaml:"tags"`
	ExtraTags []string `yaml:"extra_tags"`
}

// populateTags is a best effort to get the tags from `datadog.yaml`.
func populateTags(config model.Config) error {
	configPath := "/etc/datadog-agent/datadog.yaml"
	if runtime.GOOS == "windows" {
		configPath = "C:\\ProgramData\\Datadog\\datadog.yaml"
	}
	rawConfig, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var cfg tagsConfigFields
	err = yaml.Unmarshal(rawConfig, &cfg)
	if err != nil {
		return err
	}
	config.Set("tags", cfg.Tags, model.SourceFile)
	config.Set("extra_tags", cfg.ExtraTags, model.SourceFile)
	return nil
}

func (h *hostTagsGetter) get() []string {
	// Host tags are cached on host, but we add a timeout to avoid blocking the request
	// if the host tags are not available yet and need to be fetched
	ctx, cc := context.WithTimeout(context.Background(), time.Second)
	defer cc()
	hostTags := hosttags.Get(ctx, true, h.config)
	tags := append(hostTags.System, hostTags.GoogleCloudPlatform...)
	return tags
}
