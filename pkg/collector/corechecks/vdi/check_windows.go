// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package vdi

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/pdhutil"
)

const (
	providerAWSWorkSpaces = "aws_workspaces"
	protocolDCV           = "dcv"
)

var (
	connectionPattern = regexp.MustCompile(`^(.*):([0-9]+)$`)
	channelPattern    = regexp.MustCompile(`^(.*):([0-9]+):(.+)$`)
)

type awsWorkSpacesConfig struct {
	Product string `yaml:"product"`
}

type instanceConfig struct {
	Provider      string               `yaml:"provider"`
	AWSWorkSpaces *awsWorkSpacesConfig `yaml:"aws_workspaces"`
}

type registeredCounter struct {
	object objectDefinition
	def    counterDefinition
	single pdhutil.PdhSingleInstanceCounter
	multi  pdhutil.PdhMultiInstanceCounter
}

type checkImpl struct {
	core.CheckBase
	config   instanceConfig
	query    *pdhutil.PdhQuery
	counters []registeredCounter
}

// Factory creates a VDI check on Windows.
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &checkImpl{CheckBase: core.NewCheckBase(CheckName)}
}

// Configure initializes the VDI check.
func (c *checkImpl) Configure(senderManager sender.SenderManager, digest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string, provider string) error {
	c.BuildID(digest, rawInstance, rawInitConfig)
	if err := c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source, provider); err != nil {
		return err
	}
	s, err := c.GetSender()
	if err != nil {
		return err
	}
	s.FinalizeCheckServiceTag()

	if err := yaml.Unmarshal(rawInstance, &c.config); err != nil {
		return fmt.Errorf("invalid VDI instance configuration: %w", err)
	}
	if c.config.Provider != providerAWSWorkSpaces {
		return fmt.Errorf("unsupported VDI provider %q", c.config.Provider)
	}
	if c.config.AWSWorkSpaces == nil {
		c.config.AWSWorkSpaces = &awsWorkSpacesConfig{Product: "auto"}
	}
	if c.config.AWSWorkSpaces.Product == "" {
		c.config.AWSWorkSpaces.Product = "auto"
	}
	switch c.config.AWSWorkSpaces.Product {
	case "auto", "personal", "applications":
	default:
		return fmt.Errorf("invalid aws_workspaces.product %q", c.config.AWSWorkSpaces.Product)
	}

	c.query, err = pdhutil.CreatePdhQuery()
	if err != nil {
		return fmt.Errorf("create DCV PDH query: %w", err)
	}
	for _, object := range dcvObjects {
		for _, definition := range object.counters {
			counter := registeredCounter{object: object, def: definition}
			if object.multiple {
				if definition.optional {
					counter.multi = c.query.AddOptionalEnglishMultiInstanceCounter(object.object, definition.counter, nil)
				} else {
					counter.multi = c.query.AddEnglishMultiInstanceCounter(object.object, definition.counter, nil)
				}
			} else {
				if definition.optional {
					counter.single = c.query.AddOptionalEnglishSingleInstanceCounter(object.object, definition.counter)
				} else {
					counter.single = c.query.AddEnglishSingleInstanceCounter(object.object, definition.counter)
				}
			}
			c.counters = append(c.counters, counter)
		}
	}
	return nil
}

// Run collects Amazon DCV performance counters.
func (c *checkImpl) Run() error {
	s, err := c.GetSender()
	if err != nil {
		return err
	}
	defer s.Commit()

	pdhHealthy := true
	usableSamples := 0
	if err := c.query.CollectQueryData(); err != nil {
		pdhHealthy = false
		c.Warnf("Unable to collect Amazon DCV performance counters: %v", err)
	} else {
		for _, counter := range c.counters {
			metricName := counter.object.prefix + "." + counter.def.metric
			if counter.object.multiple {
				values, valueErr := counter.multi.GetAllValues()
				if valueErr != nil {
					if !counter.def.optional {
						pdhHealthy = false
					}
					log.Debugf("Unable to read %s\\%s: %v", counter.object.object, counter.def.counter, valueErr)
					continue
				}
				for instance, value := range values {
					submitMetric(s, counter.def.kind, metricName, value, c.tagsForInstance(counter.object.object, instance))
					usableSamples++
				}
			} else {
				value, valueErr := counter.single.GetValue()
				if valueErr != nil {
					if !counter.def.optional {
						pdhHealthy = false
					}
					log.Debugf("Unable to read %s\\%s: %v", counter.object.object, counter.def.counter, valueErr)
					continue
				}
				submitMetric(s, counter.def.kind, metricName, value, c.baseTags())
				usableSamples++
			}
		}
	}

	if usableSamples == 0 {
		pdhHealthy = false
	}
	if pdhHealthy {
		s.ServiceCheck("vdi.dcv.health", servicecheck.ServiceCheckOK, "", c.baseTags(), "")
	} else {
		s.ServiceCheck("vdi.dcv.health", servicecheck.ServiceCheckCritical, "", c.baseTags(), "Amazon DCV performance counter collection failed or was incomplete")
	}
	return nil
}

func (c *checkImpl) tagsForInstance(objectName, instance string) []string {
	tags := c.baseTags()
	if strings.EqualFold(instance, "_Total") {
		return tags
	}

	switch objectName {
	case "DCV Server Processes":
		tags = append(tags, "dcv_process:"+instance)
		lower := strings.ToLower(instance)
		for _, agentType := range []string{"session_agent", "system_agent", "user_agent"} {
			if strings.Contains(lower, agentType) {
				tags = append(tags, "dcv_agent_type:"+agentType)
				break
			}
		}
	case "DCV Server Sessions":
		tags = append(tags, "dcv_session_id:"+instance)
	case "DCV Server Connections":
		if match := connectionPattern.FindStringSubmatch(instance); match != nil {
			tags = append(tags, "dcv_session_id:"+match[1], "dcv_connection_id:"+match[2])
		}
	case "DCV Server Channels":
		if match := channelPattern.FindStringSubmatch(instance); match != nil {
			tags = append(tags, "dcv_session_id:"+match[1], "dcv_connection_id:"+match[2], "dcv_channel:"+match[3])
		}
	case "DCV Server Imaging":
		parts := strings.SplitN(instance, ":", 2)
		tags = append(tags, "dcv_session_id:"+parts[0])
		if len(parts) == 2 {
			tags = append(tags, "dcv_encoder:"+parts[1])
		}
	}
	return deduplicate(tags)
}

func (c *checkImpl) baseTags() []string {
	tags := []string{"vdi_provider:" + providerAWSWorkSpaces, "vdi_protocol:" + protocolDCV}
	product := c.config.AWSWorkSpaces.Product
	if product == "auto" {
		if os.Getenv("AppStream_Resource_Type") != "" {
			product = "applications"
		} else {
			product = "personal"
		}
	}
	tags = append(tags, "workspaces_product:"+product)
	if product == "applications" {
		for variable, tagName := range map[string]string{
			"AppStream_Resource_Name": "workspaces_fleet",
			"AppStream_Image_Arn":     "workspaces_image",
			"AppStream_Instance_Type": "instance_type",
		} {
			if value := os.Getenv(variable); value != "" {
				tags = append(tags, tagName+":"+value)
			}
		}
	}
	return deduplicate(tags)
}

func submitMetric(s sender.Sender, kind metricType, name string, value float64, tags []string) {
	if kind == monotonicCountMetric {
		s.MonotonicCount(name, value, "", tags)
		return
	}
	s.Gauge(name, value, "", tags)
}

func deduplicate(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		if _, found := seen[tag]; found {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	return result
}
