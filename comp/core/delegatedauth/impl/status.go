// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package delegatedauthimpl

import (
	"embed"
	"fmt"
	"io"
	"time"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Status Provider implementation for delegated auth

// Name returns the name for status sorting
func (d *delegatedAuthComponent) Name() string {
	return "Delegated Auth"
}

// Section returns the section name for status grouping
func (d *delegatedAuthComponent) Section() string {
	return "delegatedauth"
}

// JSON populates the status stats map
func (d *delegatedAuthComponent) JSON(_ bool, stats map[string]interface{}) error {
	d.populateStatusInfo(stats)
	return nil
}

// Text renders the text status output
func (d *delegatedAuthComponent) Text(_ bool, buffer io.Writer) error {
	stats := make(map[string]interface{})
	d.populateStatusInfo(stats)
	return status.RenderText(templatesFS, "delegatedauth.tmpl", buffer, stats)
}

// HTML renders the HTML status output
func (d *delegatedAuthComponent) HTML(_ bool, buffer io.Writer) error {
	stats := make(map[string]interface{})
	d.populateStatusInfo(stats)
	return status.RenderHTML(templatesFS, "delegatedauthHTML.tmpl", buffer, stats)
}

// populateStatusInfo gathers the current status information for delegated auth
func (d *delegatedAuthComponent) populateStatusInfo(stats map[string]interface{}) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check if delegated auth is enabled (has any configured instances)
	stats["enabled"] = len(d.instances) > 0

	if len(d.instances) == 0 {
		// Distinguish "configured but could not start" from "never configured".
		if d.disabledReason != "" {
			stats["disabledReason"] = d.disabledReason
		}
		return
	}

	// Add resolved provider information
	if d.initialized {
		stats["provider"] = d.resolvedProvider
		// Add provider-specific details
		if awsConfig, ok := d.providerConfig.(*cloudauthconfig.AWSProviderConfig); ok && awsConfig.Region != "" {
			stats["awsRegion"] = awsConfig.Region
		}
	}

	// Add information about each configured instance
	instances := make(map[string]map[string]interface{})
	for key, instance := range d.instances {
		instanceInfo := make(map[string]interface{})

		// Status
		active := instance.apiKey != nil
		if instance.credProvider != nil {
			active = instance.credProvider.hasCredential()
		}
		if active {
			instanceInfo["Status"] = "Active"
		} else {
			instanceInfo["Status"] = "Pending"
		}

		// Refresh interval
		instanceInfo["RefreshInterval"] = instance.refreshInterval.String()

		// Refresh timestamps.
		if !instance.lastRefresh.IsZero() {
			instanceInfo["LastRefresh"] = instance.lastRefresh.Format(time.RFC3339)
		}
		if !instance.nextRefresh.IsZero() {
			instanceInfo["NextRefresh"] = instance.nextRefresh.Format(time.RFC3339)
		}

		// Credential source, reported for failed attempts too.
		if reporter, ok := instance.provider.(common.CredentialSourceReporter); ok {
			if source := reporter.LastCredentialSource(); source != "" {
				instanceInfo["CredentialSource"] = source
			}
		}

		// Additional endpoint domain, if this instance manages a dual-shipping key
		if instance.additionalEndpointDomain != "" {
			instanceInfo["AdditionalEndpointDomain"] = instance.additionalEndpointDomain
		}

		// Error info for consecutive failures.
		if instance.consecutiveFailures > 0 {
			if instance.lastError != nil {
				instanceInfo["Error"] = fmt.Sprintf("%d consecutive failures, last error: %v", instance.consecutiveFailures, instance.lastError)
			} else {
				instanceInfo["Error"] = fmt.Sprintf("%d consecutive failures", instance.consecutiveFailures)
			}
		}

		instances[key] = instanceInfo
	}
	stats["instances"] = instances
}
