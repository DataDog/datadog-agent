package host

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/gce"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	k8s "github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

func getHostTags() *tags {
	splits := config.Datadog.GetStringMapString("tag_value_split_separator")
	appendToHostTags := func(old, new []string) []string {
		return appendAndSplitTags(old, new, splits)
	}

	rawHostTags := config.Datadog.GetStringSlice("tags")
	hostTags := make([]string, 0, len(rawHostTags))
	hostTags = appendToHostTags(hostTags, rawHostTags)

	env := config.Datadog.GetString("env")
	if env != "" {
		hostTags = appendToHostTags(hostTags, []string{"env:" + env})
	}

	if config.Datadog.GetBool("collect_ec2_tags") {
		ec2Tags, err := ec2.GetTags()
		if err != nil {
			log.Debugf("No EC2 host tags %v", err)
		} else {
			hostTags = appendToHostTags(hostTags, ec2Tags)
		}
	}

	clusterName := clustername.GetClusterName()
	if len(clusterName) != 0 {
		clusterNameTags := []string{"kube_cluster_name:" + clusterName}
		if !config.Datadog.GetBool("disable_cluster_name_tag_key") {
			clusterNameTags = append(clusterNameTags, "cluster_name:"+clusterName)
			log.Info("Adding both tags cluster_name and kube_cluster_name. You can use 'disable_cluster_name_tag_key' in the Agent config to keep the kube_cluster_name tag only")
		}
		hostTags = appendToHostTags(hostTags, clusterNameTags)
	}

	k8sTags, err := k8s.GetTags()
	if err != nil {
		log.Debugf("No Kubernetes host tags %v", err)
	} else {
		hostTags = appendToHostTags(hostTags, k8sTags)
	}

	dockerTags, err := docker.GetTags()
	if err != nil {
		log.Debugf("No Docker host tags %v", err)
	} else {
		hostTags = appendToHostTags(hostTags, dockerTags)
	}

	gceTags := []string{}
	if config.Datadog.GetBool("collect_gce_tags") {
		rawGceTags, err := gce.GetTags()
		if err != nil {
			log.Debugf("No GCE host tags %v", err)
		} else {
			gceTags = appendToHostTags(gceTags, rawGceTags)
		}
	}

	return &tags{
		System:              hostTags,
		GoogleCloudPlatform: gceTags,
	}
}
