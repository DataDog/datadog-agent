// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package legacy

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/config"

	yaml "gopkg.in/yaml.v2"
)

const (
	warningNewKubeCheck       string = "Warning: The Kubernetes integration has been overhauled, please see https://github.com/DataDog/datadog-agent/blob/main/docs/agent/changes.md#kubernetes-support"
	deprecationAPIServerCreds string = "please use kubernetes_kubeconfig_path instead"
	deprecationHisto          string = "please contact support to determine the best alternative for you"
	deprecationFiltering      string = "Agent6 now collects metrics from all available namespaces"
	deprecationTagPrefix      string = "please specify mapping per label via kubernetes_pod_labels_as_tags"
	deprecationCadvisorPort   string = "Agent6 default mode is to collect kubelet metrics via the new prometheus endpoints. On clusters older than 1.7.6, manually set the cadvisor_port setting to enable cadvisor collection"
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

	CollectEvents       bool   `yaml:"collect_events"`
	LeaderCandidate     bool   `yaml:"leader_candidate"`
	LeaderLeaseDuration int    `yaml:"leader_lease_duration"`
	CollectServiceTags  string `yaml:"collect_service_tags"`
	ServiceTagUpdateTag int    `yaml:"service_tag_update_freq"`
	CadvisorPort        string `yaml:"port"`

	// Deprecated
	APIServerURL       string   `yaml:"api_server_url"`
	APIServerClientCrt string   `yaml:"apiserver_client_crt"`
	APIServerClientKey string   `yaml:"apiserver_client_key"`
	APIServerCACert    string   `yaml:"apiserver_ca_cert"`
	Namespaces         []string `yaml:"namespaces"`
	NamespacesRegexp   string   `yaml:"namespace_name_regexp"`
	UseHisto           bool     `yaml:"use_histogram"`
	LabelTagPrefix     string   `yaml:"label_to_tag_prefix"`

	Tags []string `yaml:"tags"`
}

type newKubeletInstance struct {
	CadvisorPort  int      `yaml:"cadvisor_port"` // will default to 0 == disable
	Tags          []string `yaml:"tags,omitempty"`
	EnabledRates  []string `yaml:"enabled_rates,omitempty"`
	EnabledGauges []string `yaml:"enabled_gauges,omitempty"`
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
func ImportKubernetesConf(src, dst string, overwrite bool, converter *config.LegacyConfigConverter) error {
	_, err := importKubernetesConfWithDeprec(src, dst, overwrite, converter)
	return err
}

// Deprecated options are listed in the kubeDeprecations return value, for testing
func importKubernetesConfWithDeprec(src, dst string, overwrite bool, converter *config.LegacyConfigConverter) (kubeDeprecations, error) {
	fmt.Printf("%s\n", warningNewKubeCheck)
	deprecations := make(kubeDeprecations)

	// read kubernetes.yaml
	c, err := providers.GetIntegrationConfigFromFile("kubernetes", src)
	if err != nil {
		return deprecations, fmt.Errorf("Could not load %s: %s", src, err)
	}

	if len(c.Instances) == 0 {
		return deprecations, nil
	}
	if len(c.Instances) > 1 {
		fmt.Printf("Warning: %s contains more than one instance: converting only the first one\n", src)
	}

	// kubelet.yaml (only tags for now)
	newKube := &newKubeletInstance{}
	if err := yaml.Unmarshal(c.Instances[0], newKube); err != nil {
		return deprecations, fmt.Errorf("Could not parse instance from %s: %s", src, err)
	}
	newCfg := map[string][]*newKubeletInstance{
		"instances": {newKube},
	}
	data, err := yaml.Marshal(newCfg)
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
	// Create necessary destination dir
	err = os.MkdirAll(filepath.Dir(dst), 0750)
	if err != nil {
		return deprecations, err
	}
	if err := os.WriteFile(dst, data, 0640); err != nil {
		return deprecations, fmt.Errorf("Could not write new kubelet configuration to %s: %s", dst, err)
	}
	fmt.Printf("Successfully imported the contents of %s into %s\n", src, dst)

	// datadog.yaml
	instance := &legacyKubernetesInstance{}
	if err := yaml.Unmarshal(c.Instances[0], instance); err != nil {
		return deprecations, fmt.Errorf("Could not Unmarshal instances from %s: %s", src, err)
	}

	if instance.KubeletPort > 0 {
		converter.Set("kubernetes_http_kubelet_port", instance.KubeletPort)
		converter.Set("kubernetes_https_kubelet_port", instance.KubeletPort)
	}
	if len(instance.KubeletHost) > 0 {
		converter.Set("kubernetes_kubelet_host", instance.KubeletHost)
	}
	if len(instance.KubeletClientCrt) > 0 {
		converter.Set("kubelet_client_crt", instance.KubeletClientCrt)
	}
	if len(instance.KubeletClientKey) > 0 {
		converter.Set("kubelet_client_key", instance.KubeletClientKey)
	}
	if len(instance.KubeletCACert) > 0 {
		converter.Set("kubelet_client_ca", instance.KubeletCACert)
	}
	if len(instance.KubeletTokenPath) > 0 {
		converter.Set("kubelet_auth_token_path", instance.KubeletTokenPath)
	}
	if len(instance.NodeLabelsToTags) > 0 {
		converter.Set("kubernetes_node_labels_as_tags", instance.NodeLabelsToTags)
	}

	// We need to verify the kubelet_tls_verify is actually present before
	// changing the secure `true` default
	if verify, err := strconv.ParseBool(instance.KubeletTLSVerify); err == nil {
		converter.Set("kubelet_tls_verify", verify)
	}

	// Implicit default in Agent5 was true
	if verify, err := strconv.ParseBool(instance.CollectServiceTags); err == nil {
		converter.Set("kubernetes_collect_service_tags", verify)
	} else {
		converter.Set("kubernetes_collect_service_tags", true)
	}

	// Temporarily in main datadog.yaml, will move to DCA
	// Booleans are always imported as zero value is false
	converter.Set("collect_kubernetes_events", instance.CollectEvents)
	converter.Set("leader_election", instance.LeaderCandidate)

	if instance.LeaderLeaseDuration > 0 {
		converter.Set("leader_lease_duration", instance.LeaderLeaseDuration)
	}
	if instance.ServiceTagUpdateTag > 0 {
		converter.Set("kubernetes_service_tag_update_freq", instance.ServiceTagUpdateTag)
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
	if len(instance.LabelTagPrefix) > 0 {
		deprecations.add("label_to_tag_prefix", deprecationTagPrefix)
	}
	if len(instance.CadvisorPort) > 0 {
		deprecations.add("port", deprecationCadvisorPort)
	}

	deprecations.print()
	fmt.Printf("Successfully imported the contents of %s into datadog.yaml\n\n", src)

	return deprecations, nil
}
