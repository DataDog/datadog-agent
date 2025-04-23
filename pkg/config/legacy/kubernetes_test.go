// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package legacy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

const (
	kubernetesLegacyConf string = `
init_config:

instances:
 - port: 4194
   host: localhost

   # Imported to main datadog.yaml
   kubelet_port: 1234
   kubelet_client_crt: /path/to/client.crt
   kubelet_client_key: /path/to/client.key
   kubelet_cert: /path/to/ca.pem
   kubelet_tls_verify: False
   bearer_token_path: /path/to/token
   node_labels_to_host_tags:
     kubernetes.io/hostname: nodename
     beta.kubernetes.io/os: os

   # Temporarily in main datadog.yaml, will move to DCA
   collect_events: true
   leader_candidate: true
   leader_lease_duration: 1200
   #collect_service_tags: false
   service_tag_update_freq: 3000

   # Deprecated: provide a kubeconfig now, will move to DCA anyway
   api_server_url: https://kubernetes:443
   apiserver_client_crt: /path/to/client.crt
   apiserver_client_key: /path/to/client.key
   apiserver_ca_cert: /path/to/cacert.crt

   # Deprecated: we collect everything now
   namespaces:
     - default
   namespace_name_regexp: test

   # Deprecated
   use_histogram: true
   label_to_tag_prefix: "kube_"

   # Move to kubelet.yaml
   tags:
     - optional_tag1
     - optional_tag2
   enabled_rates:
     - cpu.*
     - network.*
   enabled_gauges:
     - filesystem.*
`

	kubernetesLegacyEmptyConf string = `
init_config:

instances:
 - {}
`

	kubeletNewConf string = `instances:
- cadvisor_port: 0
  tags:
  - optional_tag1
  - optional_tag2
  enabled_rates:
  - cpu.*
  - network.*
  enabled_gauges:
  - filesystem.*
`

	kubeletNewEmptyConf string = `instances:
- cadvisor_port: 0
`
)

var expectedKubeDeprecations = kubeDeprecations{
	deprecationAPIServerCreds: []string{"api_server_url", "apiserver_client_crt", "apiserver_client_key", "apiserver_ca_cert"},
	deprecationHisto:          []string{"use_histogram"},
	deprecationFiltering:      []string{"namespaces", "namespace_name_regexp"},
	deprecationTagPrefix:      []string{"label_to_tag_prefix"},
	deprecationCadvisorPort:   []string{"port"},
}

var expectedHostTags = map[string]string{
	"kubernetes.io/hostname": "nodename",
	"beta.kubernetes.io/os":  "os",
}

func TestConvertKubernetes(t *testing.T) {
	cfg := configmock.New(t)
	dir := t.TempDir()

	src := filepath.Join(dir, "kubernetes.yaml")
	srcEmpty := filepath.Join(dir, "kubernetes-empty.yaml")
	dst := filepath.Join(dir, "kubelet.yaml")
	dstEmpty := filepath.Join(dir, "kubelet-empty.yaml")

	err := os.WriteFile(src, []byte(kubernetesLegacyConf), 0640)
	require.NoError(t, err)
	err = os.WriteFile(srcEmpty, []byte(kubernetesLegacyEmptyConf), 0640)
	require.NoError(t, err)

	configConverter := NewConfigConverter()
	deprecations, err := importKubernetesConfWithDeprec(src, dst, true, configConverter)
	require.NoError(t, err)
	require.EqualValues(t, expectedKubeDeprecations, deprecations)

	newConf, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, kubeletNewConf, string(newConf))

	assert.Equal(t, 1234, cfg.GetInt("kubernetes_http_kubelet_port"))
	assert.Equal(t, 1234, cfg.GetInt("kubernetes_https_kubelet_port"))
	assert.Equal(t, "localhost", cfg.GetString("kubernetes_kubelet_host"))
	assert.Equal(t, "/path/to/client.crt", cfg.GetString("kubelet_client_crt"))
	assert.Equal(t, "/path/to/client.key", cfg.GetString("kubelet_client_key"))
	assert.Equal(t, "/path/to/ca.pem", cfg.GetString("kubelet_client_ca"))
	assert.Equal(t, "/path/to/token", cfg.GetString("kubelet_auth_token_path"))
	assert.EqualValues(t, expectedHostTags, cfg.GetStringMapString("kubernetes_node_labels_as_tags"))
	assert.Equal(t, false, cfg.GetBool("kubelet_tls_verify"))

	assert.Equal(t, true, cfg.GetBool("kubernetes_collect_service_tags"))
	assert.Equal(t, true, cfg.GetBool("collect_kubernetes_events"))
	assert.Equal(t, true, cfg.GetBool("leader_election"))
	assert.Equal(t, 1200, cfg.GetInt("leader_lease_duration"))
	assert.Equal(t, 3000, cfg.GetInt("kubernetes_service_tag_update_freq"))

	configConverter.Set("kubelet_tls_verify", true)
	deprecations, err = importKubernetesConfWithDeprec(srcEmpty, dstEmpty, true, configConverter)
	require.NoError(t, err)
	assert.Equal(t, true, cfg.GetBool("kubelet_tls_verify"))
	assert.Equal(t, 0, len(deprecations))
	newEmptyConf, err := os.ReadFile(dstEmpty)
	require.NoError(t, err)
	assert.Equal(t, kubeletNewEmptyConf, string(newEmptyConf))

	// test overwrite
	err = ImportKubernetesConf(src, dst, false, configConverter)
	require.NotNil(t, err)
	_, err = os.Stat(filepath.Join(dir, "kubelet.yaml.bak"))
	assert.True(t, os.IsNotExist(err))

	err = ImportKubernetesConf(src, dst, true, configConverter)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "kubelet.yaml.bak"))
	assert.False(t, os.IsNotExist(err))
}
