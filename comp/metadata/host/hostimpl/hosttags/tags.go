// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hosttags provides access to host tags
package hosttags

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	gpu "github.com/DataDog/datadog-agent/pkg/gpu/tags"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	ec2tags "github.com/DataDog/datadog-agent/pkg/util/ec2/tags"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/cloudprovider"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clusterinfo"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	k8s "github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/sort"
)

var (
	retrySleepTime              = 1 * time.Second
	getProvidersDefinitionsFunc = getProvidersDefinitions

	tagsCacheKey = cache.BuildAgentKey("host", "utils", "hostTags")
)

// Tags contains the detected host tags
type Tags struct {
	System              []string `json:"system"`
	GoogleCloudPlatform []string `json:"google cloud platform,omitempty"`
}

type providerDef struct {
	retries int
	getTags func(context.Context) ([]string, error)
}

func getProvidersDefinitions(conf model.Reader) map[string]*providerDef {
	providers := make(map[string]*providerDef)

	if conf.GetBool("collect_gce_tags") {
		providers["gce"] = &providerDef{1, gce.GetTags}
	}

	if conf.GetBool("collect_ec2_tags") {
		// WARNING: if this config is enabled on a non-ec2 host, then its
		// retries may time out, causing a 3s delay
		providers["ec2"] = &providerDef{10, ec2tags.GetTags}
	}

	if conf.GetBool("collect_ec2_instance_info") {
		providers["ec2_instance_info"] = &providerDef{3, ec2tags.GetInstanceInfo}
	}

	if env.IsFeaturePresent(env.Kubernetes) {
		providers["kubernetes"] = &providerDef{10, k8s.NewKubeNodeTagsProvider(conf).GetTags}
		providers["kubernetes_cluster_agent_tags"] = &providerDef{10, clusterinfo.GetClusterAgentStaticTags}
	}

	if env.IsFeaturePresent(env.Docker) {
		providers["docker"] = &providerDef{1, docker.GetTags}
	}
	return providers
}

// this is a "low-tech" version of tagger/utils/taglist.go but host tags are handled separately here for now
func appendAndSplitTags(target []string, tags []string, splits map[string]string) []string {
	if len(splits) == 0 {
		return append(target, tags...)
	}

	for _, tag := range tags {
		tagParts := strings.SplitN(tag, ":", 2)
		if len(tagParts) != 2 {
			target = append(target, tag)
			continue
		}
		name := tagParts[0]
		value := tagParts[1]

		sep, ok := splits[name]
		if !ok {
			target = append(target, tag)
			continue
		}

		for elt := range strings.SplitSeq(value, sep) {
			target = append(target, fmt.Sprintf("%s:%s", name, elt))
		}
	}
	return target
}

// Get the host tags, optionally looking in the cache
// There are two levels of caching:
// - First one controlled by `cached` boolean, used for performances (cache all tags)
// - Second one per provider, to avoid missing host tags for 30 minutes when a component fails (for instance, Cluster Agent).
// This second layer is always on.
func Get(ctx context.Context, cached bool, conf model.Reader) *Tags {
	if cached {
		if x, found := cache.Cache.Get(tagsCacheKey); found {
			tags := x.(*Tags)
			return tags
		}
	}

	splits := conf.GetStringMapString("tag_value_split_separator")
	appendToHostTags := func(old, new []string) []string {
		return appendAndSplitTags(old, new, splits)
	}

	configTags := configUtils.GetConfiguredTags(conf, false)
	hostTags := make([]string, 0, len(configTags))
	hostTags = appendToHostTags(hostTags, configTags)

	env := conf.GetString("env")
	if env != "" {
		hostTags = appendToHostTags(hostTags, []string{"env:" + env})
	}

	if infraMode := conf.GetString("infrastructure_mode"); infraMode != "" && infraMode != "full" {
		hostTags = appendToHostTags(hostTags, []string{"infra_mode:" + infraMode})
	}

	gpuTags := conf.GetBool("collect_gpu_tags")
	if gpuTags {
		hostTags = appendToHostTags(hostTags, gpu.GetTags())
	}

	hname, _ := hostname.Get(ctx)
	clusterName := clustername.GetClusterNameTagValue(ctx, hname)
	if clusterName != "" {
		clusterNameTags := []string{tags.KubeClusterName + ":" + clusterName}
		if !conf.GetBool("disable_cluster_name_tag_key") {
			clusterNameTags = append(clusterNameTags, tags.ClusterName+":"+clusterName)
			log.Info("Adding both tags cluster_name and kube_cluster_name. You can use 'disable_cluster_name_tag_key' in the Agent config to keep the kube_cluster_name tag only")
		}
		hostTags = appendToHostTags(hostTags, clusterNameTags)
	}

	if kubeDistro, err := cloudprovider.GetName(ctx); err == nil && kubeDistro != "" {
		hostTags = appendToHostTags(hostTags, []string{tags.KubeDistribution + ":" + kubeDistro})
	}

	gceTags := []string{}
	providers := getProvidersDefinitionsFunc(conf)
	for {
		for name, provider := range providers {
			provider.retries--
			providerCacheKey := cache.BuildAgentKey("provider-" + name)
			tags, err := provider.getTags(ctx)
			if err == nil {
				cache.Cache.Set(providerCacheKey, tags, cache.NoExpiration)

				// We store GCE tags separately
				if name == "gce" {
					gceTags = appendToHostTags(gceTags, tags)
				} else {
					hostTags = appendToHostTags(hostTags, tags)
				}

				delete(providers, name)
				log.Debugf("Host tags from %s retrieved successfully", name)
				continue
			}

			log.Debugf("No %s host tags, remaining attempts: %d, err: %v", name, provider.retries, err)
			if provider.retries <= 0 {
				log.Infof("Unable to get host tags from source: %s - using cached host tags", name)
				if cachedTags, found := cache.Cache.Get(providerCacheKey); found {
					// We store GCE tags separately
					if name == "gce" {
						gceTags = appendToHostTags(gceTags, cachedTags.([]string))
					} else {
						hostTags = appendToHostTags(hostTags, cachedTags.([]string))
					}
				}

				delete(providers, name)
			}
		}

		if len(providers) == 0 {
			break
		}

		time.Sleep(retrySleepTime)
	}

	t := &Tags{
		System:              sort.UniqInPlace(hostTags),
		GoogleCloudPlatform: gceTags,
	}

	cache.Cache.Set(tagsCacheKey, t, cache.NoExpiration)
	return t
}
