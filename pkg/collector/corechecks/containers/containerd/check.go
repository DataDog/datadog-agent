// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"context"
	"fmt"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	coreMetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	containerdCheckName = "containerd"
	cacheValidity       = 2 * time.Second
)

// ContainerdCheck grabs containerd metrics and events
type ContainerdCheck struct {
	corechecks.CheckBase
	instance        *ContainerdConfig
	processor       generic.Processor
	subscriber      *subscriber
	containerFilter *containers.Filter
	client          cutil.ContainerdItf
}

// ContainerdConfig contains the custom options and configurations set by the user.
type ContainerdConfig struct {
	ContainerdFilters []string `yaml:"filters"`
	CollectEvents     bool     `yaml:"collect_events"`
}

func init() {
	corechecks.RegisterCheck(containerdCheckName, ContainerdFactory)
}

// ContainerdFactory is used to create register the check and initialize it.
func ContainerdFactory() check.Check {
	return &ContainerdCheck{
		CheckBase: corechecks.NewCheckBase(containerdCheckName),
		instance:  &ContainerdConfig{},
	}
}

// Parse is used to get the configuration set by the user
func (co *ContainerdConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, co)
}

// Configure parses the check configuration and init the check
func (c *ContainerdCheck) Configure(integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	var err error
	if err = c.CommonConfigure(integrationConfigDigest, initConfig, config, source); err != nil {
		return err
	}

	if err = c.instance.Parse(config); err != nil {
		return err
	}

	c.containerFilter, err = containers.GetSharedMetricFilter()
	if err != nil {
		log.Warnf("Can't get container include/exclude filter, no filtering will be applied: %v", err)
	}

	c.client, err = cutil.NewContainerdUtil()
	if err != nil {
		return err
	}

	c.processor = generic.NewProcessor(metrics.GetProvider(), generic.MetadataContainerAccessor{}, metricsAdapter{}, getProcessorFilter(c.containerFilter))
	c.processor.RegisterExtension("containerd-custom-metrics", &containerdCustomMetricsExtension{})
	c.subscriber = createEventSubscriber("ContainerdCheck", c.client, cutil.FiltersWithNamespaces(c.instance.ContainerdFilters))

	return nil
}

// Run executes the check
func (c *ContainerdCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	defer sender.Commit()

	// As we do not rely on a singleton, we ensure connectivity every check run.
	if errHealth := c.client.CheckConnectivity(); errHealth != nil {
		sender.ServiceCheck("containerd.health", coreMetrics.ServiceCheckCritical, "", nil, fmt.Sprintf("Connectivity error %v", errHealth))
		log.Infof("Error ensuring connectivity with Containerd daemon %v", errHealth)
		return errHealth
	}
	sender.ServiceCheck("containerd.health", coreMetrics.ServiceCheckOK, "", nil, "")

	if err := c.runProcessor(sender); err != nil {
		_ = c.Warnf("Error collecting metrics: %s", err)
	}

	if err := c.runContainerdCustom(sender); err != nil {
		_ = c.Warnf("Error collecting metrics: %s", err)
	}

	c.collectEvents(sender)

	return nil
}

func (c *ContainerdCheck) runProcessor(sender aggregator.Sender) error {
	return c.processor.Run(sender, cacheValidity)
}

func (c *ContainerdCheck) runContainerdCustom(sender aggregator.Sender) error {
	namespaces, err := cutil.NamespacesToWatch(context.TODO(), c.client)
	if err != nil {
		return err
	}

	for _, namespace := range namespaces {
		if err := c.collectImageSizes(sender, c.client, namespace); err != nil {
			log.Infof("Failed to collect images size, err: %s", err)
		}
	}

	return nil
}

func (c *ContainerdCheck) collectImageSizes(sender aggregator.Sender, cl cutil.ContainerdItf, namespace string) error {
	// Report images size
	images, err := cl.ListImages(namespace)
	if err != nil {
		return err
	}

	for _, image := range images {
		var size int64

		if err := cl.CallWithClientContext(namespace, func(c context.Context) error {
			size, err = image.Size(c)
			return err
		}); err != nil {
			log.Debugf("Unable to get image size for image: %s, err: %s", image.Name(), err)
			continue
		}

		sender.Gauge("containerd.image.size", float64(size), "", getImageTags(image.Name()))
	}

	return nil
}

func (c *ContainerdCheck) collectEvents(sender aggregator.Sender) {
	if !c.instance.CollectEvents {
		return
	}

	if !c.subscriber.isRunning() {
		// Keep track of the health of the Containerd socket.
		c.subscriber.CheckEvents()
	}
	events := c.subscriber.Flush(time.Now().Unix())
	// Process events
	computeEvents(events, sender, c.containerFilter)
}
