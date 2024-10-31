// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

const (
	// CheckName is the name of the check
	CheckName           = "containerd"
	pullImageGrpcMethod = "PullImage"
	cacheValidity       = 2 * time.Second
)

var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

// ContainerdCheck grabs containerd metrics and events
type ContainerdCheck struct {
	corechecks.CheckBase
	instance        *ContainerdConfig
	processor       generic.Processor
	subscriber      *subscriber
	containerFilter *containers.Filter
	client          cutil.ContainerdItf
	httpClient      http.Client
	store           workloadmeta.Component
	tagger          tagger.Component
}

// ContainerdConfig contains the custom options and configurations set by the user.
type ContainerdConfig struct {
	ContainerdFilters   []string `yaml:"filters"`
	CollectEvents       bool     `yaml:"collect_events"`
	OpenmetricsEndpoint string   `yaml:"openmetrics_endpoint"`
}

// Factory is used to create register the check and initialize it.
func Factory(store workloadmeta.Component, tagger tagger.Component) optional.Option[func() check.Check] {
	return optional.NewOption(func() check.Check {
		return &ContainerdCheck{
			CheckBase: corechecks.NewCheckBase(CheckName),
			instance:  &ContainerdConfig{},
			store:     store,
			tagger:    tagger,
		}
	})
}

// Parse is used to get the configuration set by the user
func (co *ContainerdConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, co)
}

// Configure parses the check configuration and init the check
func (c *ContainerdCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	var err error
	if err = c.CommonConfigure(senderManager, initConfig, config, source); err != nil {
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

	c.httpClient = http.Client{Timeout: time.Duration(1) * time.Second}
	c.processor = generic.NewProcessor(metrics.GetProvider(optional.NewOption(c.store)), generic.NewMetadataContainerAccessor(c.store), metricsAdapter{}, getProcessorFilter(c.containerFilter, c.store), c.tagger)
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
		sender.ServiceCheck("containerd.health", servicecheck.ServiceCheckCritical, "", nil, fmt.Sprintf("Connectivity error %v", errHealth))
		log.Infof("Error ensuring connectivity with Containerd daemon %v", errHealth)
		return errHealth
	}
	sender.ServiceCheck("containerd.health", servicecheck.ServiceCheckOK, "", nil, "")

	if err := c.runProcessor(sender); err != nil {
		_ = c.Warnf("Error collecting metrics: %s", err)
	}

	if err := c.runContainerdCustom(sender); err != nil {
		_ = c.Warnf("Error collecting metrics: %s", err)
	}

	if err := c.scrapeOpenmetricsEndpoint(sender); err != nil {
		_ = c.Warnf("Error collecting image pull metrics: %s", err)
	}

	c.collectEvents(sender)

	return nil
}

func (c *ContainerdCheck) runProcessor(sender sender.Sender) error {
	return c.processor.Run(sender, cacheValidity)
}

func (c *ContainerdCheck) runContainerdCustom(sender sender.Sender) error {
	namespaces, err := cutil.NamespacesToWatch(context.TODO(), c.client)
	if err != nil {
		return err
	}

	for _, namespace := range namespaces {
		if err := c.collectImageSizes(sender, c.client, namespace); err != nil {
			log.Infof("Failed to scrape containerd openmetrics endpoint, err: %s", err)
		}
	}

	return nil
}

func toSnakeCase(s string) string {
	snake := matchAllCap.ReplaceAllString(s, "${1}_${2}")
	return strings.ToLower(snake)
}

func (c *ContainerdCheck) scrapeOpenmetricsEndpoint(sender sender.Sender) error {

	if c.instance.OpenmetricsEndpoint == "" {
		return nil
	}

	openmetricsEndpoint := fmt.Sprintf("%s/v1/metrics", c.instance.OpenmetricsEndpoint)
	resp, err := c.httpClient.Get(openmetricsEndpoint)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	parsedMetrics, err := prometheus.ParseMetrics(body)
	if err != nil {
		return err
	}

	for _, mf := range parsedMetrics {
		for _, sample := range mf.Samples {
			if sample == nil {
				continue
			}

			metric := sample.Metric

			metricName, ok := metric["__name__"]

			if !ok {
				continue
			}

			transform, found := defaultContainerdOpenmetricsTransformers[string(metricName)]

			if found {
				transform(sender, string(metricName), *sample)
			}
		}
	}

	return nil
}

func (c *ContainerdCheck) collectImageSizes(sender sender.Sender, cl cutil.ContainerdItf, namespace string) error {
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

func (c *ContainerdCheck) collectEvents(sender sender.Sender) {
	if !c.instance.CollectEvents {
		return
	}

	if !c.subscriber.isRunning() {
		// Keep track of the health of the Containerd socket.
		c.subscriber.CheckEvents()
	}
	events := c.subscriber.Flush(time.Now().Unix())
	// Process events
	c.computeEvents(events, sender, c.containerFilter)
}
