// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package vdi

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/pdhutil"
	vdimodel "github.com/DataDog/datadog-agent/pkg/vdi/model"
)

const (
	defaultStaleTTL = 300 * time.Second
)

var (
	connectionPattern = regexp.MustCompile(`^(.*):([0-9]+)$`)
	channelPattern    = regexp.MustCompile(`^(.*):([0-9]+):(.+)$`)
	userAgentPattern  = regexp.MustCompile(`(?i)^DCV Client \(([^)]+)\),\s*System:\s*([[:alnum:]_.-]+).*\b(arm64|aarch64|x86_64|amd64)\b`)
)

type awsWorkSpacesConfig struct {
	Product string `yaml:"product"`
}

type instanceConfig struct {
	Provider           string               `yaml:"provider"`
	AWSWorkSpaces      *awsWorkSpacesConfig `yaml:"aws_workspaces"`
	CollectUserTags    *bool                `yaml:"collect_user_tags"`
	CollectSessionTags *bool                `yaml:"collect_session_tags"`
	InventoryStaleTTL  *int                 `yaml:"inventory_stale_ttl"`
}

type inventoryClient interface {
	Get() (vdimodel.InventoryResponse, error)
}

type systemProbeInventoryClient struct {
	client *sysprobeclient.CheckClient
}

func (c *systemProbeInventoryClient) Get() (vdimodel.InventoryResponse, error) {
	return sysprobeclient.GetCheck[vdimodel.InventoryResponse](c.client, sysconfig.VDIModule)
}

type registeredCounter struct {
	object objectDefinition
	def    counterDefinition
	single pdhutil.PdhSingleInstanceCounter
	multi  pdhutil.PdhMultiInstanceCounter
}

type connectionKey struct {
	sessionID    string
	connectionID string
}

type checkImpl struct {
	core.CheckBase
	config          instanceConfig
	query           *pdhutil.PdhQuery
	counters        []registeredCounter
	inventoryClient inventoryClient
	now             func() time.Time
	startedAt       time.Time
	lastFreshAt     time.Time
}

// Factory creates a VDI check on Windows.
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	now := time.Now
	return &checkImpl{
		CheckBase: core.NewCheckBase(CheckName),
		now:       now,
		startedAt: now(),
	}
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
	if c.config.Provider != vdimodel.ProviderAWSWorkSpaces {
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
	if c.config.InventoryStaleTTL != nil && *c.config.InventoryStaleTTL < 0 {
		return errors.New("inventory_stale_ttl must not be negative")
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
					counter.multi = c.query.AddOptionalEnglishMultiInstanceCounter(object.object, definition.counter, isNotTotal)
				} else {
					counter.multi = c.query.AddEnglishMultiInstanceCounter(object.object, definition.counter, isNotTotal)
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
	c.inventoryClient = &systemProbeInventoryClient{client: sysprobeclient.GetCheckClient(
		sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
	)}
	c.startedAt = c.now()
	return nil
}

// Run collects DCV counters and enriches them with fresh desktop inventory.
func (c *checkImpl) Run() error {
	s, err := c.GetSender()
	if err != nil {
		return err
	}
	defer s.Commit()

	now := c.now().UTC()
	inventory, inventoryErr := c.inventoryClient.Get()
	providerInventory, providerPresent := inventory.Providers[vdimodel.ProviderAWSWorkSpaces]
	freshInventory := inventoryErr == nil && providerPresent && providerInventory.Status != vdimodel.StatusError
	if freshInventory {
		c.lastFreshAt = now
	}
	connections := indexConnections(providerInventory, freshInventory)
	sessionIDs := indexSessionIDs(providerInventory, freshInventory)

	pdhHealthy := true
	usableSamples := 0
	discovered := make(map[connectionKey]struct{})
	for key := range connections {
		discovered[key] = struct{}{}
	}
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
					tags, key := c.tagsForInstance(counter.object.object, instance, connections, sessionIDs)
					if key != nil {
						discovered[*key] = struct{}{}
					}
					submitMetric(s, counter.def.kind, metricName, value, tags)
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

	c.submitInventoryMetrics(s, now, providerInventory, inventoryErr, freshInventory, connections, discovered)
	return nil
}

func (c *checkImpl) submitInventoryMetrics(s sender.Sender, now time.Time, provider vdimodel.ProviderInventory, inventoryErr error, fresh bool, connections map[connectionKey]vdimodel.Connection, discovered map[connectionKey]struct{}) {
	tags := c.baseTags()
	sessionCount := 0
	if fresh {
		sessionCount = len(provider.Sessions)
	}
	enriched := 0
	for key := range discovered {
		if connection, found := connections[key]; found && connection.AuthenticatedUser != "" {
			enriched++
		}
	}
	coverage := 100.0
	if len(discovered) > 0 {
		coverage = float64(enriched) / float64(len(discovered)) * 100
	}
	s.Gauge("vdi.session.count", float64(sessionCount), "", tags)
	s.Gauge("vdi.connection.discovered", float64(len(discovered)), "", tags)
	s.Gauge("vdi.connection.enriched", float64(enriched), "", tags)
	s.Gauge("vdi.connection.enrichment_coverage", coverage, "", tags)

	if fresh {
		for _, session := range provider.Sessions {
			for _, connection := range session.Connections {
				connectionTags := c.connectionTags(session.ID, connection)
				s.Gauge("vdi.connection.connected", 1, "", connectionTags)
				if connection.LastInteractionAt != nil {
					idle := now.Sub(*connection.LastInteractionAt).Seconds()
					if idle >= 0 {
						s.Gauge("vdi.connection.idle_time", idle, "", connectionTags)
					}
				}
				if connection.ConnectedAt != nil && connection.FirstFrameAt != nil {
					firstFrame := connection.FirstFrameAt.Sub(*connection.ConnectedAt).Seconds()
					if firstFrame >= 0 {
						s.Gauge("vdi.connection.time_to_first_frame", firstFrame, "", connectionTags)
					}
				}
			}
		}
	}

	status := servicecheck.ServiceCheckOK
	message := ""
	if inventoryErr != nil {
		status = c.unavailableInventoryStatus(now)
		message = truncateMessage(inventoryErr.Error())
	} else if !fresh {
		status = c.unavailableInventoryStatus(now)
		message = truncateMessage(provider.Error)
	} else if provider.Status == vdimodel.StatusPartial || enriched != len(discovered) {
		status = servicecheck.ServiceCheckWarning
		message = truncateMessage(provider.Error)
		if message == "" {
			message = "Some VDI connections could not be enriched"
		}
	}
	s.ServiceCheck("vdi.session_enrichment.health", status, "", tags, message)
}

func (c *checkImpl) unavailableInventoryStatus(now time.Time) servicecheck.ServiceCheckStatus {
	staleTTL := defaultStaleTTL
	if c.config.InventoryStaleTTL != nil {
		staleTTL = time.Duration(*c.config.InventoryStaleTTL) * time.Second
	}
	reference := c.lastFreshAt
	if reference.IsZero() {
		reference = c.startedAt
	}
	if now.Sub(reference) > staleTTL {
		return servicecheck.ServiceCheckCritical
	}
	return servicecheck.ServiceCheckWarning
}

func indexConnections(provider vdimodel.ProviderInventory, fresh bool) map[connectionKey]vdimodel.Connection {
	connections := make(map[connectionKey]vdimodel.Connection)
	if !fresh {
		return connections
	}
	for _, session := range provider.Sessions {
		for _, connection := range session.Connections {
			connections[connectionKey{sessionID: session.ID, connectionID: connection.ID}] = connection
		}
	}
	return connections
}

func indexSessionIDs(provider vdimodel.ProviderInventory, fresh bool) map[string]struct{} {
	sessions := make(map[string]struct{})
	if !fresh {
		return sessions
	}
	for _, session := range provider.Sessions {
		sessions[session.ID] = struct{}{}
	}
	return sessions
}

func isNotTotal(instance string) bool {
	return !strings.EqualFold(instance, "_Total")
}

func (c *checkImpl) tagsForInstance(objectName, instance string, connections map[connectionKey]vdimodel.Connection, sessionIDs map[string]struct{}) ([]string, *connectionKey) {
	tags := c.baseTags()
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
		tags = c.sessionTags(tags, instance)
	case "DCV Server Connections":
		match := connectionPattern.FindStringSubmatch(instance)
		if match != nil {
			key := connectionKey{sessionID: match[1], connectionID: match[2]}
			tags = c.tagsForConnection(tags, key, connections)
			return deduplicate(tags), &key
		}
	case "DCV Server Channels":
		match := channelPattern.FindStringSubmatch(instance)
		if match != nil {
			key := connectionKey{sessionID: match[1], connectionID: match[2]}
			tags = c.tagsForConnection(tags, key, connections)
			tags = append(tags, "dcv_channel:"+match[3])
		}
	case "DCV Server Imaging":
		sessionID, encoder := matchImagingInstance(instance, sessionIDs)
		tags = c.sessionTags(tags, sessionID)
		if encoder != "" {
			tags = append(tags, "dcv_encoder:"+encoder)
		}
	}
	return deduplicate(tags), nil
}

func (c *checkImpl) sessionTags(tags []string, sessionID string) []string {
	if c.collectSessionTags() {
		tags = append(tags, "vdi_session_id:"+sessionID)
	}
	return tags
}

func (c *checkImpl) tagsForConnection(tags []string, key connectionKey, connections map[connectionKey]vdimodel.Connection) []string {
	if c.collectSessionTags() {
		tags = append(tags, "vdi_session_id:"+key.sessionID, "vdi_connection_id:"+key.connectionID)
	}
	if connection, found := connections[key]; found {
		tags = append(tags, c.connectionMetadataTags(connection)...)
	}
	return tags
}

func (c *checkImpl) connectionTags(sessionID string, connection vdimodel.Connection) []string {
	tags := c.baseTags()
	if c.collectSessionTags() {
		tags = append(tags, "vdi_session_id:"+sessionID, "vdi_connection_id:"+connection.ID)
	}
	return deduplicate(append(tags, c.connectionMetadataTags(connection)...))
}

func (c *checkImpl) connectionMetadataTags(connection vdimodel.Connection) []string {
	var tags []string
	if c.collectUserTags() && connection.AuthenticatedUser != "" {
		tags = append(tags, "vdi_connection_user:"+connection.AuthenticatedUser)
	}
	if connection.Transport != "" {
		tags = append(tags, "dcv_transport:"+connection.Transport)
	}
	if connection.ClientMode != "" {
		tags = append(tags, "dcv_client_mode:"+connection.ClientMode)
	}
	version, osName, arch := parseUserAgent(connection.UserAgent)
	if version != "" {
		tags = append(tags, "dcv_client_version:"+version)
	}
	if osName != "" {
		tags = append(tags, "dcv_client_os:"+osName)
	}
	if arch != "" {
		tags = append(tags, "dcv_client_arch:"+arch)
	}
	return tags
}

func (c *checkImpl) baseTags() []string {
	tags := []string{"vdi_provider:" + vdimodel.ProviderAWSWorkSpaces, "vdi_protocol:" + vdimodel.ProtocolDCV}
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

func (c *checkImpl) collectUserTags() bool {
	return c.config.CollectUserTags == nil || *c.config.CollectUserTags
}

func (c *checkImpl) collectSessionTags() bool {
	return c.config.CollectSessionTags == nil || *c.config.CollectSessionTags
}

func matchImagingInstance(instance string, sessionIDs map[string]struct{}) (string, string) {
	ordered := make([]string, 0, len(sessionIDs))
	for session := range sessionIDs {
		ordered = append(ordered, session)
	}
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i]) > len(ordered[j]) })
	for _, session := range ordered {
		if instance == session {
			return session, ""
		}
		if strings.HasPrefix(instance, session+":") {
			return session, strings.TrimPrefix(instance, session+":")
		}
	}
	parts := strings.SplitN(instance, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return instance, ""
}

func parseUserAgent(userAgent string) (string, string, string) {
	match := userAgentPattern.FindStringSubmatch(userAgent)
	if match == nil {
		return "", "", ""
	}
	arch := strings.ToLower(match[3])
	if arch == "aarch64" {
		arch = "arm64"
	} else if arch == "x86_64" {
		arch = "amd64"
	}
	return match[1], strings.ToLower(match[2]), arch
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

func truncateMessage(message string) string {
	const maxMessageLength = 1024
	if len(message) <= maxMessageLength {
		return message
	}
	return message[:maxMessageLength]
}
