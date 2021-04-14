package host

import (
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/gce"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	k8s "github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	retrySleepTime = time.Second
)

// this is a "low-tech" version of tagger/utils/taglist.go
// but host tags are handled separately here for now
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

		for _, elt := range strings.Split(value, sep) {
			target = append(target, fmt.Sprintf("%s:%s", name, elt))
		}
	}

	return target
}

// GetHostTags get the host tags, optionally looking in the cache
func GetHostTags(cached bool) *Tags {

	key := buildKey("hostTags")
	if cached {
		if x, found := cache.Cache.Get(key); found {
			tags := x.(*Tags)
			return tags
		}
	}

	splits := config.Datadog.GetStringMapString("tag_value_split_separator")
	appendToHostTags := func(old, new []string) []string {
		return appendAndSplitTags(old, new, splits)
	}

	rawHostTags := config.GetConfiguredTags(false)
	hostTags := make([]string, 0, len(rawHostTags))
	hostTags = appendToHostTags(hostTags, rawHostTags)

	env := config.Datadog.GetString("env")
	if env != "" {
		hostTags = appendToHostTags(hostTags, []string{"env:" + env})
	}

	hostname, _ := util.GetHostname()
	clusterName := clustername.GetClusterName(hostname)
	if len(clusterName) != 0 {
		clusterNameTags := []string{"kube_cluster_name:" + clusterName}
		if !config.Datadog.GetBool("disable_cluster_name_tag_key") {
			clusterNameTags = append(clusterNameTags, "cluster_name:"+clusterName)
			log.Info("Adding both tags cluster_name and kube_cluster_name. You can use 'disable_cluster_name_tag_key' in the Agent config to keep the kube_cluster_name tag only")
		}
		hostTags = appendToHostTags(hostTags, clusterNameTags)
	}

	getEC2 := func() ([]string, error) {
		if config.Datadog.GetBool("collect_ec2_tags") {
			return ec2.GetTags()
		}
		return nil, nil
	}

	gceTags := []string{}
	getGCE := func() ([]string, error) {
		if config.Datadog.GetBool("collect_gce_tags") {
			rawGceTags, err := gce.GetTags()
			if err != nil {
				return nil, err
			}
			gceTags = appendToHostTags(gceTags, rawGceTags)
		}
		return nil, nil
	}

	providers := map[string]*struct {
		retries   int
		getTags   func() ([]string, error)
		retrieved bool
	}{
		"ec2":        {1, getEC2, false},
		"kubernetes": {1, k8s.GetTags, false},
		"docker":     {1, docker.GetTags, false},
		"gce":        {1, getGCE, false},
	}

	if config.IsKubernetes() {
		providers["kubernetes"].retries = 10
	}

	for {
		for name, provider := range providers {
			provider.retries--
			tags, err := provider.getTags()
			if err != nil {
				log.Debugf("No %s host tags, remaining attempts: %d, err: %v", name, provider.retries, err)
			} else {
				provider.retrieved = true
				hostTags = appendToHostTags(hostTags, tags)
				log.Debugf("Host tags from %s retrieved successfully", name)
			}

			if provider.retrieved || provider.retries <= 0 {
				delete(providers, name)
			}
		}

		if len(providers) == 0 {
			break
		}

		time.Sleep(retrySleepTime)
	}

	t := &Tags{
		System:              util.SortUniqInPlace(hostTags),
		GoogleCloudPlatform: gceTags,
	}

	cache.Cache.Set(key, t, cache.NoExpiration)
	return t

}
