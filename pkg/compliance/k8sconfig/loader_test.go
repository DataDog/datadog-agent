// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package k8sconfig

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const eksProcTable = `
kubelet \
	--config /etc/kubernetes/kubelet/kubelet-config.json \
	--kubeconfig /var/lib/kubelet/kubeconfig \
	--container-runtime-endpoint unix:///run/containerd/containerd.sock \
	--image-credential-provider-config /etc/eks/image-credential-provider/config.json \
	--image-credential-provider-bin-dir /etc/eks/image-credential-provider \
	--node-ip=192.168.78.181 \
	--pod-infra-container-image=602401143452.dkr.ecr.eu-west-3.amazonaws.com/eks/pause:3.5 \
	--v=2 \
	--cloud-provider=aws \
	--container-runtime=remote \
	--node-labels=eks.amazonaws.com/sourceLaunchTemplateVersion=1,alpha.eksctl.io/cluster-name=Sandbox,alpha.eksctl.io/nodegroup-name=standard,eks.amazonaws.com/nodegroup-image=ami-09f37ddb4a6ecc85e,eks.amazonaws.com/capacityType=ON_DEMAND,eks.amazonaws.com/nodegroup=standard,eks.amazonaws.com/sourceLaunchTemplateId=lt-0df2e04572534b928 \
	--max-pods=17 \
	--rotate-server-certificates=true
`

// TODO(jinroh): use testdata files
var eksFs = []*mockFile{
	{
		name: "/etc/eks/image-credential-provider",
		mode: 0755, isDir: true,
	},
	{
		name: "/etc/eks/image-credential-provider/config.json",
		mode: 0644,
		content: `{
  "apiVersion": "kubelet.config.k8s.io/v1alpha1",
  "kind": "CredentialProviderConfig",
  "providers": [
    {
      "name": "ecr-credential-provider",
      "matchImages": [
        "*.dkr.ecr.*.amazonaws.com",
        "*.dkr.ecr.*.amazonaws.com.cn",
        "*.dkr.ecr-fips.*.amazonaws.com",
        "*.dkr.ecr.us-iso-east-1.c2s.ic.gov",
        "*.dkr.ecr.us-isob-east-1.sc2s.sgov.gov"
      ],
      "defaultCacheDuration": "12h",
      "apiVersion": "credentialprovider.kubelet.k8s.io/v1alpha1"
    }
  ]
}`,
	},
	{
		name: "/etc/eks/release",
		mode: 0664,
		content: `BASE_AMI_ID="ami-0528ac959959021be"
BUILD_TIME="Sat May 13 01:48:34 UTC 2023"
BUILD_KERNEL="5.10.178-162.673.amzn2.aarch64"
ARCH="aarch64"`,
	},
	{
		name: "/etc/kubernetes/kubelet/kubelet-config.json",
		mode: 0644,
		content: `{
  "kind": "KubeletConfiguration",
  "apiVersion": "kubelet.config.k8s.io/v1beta1",
  "address": "0.0.0.0",
  "authentication": {
    "anonymous": {
      "enabled": false
    },
    "webhook": {
      "cacheTTL": "2m0s",
      "enabled": true
    },
    "x509": {
      "clientCAFile": "/etc/kubernetes/pki/ca.crt"
    }
  },
  "authorization": {
    "mode": "Webhook",
    "webhook": {
      "cacheAuthorizedTTL": "5m0s",
      "cacheUnauthorizedTTL": "30s"
    }
  },
  "clusterDomain": "cluster.local",
  "hairpinMode": "hairpin-veth",
  "cgroupDriver": "systemd",
  "cgroupRoot": "/",
  "featureGates": {
    "RotateKubeletServerCertificate": true,
    "KubeletCredentialProviders": true
  },
  "protectKernelDefaults": true,
  "serializeImagePulls": false,
  "serverTLSBootstrap": true,
  "tlsCipherSuites": [
    "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
    "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
    "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305",
    "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
    "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305",
    "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
    "TLS_RSA_WITH_AES_256_GCM_SHA384",
    "TLS_RSA_WITH_AES_128_GCM_SHA256"
  ],
  "clusterDNS": [
    "10.100.0.10"
  ],
  "kubeAPIQPS": 10,
  "kubeAPIBurst": 20,
  "evictionHard": {
    "memory.available": "100Mi",
    "nodefs.available": "10%",
    "nodefs.inodesFree": "5%"
  },
  "kubeReserved": {
    "cpu": "70m",
    "ephemeral-storage": "1Gi",
    "memory": "442Mi"
  },
  "systemReservedCgroup": "/system",
  "kubeReservedCgroup": "/runtime",
  "maxPods": 18
}`,
	},
	{
		name: "/etc/systemd/system/kubelet.service.d/10-kubelet-args.conf",
		mode: 0644,
		content: `[Service]
Environment='KUBELET_ARGS=--node-ip=192.168.78.181 \
	--pod-infra-container-image=602401143452.dkr.ecr.eu-west-3.amazonaws.com/eks/pause:3.5 \
	--v=2 \
	--cloud-provider=aws \
	--container-runtime=remote'`,
	},
	{
		name: "/etc/kubernetes/pki/ca.crt",
		mode: 0644,
		content: `-----BEGIN CERTIFICATE-----
MIIDFTCCAf2gAwIBAgIBATANBgkqhkiG9w0BAQsFADAfMR0wGwYDVQQDDBRsb2Nh
bGhvc3RAMTUxNTQ2MjIwNjAgFw0xODAxMDkwMTQzMjZaGA8yMTE4MDEwOTAxNDMy
NlowHzEdMBsGA1UEAwwUbG9jYWxob3N0QDE1MTU0NjIyMDYwggEiMA0GCSqGSIb3
DQEBAQUAA4IBDwAwggEKAoIBAQC2hIORzonehlNadYyI30v1Jj8lhhABuiWiTSkl
KCLqZjwBfWfSC4w02zxi2SAH9ju20XCJrUauwPq1qXCp/CqXC/rVgZrzluDlpJpe
gF9AilQvGOxhrZhV4kqpOjGVE78uOmpfxiOyNermoJ0OVE8ugh3s/LLTNK/qmCAX
uEYTQccAvNEiPX3XPBCiaFlSCkUNS0zp12mJNP43+KF9y0CbtYs1gXKHmmJVSpjR
YmcuJJUfHxNrV2YR3ek6O4IIJFIlnLxgpjRBseBPkTenAT3S2YY9MyQkkBrRSPBa
vLM24al3KDvXYikYe3WpxeYNHGNcHIgR+hKlRTQ5VrWlfx9dAgMBAAGjWjBYMA4G
A1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggrBgEFBQcDATAPBgNVHRMBAf8EBTAD
AQH/MCAGA1UdEQQZMBeCCWxvY2FsaG9zdIcEfwAAAYcEfwAAATANBgkqhkiG9w0B
AQsFAAOCAQEAFhW8cVTraHPNsE+Jo0ZvcE2ic8lEzeOhWI2O/fpkrUJS5LptPKHS
nTK+CPxA0zhIS/vlJznIabeddXwtq7Xb5SwlJMHYMnHD6f5qwpD22D2dxJJa5sma
3yrK/4CutuEae08qqSeakfgCjcHLL9p7FZWxujkV9/5CEH5lFWYLGumyIoS46Svf
nSfDFKTrOj8P60ncCoWcSpMbdVQBDuKlIZuBMmz9CguC1CtuQWPDUmOGJuPs/+So
yusHbBfj+ATUWDYTg1lLjOIOSJpHGUQkvS+8Bo47SThD/b4w2i6VC72ldxtBuxGf
L7+jALMhMhiQD+Q4qsNuyvvNQLoYcTTFTw==
-----END CERTIFICATE-----
`,
	},
	{
		name: "/var/lib/kubelet/kubeconfig",
		mode: 0644,
		content: `apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority: /etc/kubernetes/pki/ca.crt
    server: https://1DB2F34ED30B77AFEA800D56D3EBED0B.sk1.eu-west-3.eks.amazonaws.com
  name: kubernetes
  contexts:
- context:
    cluster: kubernetes
    user: kubelet
  name: kubelet
  current-context: kubelet
users:
- name: kubelet
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: /usr/bin/aws-iam-authenticator
      args:
        - "token"
        - "-i"
        - "Sandbox"
        - --region
        - "eu-west-3"
`,
	},
}

const kubadmProcTable = `
kube-proxy \
	--config=/var/lib/kube-proxy/config.conf \
	--hostname-override=lima-k8s

kubelet \
	--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf \
	--kubeconfig=/etc/kubernetes/kubelet.conf \
	--config=/var/lib/kubelet/config.yaml \
	--container-runtime-endpoint=unix:///run/containerd/containerd.sock \
	--pod-infra-container-image=registry.k8s.io/pause:3.9

etcd \
	--advertise-client-urls=https://192.168.5.15:2379 \
	--cert-file=/etc/kubernetes/pki/etcd/server.crt \
	--client-cert-auth=true \
	--data-dir=/var/lib/etcd \
	--experimental-initial-corrupt-check=true \
	--experimental-watch-progress-notify-interval=5s \
	--initial-advertise-peer-urls=https://192.168.5.15:2380 \
	--initial-cluster=lima-k8s=https://192.168.5.15:2380 \
	--key-file=/etc/kubernetes/pki/etcd/server.key \
	--listen-client-urls=https://127.0.0.1:2379,https://192.168.5.15:2379 \
	--listen-metrics-urls=http://127.0.0.1:2381 \
	--listen-peer-urls=https://192.168.5.15:2380 \
	--name=lima-k8s \
	--peer-cert-file=/etc/kubernetes/pki/etcd/peer.crt \
	--peer-client-cert-auth=true \
	--peer-key-file=/etc/kubernetes/pki/etcd/peer.key \
	--peer-trusted-ca-file=/etc/kubernetes/pki/etcd/ca.crt \
	--snapshot-count=10000 \
	--trusted-ca-file=/etc/kubernetes/pki/etcd/ca.crt

kube-controller-manager \
	--allocate-node-cidrs=true \
	--authentication-kubeconfig=/etc/kubernetes/controller-manager.conf \
	--authorization-kubeconfig=/etc/kubernetes/controller-manager.conf \
	--bind-address=127.0.0.1 \
	--client-ca-file=/etc/kubernetes/pki/ca.crt \
	--cluster-cidr=10.244.0.0/16 \
	--cluster-name=kubernetes \
	--cluster-signing-cert-file=/etc/kubernetes/pki/ca.crt \
	--cluster-signing-key-file=/etc/kubernetes/pki/ca.key \
	--controllers=*,bootstrapsigner,tokencleaner \
	--kubeconfig=/etc/kubernetes/controller-manager.conf \
	--leader-elect=true \
	--requestheader-client-ca-file=/etc/kubernetes/pki/front-proxy-ca.crt \
	--root-ca-file=/etc/kubernetes/pki/ca.crt \
	--service-account-private-key-file=/etc/kubernetes/pki/sa.key \
	--service-cluster-ip-range=10.96.0.0/12 \
	--use-service-account-credentials=true

kube-scheduler \
	--authentication-kubeconfig=/etc/kubernetes/scheduler.conf \
	--authorization-kubeconfig=/etc/kubernetes/scheduler.conf \
	--bind-address=127.0.0.1 \
	--kubeconfig=/etc/kubernetes/scheduler.conf \
	--leader-elect=true

kube-apiserver \
	--audit-policy-file=/etc/kubernetes/audit-policy.yaml \
	--audit-log-path=/var/log/kubernetes/audit/audit.log \
	--advertise-address=192.168.5.15 \
	--allow-privileged=true \
	--authorization-mode=Node,RBAC \
	--client-ca-file=/etc/kubernetes/pki/ca.crt \
	--enable-admission-plugins=NodeRestriction \
	--enable-bootstrap-token-auth=true \
	--etcd-cafile=/etc/kubernetes/pki/etcd/ca.crt \
	--etcd-certfile=/etc/kubernetes/pki/apiserver-etcd-client.crt \
	--etcd-keyfile=/etc/kubernetes/pki/apiserver-etcd-client.key \
	--etcd-servers=https://127.0.0.1:2379 \
	--kubelet-client-certificate=/etc/kubernetes/pki/apiserver-kubelet-client.crt \
	--kubelet-client-key=/etc/kubernetes/pki/apiserver-kubelet-client.key \
	--kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname \
	--proxy-client-cert-file=/etc/kubernetes/pki/front-proxy-client.crt \
	--proxy-client-key-file=/etc/kubernetes/pki/front-proxy-client.key \
	--requestheader-allowed-names=front-proxy-client \
	--requestheader-client-ca-file=/etc/kubernetes/pki/front-proxy-ca.crt \
	--requestheader-extra-headers-prefix=X-Remote-Extra- \
	--requestheader-group-headers=X-Remote-Group \
	--requestheader-username-headers=X-Remote-User \
	--secure-port=6443 \
	--service-account-issuer=https://kubernetes.default.svc.cluster.local \
	--service-account-key-file=/etc/kubernetes/pki/sa.pub \
	--service-account-signing-key-file=/etc/kubernetes/pki/sa.key \
	--service-cluster-ip-range=10.96.0.0/12 \
	--tls-cert-file=/etc/kubernetes/pki/apiserver.crt \
	--tls-private-key-file=/etc/kubernetes/pki/apiserver.key
`

func procTable(str string) []proc {
	var table []proc
	str = strings.ReplaceAll(str, "\\\n", "")
	for _, l := range strings.Split(str, "\n") {
		if l == "" {
			continue
		}
		cmdline := strings.Fields(l)
		table = append(table, buildProc(cmdline[0], cmdline))
	}
	return table
}

func TestKubAdmConfigLoader(t *testing.T) {
	tmpDir := t.TempDir()
	conf := loadTestConfiguration(tmpDir, kubadmProcTable)
	assert.Empty(t, conf.Errors)
}

func TestKubEksConfigLoader(t *testing.T) {
	tmpDir := t.TempDir()
	for _, f := range eksFs {
		f.create(t, tmpDir)
	}
	conf := loadTestConfiguration(tmpDir, eksProcTable)
	assert.Empty(t, conf.Errors)

	assert.NotNil(t, conf.ManagedEnvironment)
	assert.Equal(t, conf.ManagedEnvironment.Name, "eks")
	assert.True(t, strings.HasPrefix(conf.ManagedEnvironment.Metadata.(string), `BASE_AMI_ID="ami-0528ac959959021be"`))

	assert.Nil(t, conf.Components.KubeApiserver)
	assert.Nil(t, conf.Components.Etcd)
	assert.Nil(t, conf.Components.KubeControllerManager)
	assert.Nil(t, conf.Components.KubeScheduler)
	assert.Nil(t, conf.Components.KubeProxy)

	assert.NotNil(t, conf.Components.Kubelet)
	assert.NotNil(t, conf.Components.Kubelet.Config)
	assert.NotNil(t, conf.Components.Kubelet.Kubeconfig)
	assert.NotNil(t, conf.Components.Kubelet.Config.Content)

	kubeletConfig := conf.Components.Kubelet.Config.Content.(map[string]interface{})

	{
		assert.Nil(t, conf.Components.Kubelet.AnonymousAuth)
		assert.Equal(t, false, kubeletConfig["authentication"].(map[string]interface{})["anonymous"].(map[string]interface{})["enabled"])
	}

	{
		v := 10255
		assert.NotNil(t, conf.Components.Kubelet.ReadOnlyPort)
		assert.Equal(t, &v, conf.Components.Kubelet.ReadOnlyPort)
		assert.Nil(t, kubeletConfig["readOnlyPort"])
	}

	{
		content, ok := conf.Components.Kubelet.Config.Content.(map[string]interface{})
		assert.True(t, ok)
		authentication, ok := content["authentication"].(map[string]interface{})
		assert.True(t, ok)
		x509, ok := authentication["x509"].(map[string]interface{})
		assert.True(t, ok)
		clientCAFile, ok := x509["clientCAFile"].(*K8sCertFileMeta)
		assert.True(t, ok)
		assert.NotNil(t, clientCAFile)
		assert.Nil(t, conf.Components.Kubelet.ClientCaFile)

		assert.Equal(t, true, content["featureGates"].(map[string]interface{})["RotateKubeletServerCertificate"])

		assert.Nil(t, conf.Components.Kubelet.AuthorizationMode)
		assert.Equal(t, "Webhook", content["authorization"].(map[string]interface{})["mode"])

		sevenTeen := 17
		eigthTeen := 18
		assert.Equal(t, &sevenTeen, conf.Components.Kubelet.MaxPods)
		assert.Equal(t, float64(eigthTeen), content["maxPods"])
	}
}

func loadTestConfiguration(hostroot string, table string) *K8sNodeConfig {
	l := &loader{hostroot: hostroot}
	_, data := l.load(context.Background(), func(ctx context.Context) []proc {
		return procTable(table)
	})
	return data
}

type mockFile struct {
	isDir   bool
	name    string
	mode    uint32
	content string
}

func (f *mockFile) create(t *testing.T, root string) {
	if f.isDir {
		if err := os.MkdirAll(filepath.Join(root, f.name), fs.FileMode(f.mode)); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(f.name)), fs.FileMode(0755)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, f.name), []byte(f.content), os.FileMode(f.mode)); err != nil {
			t.Fatal(err)
		}
	}
}
