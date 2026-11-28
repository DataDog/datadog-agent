// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package vdi

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
	"github.com/DataDog/datadog-agent/pkg/vdi/model"
)

func (c *checkImpl) runAVD(s sender.Sender) error {
	inventory, err := c.inventoryClient.Get()
	now := c.now().UTC()
	provider, present := inventory.Providers[model.ProviderAzureVirtualDesktop]
	validStatus := provider.Status == model.StatusOK || provider.Status == model.StatusPartial
	// Never attach identities from a stale response or interpret a failed
	// enumeration as an empty desktop host.
	fresh := !inventory.CollectedAt.IsZero() && now.Sub(inventory.CollectedAt) <= time.Minute && inventory.CollectedAt.Sub(now) <= 5*time.Second
	if err != nil || !present || !validStatus || !fresh {
		message := "AVD inventory is missing, invalid, or stale"
		if err != nil {
			message = fmt.Sprintf("AVD inventory unavailable: %v", err)
		} else if provider.Error != "" {
			message = provider.Error
		}
		c.Warnf("%s", message)
		s.ServiceCheck("vdi.session_enrichment.health", c.unavailableInventoryStatus(now), "", c.baseTags(), utilstrings.TruncateUTF8(message, 1024))
		return nil
	}

	status := servicecheck.ServiceCheckOK
	if provider.Status == model.StatusPartial {
		status = servicecheck.ServiceCheckWarning
		c.Warnf("AVD inventory is incomplete: %s", provider.Error)
	} else {
		c.lastFreshAt = now
		active, disconnected := 0, 0
		for _, session := range provider.Sessions {
			switch session.State {
			case "active":
				active++
			case "disconnected":
				disconnected++
			}
		}
		s.Gauge("vdi.session.count", float64(len(provider.Sessions)), "", c.baseTags())
		s.Gauge("vdi.session.active", float64(active), "", c.baseTags())
		s.Gauge("vdi.session.disconnected", float64(disconnected), "", c.baseTags())
	}

	// A session identifier is required to keep per-session series separate.
	// Aggregate counts remain available when session tags are disabled.
	if c.collectSessionTags() {
		for _, session := range provider.Sessions {
			tags := append(c.baseTags(), "vdi_session_id:"+session.ID, "vdi_session_state:"+session.State)
			if c.collectUserTags() && session.User != "" {
				tags = append(tags, "vdi_session_user:"+session.User)
			}
			s.Gauge("vdi.session.present", 1, "", tags)
			if session.LogonAt != nil && !session.LogonAt.After(inventory.CollectedAt) {
				s.Gauge("vdi.session.duration", inventory.CollectedAt.Sub(*session.LogonAt).Seconds(), "", tags)
			}
			if session.LastInputAt != nil && !session.LastInputAt.After(inventory.CollectedAt) {
				s.Gauge("vdi.session.idle_time", inventory.CollectedAt.Sub(*session.LastInputAt).Seconds(), "", tags)
			}
		}
	}
	s.ServiceCheck("vdi.session_enrichment.health", status, "", c.baseTags(), utilstrings.TruncateUTF8(provider.Error, 1024))
	return nil
}

const defaultStaleTTL = 300 * time.Second

type inventoryClient interface {
	Get() (model.InventoryResponse, error)
}

type systemProbeInventoryClient struct{ client *sysprobeclient.CheckClient }

func newSystemProbeInventoryClient() *systemProbeInventoryClient {
	return &systemProbeInventoryClient{client: sysprobeclient.GetCheckClient(
		sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
	)}
}

func (c *systemProbeInventoryClient) Get() (model.InventoryResponse, error) {
	return sysprobeclient.GetCheck[model.InventoryResponse](c.client, sysconfig.VDIModule)
}

func (c *checkImpl) collectUserTags() bool {
	return c.config.CollectUserTags == nil || *c.config.CollectUserTags
}

func (c *checkImpl) collectSessionTags() bool {
	return c.config.CollectSessionTags == nil || *c.config.CollectSessionTags
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
