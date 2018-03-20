// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package legacy

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	"github.com/DataDog/datadog-agent/pkg/config"

	yaml "gopkg.in/yaml.v2"
)

const (
	warningNewKubeCheck       string = "Warning: The Kubernetes integration has been overhauled, please see https://github.com/DataDog/datadog-agent/blob/master/docs/agent/changes.md#kubernetes-support"
	deprecationAPIServerCreds string = "please use kubernetes_kubeconfig_path instead"
	deprecationHisto          string = "please contact support to determine the best alternative for you"
	deprecationFiltering      string = "Agent6 now collects all available namespaces and metrics"
)

type legacyKubernetesInstance struct {
	KubeletPort      int               `yaml:"kubelet_port"`
	KubeletHost      string            `yaml:"host"`
	KubeletClientCrt string            `yaml:"kubelet_client_crt"`
	KubeletClientKey string            `yaml:"kubelet_client_key"`
	KubeletCACert    string            `yaml:"kubelet_cert"`
	KubeletTokenPath string            `yaml:"bearer_token_path"`
	KubeletTLSVerify string            `yaml:"kubelet_tls_verify"`
	NodeLabelsToTags map[string]string `yaml:"node_labels_to_host_tags"`

	CollectEvents       bool `yaml:"collect_events"`
	LeaderCandidate     bool `yaml:"leader_candidate"`
	LeaderLeaseDuration int  `yaml:"leader_lease_duration"`
	CollectServiceTags  bool `yaml:"collect_service_tags"`
	ServiceTagUpdateTag int  `yaml:"service_tag_update_freq"`

	// Deprecated
	APIServerURL       string   `yaml:"api_server_url"`
	APIServerClientCrt string   `yaml:"apiserver_client_crt"`
	APIServerClientKey string   `yaml:"apiserver_client_key"`
	APIServerCACert    string   `yaml:"apiserver_ca_cert"`
	Namespaces         []string `yaml:"namespaces"`
	NamespacesRegexp   string   `yaml:"namespace_name_regexp"`
	Rates              []string `yaml:"enabled_rates"`
	Gauges             []string `yaml:"enabled_gauges"`
	UseHisto           bool     `yaml:"use_histogram"`

	Tags []string `yaml:"tags"`
}

type newKubeletInstance struct {
	Tags []string `yaml:"tags"`
}

type kubeDeprecations map[string][]string

func (k kubeDeprecations) add(field, message string) {
	k[message] = append(k[message], field)
}

func (k kubeDeprecations) print() {
	if len(k) == 0 {
		return
	}
	fmt.Println("The following fields are deprecated and not converted:")
	for msg, fields := range k {
		fmt.Printf("  - %s: %s\n", strings.Join(fields, ", "), msg)
	}
}

// ImportKubernetesConf reads the configuration from the kubernetes check (agent5)
// and create the configuration for the new kubelet check (agent 6) and moves
// relevant options to datadog.yaml
func ImportKubernetesConf(src, dst string, overwrite bool) error {
	_, err := importKubernetesConfWithDeprec(src, dst, overwrite)
	return err
}

// Deprecated options are listed in the kubeDeprecations return value, for testing
func importKubernetesConfWithDeprec(src, dst string, overwrite bool) (kubeDeprecations, error) {
	fmt.Printf("%s\n", warningNewKubeCheck)
	deprecations := make(kubeDeprecations)

	// read kubernetes.yaml
	c, err := providers.GetCheckConfigFromFile("kubernetes", src)
	if err != nil {
		return deprecations, fmt.Errorf("Could not load %s: %s", src, err)
	}

	if len(c.Instances) == 0 {
		return deprecations, nil
	}
	if len(c.Instances) > 1 {
		fmt.Printf("Warning: %s contains more than one instance: converting only the first one", src)
	}

	// kubelet.yaml (only tags for now)
	newKube := &newKubeletInstance{}
	if err := yaml.Unmarshal(c.Instances[0], newKube); err != nil {
		return deprecations, fmt.Errorf("Could not parse instance from %s: %s", src, err)
	}
	data, err := yaml.Marshal(newKube)
	if err != nil {
		return deprecations, fmt.Errorf("Could not marshall final configuration for the new kubelet check: %s", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		if overwrite {
			// we'll overwrite, backup the original file first
			err = os.Rename(dst, dst+".bak")
			if err != nil {
				return deprecations, fmt.Errorf("unable to create a backup copy of the destination file: %v", err)
			}
		} else {
			return deprecations, fmt.Errorf("destination file already exists, run the command again with --force or -f to overwrite it")
		}
	}
	if err := ioutil.WriteFile(dst, data, 0640); err != nil {
		return deprecations, fmt.Errorf("Could not write new kubelet configuration to %s: %s", dst, err)
	}
	fmt.Printf("Successfully imported the contents of %s into %s\n", src, dst)

	// datadog.yaml
	instance := &legacyKubernetesInstance{}
	if err := yaml.Unmarshal(c.Instances[0], instance); err != nil {
		return deprecations, fmt.Errorf("Could not Unmarshal instances from %s: %s", src, err)
	}

	if instance.KubeletPort > 0 {
		config.Datadog.Set("kubernetes_http_kubelet_port", instance.KubeletPort)
		config.Datadog.Set("kubernetes_https_kubelet_port", instance.KubeletPort)
	}
	if len(instance.KubeletHost) > 0 {
		config.Datadog.Set("kubernetes_kubelet_host", instance.KubeletHost)
	}
	if len(instance.KubeletClientCrt) > 0 {
		config.Datadog.Set("kubelet_client_crt", instance.KubeletClientCrt)
	}
	if len(instance.KubeletClientKey) > 0 {
		config.Datadog.Set("kubelet_client_key", instance.KubeletClientKey)
	}
	if len(instance.KubeletCACert) > 0 {
		config.Datadog.Set("kubelet_client_ca", instance.KubeletCACert)
	}
	if len(instance.KubeletTokenPath) > 0 {
		config.Datadog.Set("kubelet_auth_token_path", instance.KubeletTokenPath)
	}
	if len(instance.NodeLabelsToTags) > 0 {
		config.Datadog.Set("kubernetes_node_labels_as_tags", instance.NodeLabelsToTags)
	}

	// We need to verify the kubelet_tls_verify is actually present before
	// changing the secure `true` default
	if verify, err := strconv.ParseBool(instance.KubeletTLSVerify); err == nil {
		config.Datadog.Set("kubelet_tls_verify", verify)
	}

	// Temporarily in main datadog.yaml, will move to DCA
	// Booleans are always imported as zero value is false
	config.Datadog.Set("collect_kubernetes_events", instance.CollectEvents)
	config.Datadog.Set("leader_election", instance.LeaderCandidate)
	config.Datadog.Set("kubernetes_collect_service_tags", instance.CollectServiceTags)

	if instance.LeaderLeaseDuration > 0 {
		config.Datadog.Set("leader_lease_duration", instance.LeaderLeaseDuration)
	}
	if instance.ServiceTagUpdateTag > 0 {
		config.Datadog.Set("kubernetes_service_tag_update_freq", instance.ServiceTagUpdateTag)
	}

	// Deprecations
	if len(instance.APIServerURL) > 0 {
		deprecations.add("api_server_url", deprecationAPIServerCreds)
	}
	if len(instance.APIServerClientCrt) > 0 {
		deprecations.add("apiserver_client_crt", deprecationAPIServerCreds)
	}
	if len(instance.APIServerClientKey) > 0 {
		deprecations.add("apiserver_client_key", deprecationAPIServerCreds)
	}
	if len(instance.APIServerCACert) > 0 {
		deprecations.add("apiserver_ca_cert", deprecationAPIServerCreds)
	}
	if instance.UseHisto {
		deprecations.add("use_histogram", deprecationHisto)
	}
	if len(instance.Namespaces) > 0 {
		deprecations.add("namespaces", deprecationFiltering)
	}
	if len(instance.NamespacesRegexp) > 0 {
		deprecations.add("namespace_name_regexp", deprecationFiltering)
	}
	if len(instance.Rates) > 0 {
		deprecations.add("enabled_rates", deprecationFiltering)
	}
	if len(instance.Gauges) > 0 {
		deprecations.add("enabled_gauges", deprecationFiltering)
	}

	deprecations.print()
	fmt.Printf("Successfully imported the contents of %s into datadog.yaml\n\n", src)

	return deprecations, nil
}
