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
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
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
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

const (
	// CheckName is the name of the check
	CheckName           = "containerd"
	pullImageGrpcMethod = "PullImage"
	cacheValidity       = 2 * time.Second

	imageSizeQueryInterval = 10 * time.Minute
	imageCreateEvent       = "/images/create"
	imageUpdateEvent       = "/images/update"
	imageDeleteEvent       = "/images/delete"
	imageWildcardEvent     = "/images/*"
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
func Factory(store workloadmeta.Component, tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
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
	c.processor = generic.NewProcessor(metrics.GetProvider(option.New(c.store)), generic.NewMetadataContainerAccessor(c.store), metricsAdapter{}, getProcessorFilter(c.containerFilter, c.store), c.tagger, false)
	c.processor.RegisterExtension("containerd-custom-metrics", &containerdCustomMetricsExtension{})
	c.subscriber = createEventSubscriber("ContainerdCheck", c.client, cutil.FiltersWithNamespaces(c.instance.ContainerdFilters))

	c.subscriber.isCacheConfigValid = c.isEventConfigValid()
	if err := c.initializeImageCache(); err != nil {
		log.Warnf("Failed to initialize image size cache: %v", err)
	}
	if !c.subscriber.isCacheConfigValid {
		log.Debugf("Image event collection not configured. Starting periodic cache updates.")
		go c.periodicImageSizeQuery()
	}

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
		if err := c.collectImageSizes(sender, namespace); err != nil {
			log.Infof("Namespace skipped: %s", err)
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

func (c *ContainerdCheck) collectImageSizes(sender sender.Sender, namespace string) error {
	imageSizes := c.subscriber.GetImageSizes()
	cachedImages, ok := imageSizes[namespace]
	if !ok {
		return fmt.Errorf("no cached images found for namespace: %s", namespace)
	}

	for imageName, size := range cachedImages {
		sender.Gauge("containerd.image.size", float64(size), "", getImageTags(imageName))
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

func (c *ContainerdCheck) initializeImageCache() error {
	namespaces, err := cutil.NamespacesToWatch(context.TODO(), c.client)
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	newCache := make(map[string]map[string]int64)

	for _, namespace := range namespaces {
		images, err := c.client.ListImages(namespace)
		if err != nil {
			log.Warnf("Failed to list images for namespace %s: %v", namespace, err)
			continue
		}

		newCache[namespace] = make(map[string]int64)

		for _, image := range images {
			size, err := c.subscriber.getImageSize(namespace, image.Name())
			if err != nil {
				log.Debugf("Failed to get size for image %s in namespace %s: %v", image.Name(), namespace, err)
				continue
			}

			newCache[namespace][image.Name()] = size
		}
	}

	c.subscriber.imageSizeCacheLock.Lock()
	c.subscriber.imageSizeCache = newCache
	c.subscriber.imageSizeCacheLock.Unlock()

	return nil
}

func (c *ContainerdCheck) isEventConfigValid() bool {
	if !c.instance.CollectEvents {
		return false
	}

	hasImageEvents := map[string]bool{
		imageCreateEvent: false,
		imageUpdateEvent: false,
		imageDeleteEvent: false,
	}

	for _, filter := range c.instance.ContainerdFilters {
		strippedFilter := strings.Trim(strings.TrimPrefix(filter, "topic=="), `"`)
		if strippedFilter == imageWildcardEvent {
			return true
		}
		if _, ok := hasImageEvents[strippedFilter]; ok {
			hasImageEvents[strippedFilter] = true
		}
	}
	for _, included := range hasImageEvents {
		if !included {
			return false
		}
	}
	return true
}

func (c *ContainerdCheck) periodicImageSizeQuery() {
	ticker := time.NewTicker(imageSizeQueryInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := c.initializeImageCache(); err != nil {
			log.Warnf("Failed to refresh image size cache: %v", err)
		}
	}
}
