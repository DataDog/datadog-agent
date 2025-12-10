// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build !serverless

package listeners

import (
	"hash/fnv"
	"maps"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	tolerateUnreadyAnnotation = "ad.datadoghq.com/tolerate-unready"

	// Keys of standard tags
	tagKeyEnv     = "env"
	tagKeyVersion = "version"
	tagKeyService = "service"
)

// getStandardTags extract standard tags from labels of kubernetes services
func getStandardTags(labels map[string]string) []string {
	if labels == nil {
		return []string{}
	}
	labelToTagKeys := map[string]string{
		kubernetes.EnvTagLabelKey:     tagKeyEnv,
		kubernetes.VersionTagLabelKey: tagKeyVersion,
		kubernetes.ServiceTagLabelKey: tagKeyService,
	}
	tags := make([]string, 0, len(labelToTagKeys))
	for labelKey, tagKey := range labelToTagKeys {
		if tagValue, found := labels[labelKey]; found {
			tags = append(tags, tagKey+":"+tagValue)
		}
	}
	return tags
}

// standardTagsDigest computes the hash of standard tags in a map
func standardTagsDigest(labels map[string]string) string {
	if labels == nil {
		return ""
	}
	h := fnv.New64()
	// the implementation of h.Write never returns a non-nil error
	_, _ = h.Write([]byte(labels[kubernetes.EnvTagLabelKey]))
	_, _ = h.Write([]byte(labels[kubernetes.VersionTagLabelKey]))
	_, _ = h.Write([]byte(labels[kubernetes.ServiceTagLabelKey]))
	return strconv.FormatUint(h.Sum64(), 16)
}

// getPrometheusIncludeAnnotations returns the Prometheus AD include annotations based on the Prometheus config
func getPrometheusIncludeAnnotations() types.PrometheusAnnotations {
	annotations := types.PrometheusAnnotations{}
	tmpConfigString := pkgconfigsetup.Datadog().GetString("prometheus_scrape.checks")

	var checks []*types.PrometheusCheck
	if len(tmpConfigString) > 0 {
		var err error
		checks, err = types.PrometheusScrapeChecksTransformer(tmpConfigString)
		if err != nil {
			log.Warnf("Couldn't get configurations from 'prometheus_scrape.checks': %v", err)
			return annotations
		}
	}

	if len(checks) == 0 {
		annotations[types.PrometheusScrapeAnnotation] = "true"
		return annotations
	}

	for _, check := range checks {
		if err := check.Init(pkgconfigsetup.Datadog().GetInt("prometheus_scrape.version")); err != nil {
			log.Errorf("Couldn't init check configuration: %v", err)
			continue
		}
		maps.Copy(annotations, check.AD.GetIncludeAnnotations())
	}
	return annotations
}

// shouldSkipPodReadiness checks if a pod should skip readiness checks based on its annotations
func shouldSkipPodReadiness(pod *workloadmeta.KubernetesPod) bool {
	tolerate, ok := pod.Annotations[tolerateUnreadyAnnotation]
	return ok && tolerate == "true"
}
