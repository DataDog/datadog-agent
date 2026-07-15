// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/ad-misconfiguration"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	//nolint needed as these constants are defined in a file without a build tag,
	// but only used in multiple files with different build tags, none of which
	// are used in the IoT Agent.
	//nolint:unused,deadcode
	instancePath string = "instances"
	//nolint:unused,deadcode
	checkNamePath string = "check_names"
	//nolint:unused,deadcode
	initConfigPath string = "init_configs"
)

func buildStoreKey(key ...string) string {
	parts := []string{pkgconfigsetup.Datadog().GetString("autoconf_template_dir")}
	parts = append(parts, key...)
	return path.Join(parts...)
}

// GetPollInterval computes the poll interval from the config
func GetPollInterval(cp pkgconfigsetup.ConfigurationProviders) time.Duration {
	if cp.PollInterval != "" {
		customInterval, err := time.ParseDuration(cp.PollInterval)
		if err == nil {
			return customInterval
		}
	}
	return pkgconfigsetup.Datadog().GetDuration("ad_config_poll_interval") * time.Second
}

// providerCache supports monitoring a service for changes either to the number
// of things being monitored, or to one of those things being modified.  This
// can be used to determine IsUpToDate() and avoid full Collect() calls when
// nothing has changed.
// nolint needed as this type is defined in a file without a build tag,
// but only used in multiple files with different build tags, none of which
// are used in the IoT Agent.
//
//nolint:unused
type providerCache struct {
	// mostRecentMod is the most recent modification timestamp of a
	// monitored thing
	mostRecentMod float64

	// count is the number of monitored things
	count int
}

// newProviderCache instantiate a ProviderCache.
// nolint needed as this function is defined in a file without a build tag,
// but only used in multiple files with different build tags, none of which
// are used in the IoT Agent.
//
//nolint:deadcode,unused
func newProviderCache() *providerCache {
	return &providerCache{
		mostRecentMod: 0,
		count:         0,
	}
}

// ignoreADTagsFromAnnotations returns of the `ad.datadoghq.com/{endpoints,service}.ignore_autodiscovery_tags` annotation
// TODO(CINT)(Agent 7.53+) Remove support for hybrid scenarios
//
//nolint:deadcode,unused
func ignoreADTagsFromAnnotations(annotations map[string]string, prefix string) bool {
	ignoreAdForHybridScenariosTags, _ := strconv.ParseBool(annotations[prefix+"ignore_autodiscovery_tags"])
	return ignoreAdForHybridScenariosTags
}

// adAnnotationIssueID returns the health-platform issue id for an AD annotation
// error on the given entity. The build and resolve paths must use the same id,
// so both call this helper rather than inlining the string.
func adAnnotationIssueID(entityName string) string {
	return admisconfig.AnnotationIssueID + ":" + entityName
}

// reportConfigurationError reports the AD configuration errors for an entity to
// the health platform. It is shared by the container, service, and endpoint
// config providers so that Autodiscovery annotation misconfigurations surface
// consistently regardless of which provider parsed them.
func reportConfigurationError(hp healthplatformdef.Component, entityName string, errMsgSet types.ErrorMsgSet, errorSource types.ErrorSource) {
	if hp == nil {
		return
	}

	// Sort error messages for stable issue content across reports.
	errMsgs := make([]string, 0, len(errMsgSet))
	for msg := range errMsgSet {
		errMsgs = append(errMsgs, msg)
	}
	sort.Strings(errMsgs)
	errorMsg := strings.Join(errMsgs, ", ")

	issueID := adAnnotationIssueID(entityName)
	context := map[string]string{
		"entityName":   entityName,
		"errorMessage": errorMsg,
		"errorSource":  string(errorSource),
	}
	issue, buildErr := admisconfig.NewADAnnotationIssue().BuildIssue(context)
	if buildErr != nil {
		issue = &healthplatformpayload.Issue{
			Id:        issueID,
			IssueName: admisconfig.AnnotationIssueName,
			Title:     "Autodiscovery Misconfiguration on '" + entityName + "'",
			Source:    admisconfig.Source,
		}
	} else {
		issue.Id = issueID
	}
	if err := hp.ReportIssue(issue); err != nil {
		log.Debugf("Failed to report AD annotation issue for %s: %v", entityName, err)
	}
}

// clearConfigurationErrors resolves any previously reported AD configuration
// issue for the given entity.
func clearConfigurationErrors(hp healthplatformdef.Component, entityName string) {
	if hp == nil {
		return
	}
	hp.ResolveIssue(adAnnotationIssueID(entityName))
}
