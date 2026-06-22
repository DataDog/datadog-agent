// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"sort"
	"strings"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/admisconfig"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// adAnnotationIssueID returns the health-platform issue id for an AD annotation
// error on the given entity. The build and resolve paths must use the same id,
// so both call this helper rather than inlining the string.
func adAnnotationIssueID(entityName string) string {
	return "ad-annotation:" + entityName
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
	issue, buildErr := admisconfig.NewADMisconfigurationIssue().BuildIssue(context)
	if buildErr != nil {
		issue = &healthplatformpayload.Issue{
			Id:        issueID,
			IssueName: healthplatformdef.ADMisconfigurationIssueName,
			Title:     "Autodiscovery Misconfiguration on '" + entityName + "'",
			Source:    healthplatformdef.ADMisconfigurationSource,
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
