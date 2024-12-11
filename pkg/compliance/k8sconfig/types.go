// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8sconfig

import (
	"net"
	"time"
)

//revive:disable

type K8sNodeConfig struct {
	Version            string               `json:"version"`
	ManagedEnvironment *K8sManagedEnvConfig `json:"managedEnvironment,omitempty"`
	KubeletService     *K8sConfigFileMeta   `json:"kubeletService,omitempty"`
	AdminKubeconfig    *K8sKubeconfigMeta   `json:"adminKubeconfig,omitempty"`
	Components         struct {
		Etcd                  *K8sEtcdConfig                  `json:"etcd,omitempty"`
		KubeApiserver         *K8sKubeApiserverConfig         `json:"kubeApiserver,omitempty"`
		KubeControllerManager *K8sKubeControllerManagerConfig `json:"kubeControllerManager,omitempty"`
		Kubelet               *K8sKubeletConfig               `json:"kubelet,omitempty"`
		KubeProxy             *K8sKubeProxyConfig             `json:"kubeProxy,omitempty"`
		KubeScheduler         *K8sKubeSchedulerConfig         `json:"kubeScheduler,omitempty"`
	} `json:"components"`
	Manifests struct {
		Etcd                 *K8sConfigFileMeta `json:"etcd,omitempty"`
		KubeContollerManager *K8sConfigFileMeta `json:"kubeControllerManager,omitempty"`
		KubeApiserver        *K8sConfigFileMeta `json:"kubeApiserver,omitempty"`
		KubeScheduler        *K8sConfigFileMeta `json:"kubeScheduler,omitempty"`
	} `json:"manifests"`
	Errors []string `json:"errors,omitempty"`
}

type K8sManagedEnvConfig struct {
	Name     string      `json:"name"`
	Metadata interface{} `json:"metadata"`
}

type K8sDirMeta struct {
	Path  string `json:"path"`
	User  string `json:"user,omitempty"`
	Group string `json:"group,omitempty"`
	Mode  uint32 `json:"mode,omitempty"`
}

type K8sConfigFileMeta struct {
	Path    string      `json:"path"`
	User    string      `json:"user,omitempty"`
	Group   string      `json:"group,omitempty"`
	Mode    uint32      `json:"mode,omitempty"`
	Content interface{} `json:"content,omitempty" jsonschema:"type=object"`
}

type K8sTokenFileMeta struct {
	Path  string `json:"path"`
	User  string `json:"user,omitempty"`
	Group string `json:"group,omitempty"`
	Mode  uint32 `json:"mode,omitempty"`
}

// https://github.com/kubernetes/kubernetes/blob/6356023cb42d681b7ad0e6d14d1652247d75b797/staging/src/k8s.io/apiserver/pkg/apis/apiserver/types.go#L30
type (
	k8sAdmissionConfigSource struct {
		Plugins []struct {
			Name          string      `yaml:"name"`
			Path          string      `yaml:"path"`
			Configuration interface{} `yaml:"configuration"`
		} `yaml:"plugins"`
	}

	K8sAdmissionPluginConfigMeta struct {
		Name          string      `json:"name"`
		Configuration interface{} `json:"configuration,omitempty"`
	}

	K8sAdmissionConfigFileMeta struct {
		Path    string                          `json:"path"`
		User    string                          `json:"user,omitempty"`
		Group   string                          `json:"group,omitempty"`
		Mode    uint32                          `json:"mode,omitempty"`
		Plugins []*K8sAdmissionPluginConfigMeta `json:"plugins"`
	}
)

type K8sKubeconfigMeta struct {
	Path       string         `json:"path"`
	User       string         `json:"user,omitempty"`
	Group      string         `json:"group,omitempty"`
	Mode       uint32         `json:"mode,omitempty"`
	Kubeconfig *K8SKubeconfig `json:"kubeconfig,omitempty"`
}

type K8sKeyFileMeta struct {
	Path  string `json:"path"`
	User  string `json:"user,omitempty"`
	Group string `json:"group,omitempty"`
	Mode  uint32 `json:"mode,omitempty"`
}

type K8sCertFileMeta struct {
	Path        string `json:"path"`
	User        string `json:"user,omitempty"`
	Group       string `json:"group,omitempty"`
	Mode        uint32 `json:"mode,omitempty"`
	DirUser     string `json:"dirUser,omitempty"`
	DirGroup    string `json:"dirGroup,omitempty"`
	DirMode     uint32 `json:"dirMode,omitempty"`
	Certificate struct {
		Fingerprint    string     `json:"fingerprint,omitempty"`
		SerialNumber   string     `json:"serialNumber,omitempty"`
		SubjectKeyId   string     `json:"subjectKeyId,omitempty"`
		AuthorityKeyId string     `json:"authorityKeyId,omitempty"`
		CommonName     string     `json:"commonName,omitempty"`
		Organization   []string   `json:"organization,omitempty"`
		DNSNames       []string   `json:"dnsNames,omitempty"`
		IPAddresses    []net.IP   `json:"ipAddresses,omitempty"`
		NotAfter       *time.Time `json:"notAfter,omitempty"`
		NotBefore      *time.Time `json:"notBefore,omitempty"`
	} `json:"certificate"`
}

// k8SKubeconfigSource is used to parse the kubeconfig files. It is not
// exported as-is, and used to build K8sKubeconfig.
// https://github.com/kubernetes/kubernetes/blob/ad18954259eae3db51bac2274ed4ca7304b923c4/staging/src/k8s.io/client-go/tools/clientcmd/api/types.go#LL31C1-L55C2
type (
	k8SKubeconfigSource struct {
		Kind       string `yaml:"kind,omitempty"`
		APIVersion string `yaml:"apiVersion,omitempty"`

		Clusters []struct {
			Name    string                     `yaml:"name"`
			Cluster k8sKubeconfigClusterSource `yaml:"cluster"`
		} `yaml:"clusters"`

		Users []struct {
			Name string                  `yaml:"name"`
			User k8sKubeconfigUserSource `yaml:"user"`
		} `yaml:"users"`

		Contexts []struct {
			Name    string                     `yaml:"name"`
			Context k8sKubeconfigContextSource `yaml:"context"`
		} `yaml:"contexts"`

		CurrentContext string `yaml:"current-context"`
	}

	k8sKubeconfigClusterSource struct {
		Server                   string `yaml:"server"`
		TLSServerName            string `yaml:"tls-server-name,omitempty"`
		InsecureSkipTLSVerify    bool   `yaml:"insecure-skip-tls-verify,omitempty"`
		CertificateAuthority     string `yaml:"certificate-authority,omitempty"`
		CertificateAuthorityData string `yaml:"certificate-authority-data,omitempty"`
		ProxyURL                 string `yaml:"proxy-url,omitempty"`
		DisableCompression       bool   `yaml:"disable-compression,omitempty"`
	}

	k8sKubeconfigUserSource struct {
		ClientCertificate     string `yaml:"client-certificate,omitempty"`
		ClientCertificateData string `yaml:"client-certificate-data,omitempty"`
		ClientKey             string `yaml:"client-key,omitempty"`
		Token                 string `yaml:"token,omitempty"`
		TokenFile             string `yaml:"tokenFile,omitempty"`
		Username              string `yaml:"username,omitempty"`
		Password              string `yaml:"password,omitempty"`
		Exec                  *struct {
			APIVersion string   `yaml:"apiVersion,omitempty"`
			Command    string   `yaml:"command,omitempty"`
			Args       []string `yaml:"args,omitempty"`
		} `yaml:"exec,omitempty"`
	}

	k8sKubeconfigContextSource struct {
		Cluster   string `yaml:"cluster"`
		User      string `yaml:"user"`
		Namespace string `yaml:"namespace,omitempty"`
	}

	K8SKubeconfig struct {
		Clusters       map[string]*K8sKubeconfigCluster `json:"clusters"`
		Users          map[string]*K8sKubeconfigUser    `json:"users"`
		Contexts       map[string]*K8sKubeconfigContext `json:"contexts"`
		CurrentContext string                           `json:"currentContext"`
	}

	K8sKubeconfigCluster struct {
		Server                string           `json:"server"`
		TLSServerName         string           `json:"tlsServerName,omitempty"`
		InsecureSkipTLSVerify bool             `json:"insecureSkipTlsVerify,omitempty"`
		CertificateAuthority  *K8sCertFileMeta `json:"certificateAuthority,omitempty"`
		ProxyURL              string           `json:"proxyUrl,omitempty"`
		DisableCompression    bool             `json:"disableCompression,omitempty"`
	}

	K8sKubeconfigUser struct {
		UseToken    bool `json:"useToken"`
		UsePassword bool `json:"usePassword"`
		Exec        struct {
			APIVersion string   `json:"apiVersion,omitempty"`
			Command    string   `json:"command,omitempty"`
			Args       []string `json:"args,omitempty"`
		} `json:"exec,omitempty"`
		ClientCertificate *K8sCertFileMeta `json:"clientCertificate,omitempty"`
		ClientKey         *K8sKeyFileMeta  `json:"clientKey,omitempty"`
	}

	K8sKubeconfigContext struct {
		Cluster   string `json:"cluster"`
		User      string `json:"user"`
		Namespace string `json:"namespace,omitempty"`
	}
)

// https://github.com/kubernetes/kubernetes/blob/e1ad9bee5bba8fbe85a6bf6201379ce8b1a611b1/staging/src/k8s.io/apiserver/pkg/apis/config/types.go#L70
type (
	K8sEncryptionProviderConfigFileMeta struct {
		Path      string `json:"path,omitempty"`
		User      string `json:"user,omitempty"`
		Group     string `json:"group,omitempty"`
		Mode      uint32 `json:"mode,omitempty"`
		Resources []struct {
			Resources []string `yaml:"resources" json:"resources"`
			Providers []struct {
				AESGCM    *K8sEncryptionProviderKeysSource `yaml:"aesgcm,omitempty" json:"aesgcm,omitempty"`
				AESCBC    *K8sEncryptionProviderKeysSource `yaml:"aescbc,omitempty" json:"aescbc,omitempty"`
				Secretbox *K8sEncryptionProviderKeysSource `yaml:"secretbox,omitempty" json:"secretbox,omitempty"`
				Identity  *struct{}                        `yaml:"identity,omitempty" json:"identity,omitempty"`
				KMS       *K8sEncryptionProviderKMSSource  `yaml:"kms,omitempty" json:"kms,omitempty"`
			} `yaml:"providers" json:"providers"`
		} `yaml:"resources" json:"resources"`
	}

	K8sEncryptionProviderKMSSource struct {
		Name      string `yaml:"name" json:"name"`
		Endpoint  string `yaml:"endpoint" json:"endpoint"`
		CacheSize int    `yaml:"cachesize" json:"cachesize"`
		Timeout   string `yaml:"timeout" json:"timeout"`
	}

	K8sEncryptionProviderKeysSource struct {
		Keys []struct {
			Name string `yaml:"name" json:"name"`
		} `yaml:"keys" json:"keys"`
	}
)
