// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustername provides helpers to get a Kubernetes cluster name.
package clustername

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	ec2tags "github.com/DataDog/datadog-agent/pkg/util/ec2/tags"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/nodename"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	clusterIDEnv = "DD_ORCHESTRATOR_CLUSTER_ID"

	otelResourceAttributesEnv = "OTEL_RESOURCE_ATTRIBUTES"
	otelClusterNameAttribute  = "k8s.cluster.name"
)

// The cluster name can be up to 40 characters with the following restrictions:
// * Must contain only lowercase letters, numbers, dots, hyphens and underscores.
// * Must start with an alphanumeric character.
// * Must end with an alphanumeric character.
// * Must be FQDN-like, without a trailing period.
var validClusterName = regexp.MustCompile(`^([a-z0-9]([a-z0-9\-_]*[a-z0-9])?\.)*([a-z0-9]([a-z0-9\-_]*[a-z0-9])?)$`)

type clusterNameData struct {
	clusterName string
	initDone    bool
	mutex       sync.Mutex
}

// Provider is a generic function to grab the clustername and return it
type Provider func(context.Context) (string, error)

// ProviderCatalog holds all the various kinds of clustername providers
var ProviderCatalog map[string]Provider

func newClusterNameData() *clusterNameData {
	return &clusterNameData{}
}

var defaultClusterNameData *clusterNameData

// nodeClusterNameLabel returns the cluster name read from the local node's
// labels, which may or may not override clusterName, the one found so far, or
// clusterName itself when no label applies. Tests replace it.
var nodeClusterNameLabel = func(ctx context.Context, clusterName string) (string, error) {
	nodeInfo, err := hostinfo.NewNodeInfo()
	if err != nil {
		return "", err
	}
	return nodeInfo.GetNodeClusterNameLabel(ctx, clusterName)
}

func init() {
	defaultClusterNameData = newClusterNameData()
	ProviderCatalog = map[string]Provider{
		"gce":   gce.GetClusterName,
		"azure": azure.GetClusterName,
		"ec2":   ec2tags.GetClusterName,
	}
}

func getClusterName(ctx context.Context, data *clusterNameData, hostname string) string {
	data.mutex.Lock()
	defer data.mutex.Unlock()

	if !env.IsFeaturePresent(env.Kubernetes) {
		return ""
	}

	if !data.initDone {
		data.clusterName = pkgconfigsetup.Datadog().GetString("cluster_name")
		if data.clusterName != "" {
			log.Infof("Got cluster name %s from config", data.clusterName)
			data.clusterName = validateClusterName(data.clusterName, hostname, "the config")
		}

		// autodiscover clustername through k8s providers' API
		if data.clusterName == "" {
			for cloudProvider, getClusterNameFunc := range ProviderCatalog {
				log.Debugf("Trying to auto discover the cluster name from the %s API...", cloudProvider)
				clusterName, err := getClusterNameFunc(ctx)
				if err != nil {
					log.Debugf("Unable to auto discover the cluster name from the %s API: %s", cloudProvider, err)
					// try the next cloud provider
					continue
				}
				// if the clustername is valid but contains a "_" in the middle of it, we will replace it later such that
				// to make it valid to RFC1123.
				if clusterName != "" {
					log.Infof("Using cluster name %s auto discovered from the %s API", clusterName, cloudProvider)
					if strings.HasSuffix(clusterName, "-") || strings.HasSuffix(clusterName, "_") {
						log.Errorf("Registering an invalid clusterName as they are not allowed to end with `_` or `-`")
					}
					data.clusterName = clusterName
					break
				}
			}
		}

		// Cluster check runners aren't scheduled on a specific node and don't have a
		// reachable local kubelet, so skip the node-label based auto discovery to avoid
		// noisy "Impossible to reach Kubelet" warnings. Neither does otel-agent when its
		// configuration leaves it without kubelet access, where that discovery could
		// only fail, possibly after waiting on connection timeouts.
		var clusterName string
		if !helper.IsCLCRunner(pkgconfigsetup.Datadog()) && !nodename.KubeletDisabled(pkgconfigsetup.Datadog()) {
			var err error
			clusterName, err = nodeClusterNameLabel(ctx, data.clusterName)
			if err != nil {
				log.Debugf("Unable to auto discover the cluster name from node label : %s", err)
			}
		}
		if len(clusterName) > 0 {
			if !IsRFC1123CompliantClusterName(clusterName) {
				log.Warnf("Cluster name \"%s\" is not RFC 1123 compliant, it will be converted, ", clusterName)
			}
			data.clusterName = MakeClusterNameRFC1123Compliant(clusterName)
			log.Infof("Using cluster name %s from the node label", data.clusterName)
		}

		// otel-agent: OTel deployment tools (the OTel Helm charts) and users give
		// the collector its cluster's name in the k8s.cluster.name attribute of
		// its own telemetry resource, which is also where the cluster_name host
		// tag of the telemetry it exports comes from. It comes last, after the
		// cloud providers' APIs and the node labels, because charts fill it with a
		// placeholder by default (unknown_k8s_cluster), which the EKS and AKS node
		// labels wouldn't override. Unlike cluster_name, it is free-form, so it is
		// lowercased before being validated.
		if data.clusterName == "" && flavor.GetFlavor() == flavor.OTelAgent {
			if resourceClusterName, source := otelResourceClusterName(); resourceClusterName != "" {
				resourceClusterName = strings.ToLower(resourceClusterName)
				log.Infof("Got cluster name %s from the %s resource attribute in %s", resourceClusterName, otelClusterNameAttribute, source)
				data.clusterName = validateClusterName(resourceClusterName, hostname, source)
			}
		}

		if data.clusterName != "" {
			if lower := strings.ToLower(data.clusterName); lower != data.clusterName {
				log.Infof("Putting cluster name %q in lowercase, became: %q", data.clusterName, lower)
				data.clusterName = lower
			}
		}

		data.initDone = true
	}

	return data.clusterName
}

// validateClusterName returns clusterName, converted to be RFC 1123 compliant,
// or an empty string, after logging an error, when it isn't a valid cluster
// name. source names where it was read from, for that error.
func validateClusterName(clusterName, hostname, source string) string {
	// the host alias "hostname-clustername" must not exceed 255 chars
	hostAlias := hostname + "-" + clusterName
	if !validClusterName.MatchString(clusterName) || len(hostAlias) > 255 {
		log.Errorf("\"%s\" isn't a valid cluster name. The cluster name can be up to 40 characters with the following restrictions:\n"+
			"\t- must contain only lowercase letters, numbers, dots, hyphens and underscores, \n"+
			"\t- must start with an alphanumeric character, \n"+
			"\t- must end with an alphanumeric character, \n"+
			"\t- must be FQDN-like, without a trailing period, \n"+
			"and \"%s\" must not exceed 255 chars", clusterName, hostAlias)
		log.Errorf("As a consequence, the cluster name provided by %s will be ignored", source)
		return ""
	}
	if !IsRFC1123CompliantClusterName(clusterName) {
		RFC1123CompliantClusterName := MakeClusterNameRFC1123Compliant(clusterName)
		log.Warnf("Cluster name \"%s\" is not RFC 1123 compliant, it will be converted to \"%s\"", clusterName, RFC1123CompliantClusterName)
		return RFC1123CompliantClusterName
	}
	return clusterName
}

// otelResourceClusterName returns the k8s.cluster.name attribute of the
// collector's telemetry resource and where it was read from, resolved as the
// collector does: service.telemetry.resource in the collector config, which
// otel-agent copies to otelcollector.standalone.telemetry_resource_cluster_name,
// takes precedence over OTEL_RESOURCE_ATTRIBUTES.
func otelResourceClusterName() (clusterName, source string) {
	if clusterName = pkgconfigsetup.Datadog().GetString("otelcollector.standalone.telemetry_resource_cluster_name"); clusterName != "" {
		return clusterName, "service.telemetry.resource in the collector config"
	}
	if clusterName = otelResourceAttribute(otelClusterNameAttribute); clusterName != "" {
		return clusterName, "the " + otelResourceAttributesEnv + " environment variable"
	}
	return "", ""
}

// otelResourceAttribute returns the value of the key resource attribute in the
// OTEL_RESOURCE_ATTRIBUTES environment variable, or an empty string when it
// isn't set. It parses the variable as the OTel SDK does (sdk/resource env.go):
// comma-separated key=value pairs, with whitespace trimmed, percent-encoded
// values, and the last pair winning when a key repeats.
func otelResourceAttribute(key string) string {
	var value string
	for _, pair := range strings.Split(os.Getenv(otelResourceAttributesEnv), ",") {
		k, v, found := strings.Cut(pair, "=")
		if !found || strings.TrimSpace(k) != key {
			continue
		}
		v = strings.TrimSpace(v)
		if decoded, err := url.PathUnescape(v); err == nil {
			v = decoded
		}
		value = v
	}
	return value
}

// GetClusterName returns a k8s cluster name if it exists, either directly specified or autodiscovered
func GetClusterName(ctx context.Context, hostname string) string {
	return getClusterName(ctx, defaultClusterNameData, hostname)
}

// GetClusterNameTagValue  a k8s cluster name if it exists, either directly specified or autodiscovered
//
// This function also "normalize" the k8s cluster name if the configuration option
// "enabled_rfc1123_compliant_cluster_name_tag" is set to "true"
// this allow to limit the risk of breaking user that currently rely on previous `kube_cluster_name` tag value.
func GetClusterNameTagValue(ctx context.Context, hostname string) string {
	if pkgconfigsetup.Datadog().GetBool("enabled_rfc1123_compliant_cluster_name_tag") {
		return GetRFC1123CompliantClusterName(ctx, hostname)
	}
	return GetClusterName(ctx, hostname)
}

// IsRFC1123CompliantClusterName check if the clusterName is RFC1123 compliant
// return false if not compliant
func IsRFC1123CompliantClusterName(clusterName string) bool {
	return !strings.Contains(clusterName, "_")
}

// GetRFC1123CompliantClusterName returns an RFC-1123 compliant k8s cluster
// name if it exists, either directly specified or autodiscovered
func GetRFC1123CompliantClusterName(ctx context.Context, hostname string) string {
	return MakeClusterNameRFC1123Compliant(getClusterName(ctx, defaultClusterNameData, hostname))
}

func resetClusterName(data *clusterNameData) {
	data.mutex.Lock()
	defer data.mutex.Unlock()
	data.initDone = false
}

// ResetClusterName resets the clustername, which allows it to be detected again. Used for tests
func ResetClusterName() {
	resetClusterName(defaultClusterNameData)
}

// GetClusterID returns the Kubernetes cluster ID.
// The Cluster Agent gets it from Kubernetes. Other agents use the
// DD_ORCHESTRATOR_CLUSTER_ID env variable when it is set, otherwise the Cluster Agent API.
func GetClusterID() (string, error) {
	cacheClusterIDKey := cache.BuildAgentKey(constants.ClusterIDCacheKey)
	if cachedClusterID, found := cache.Cache.Get(cacheClusterIDKey); found {
		return cachedClusterID.(string), nil
	}

	// Do not call the Cluster Agent service from a Cluster Agent: a follower can
	// call itself or another replica that is not ready.
	if flavor.GetFlavor() == flavor.ClusterAgent {
		return getClusterAgentClusterID()
	}

	// in older setups the cluster ID was exposed as an env var from a configmap created by the cluster agent
	clusterID, found := os.LookupEnv(clusterIDEnv)
	if !found {
		log.Debugf("Cluster ID env variable %s is missing, calling the Cluster Agent", clusterIDEnv)

		dcaClient, err := clusteragent.GetClusterAgentClient()
		if err != nil {
			return "", err
		}
		clusterID, err = dcaClient.GetKubernetesClusterID()
		if err != nil {
			return "", err
		}
		log.Debugf("Cluster ID retrieved from the Cluster Agent, set to %s", clusterID)
	}

	if len(clusterID) != 36 {
		err := fmt.Errorf("Unexpected value for Cluster ID: %s, ignoring it", clusterID)
		return "", err
	}

	cache.Cache.Set(cacheClusterIDKey, clusterID, cache.NoExpiration)
	return clusterID, nil
}

// MakeClusterNameRFC1123Compliant returns the RFC-1123 compliant cluster name.
func MakeClusterNameRFC1123Compliant(clusterName string) string {
	if strings.Contains(clusterName, "_") {
		return strings.ReplaceAll(clusterName, "_", "-")
	}
	return clusterName
}

// setProviderCatalog should only be used for testing.
func setProviderCatalog(catalog map[string]Provider) {
	ProviderCatalog = catalog
}
