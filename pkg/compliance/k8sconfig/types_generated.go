// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package k8sconfig TODO
// !!!
// This is a generated file: regenerate with go run ./pkg/compliance/tools/k8s_types_generator.go
// !!!
package k8sconfig

import (
	"strings"
	"time"
)

type K8sKubeApiserverConfig struct {
	AdmissionControlConfigFile      *K8sAdmissionConfigFileMeta          `json:"admission-control-config-file"`      // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	AllowPrivileged                 bool                                 `json:"allow-privileged"`                   // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	AnonymousAuth                   bool                                 `json:"anonymous-auth"`                     // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	AuditLogMaxage                  int                                  `json:"audit-log-maxage"`                   // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	AuditLogMaxbackup               int                                  `json:"audit-log-maxbackup"`                // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	AuditLogMaxsize                 int                                  `json:"audit-log-maxsize"`                  // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	AuditLogPath                    string                               `json:"audit-log-path"`                     // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	AuditPolicyFile                 *K8sConfigFileMeta                   `json:"audit-policy-file"`                  // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	AuthorizationMode               []string                             `json:"authorization-mode"`                 // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	BindAddress                     string                               `json:"bind-address"`                       // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ClientCaFile                    *K8sCertFileMeta                     `json:"client-ca-file"`                     // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	DisableAdmissionPlugins         []string                             `json:"disable-admission-plugins"`          // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	EnableAdmissionPlugins          []string                             `json:"enable-admission-plugins"`           // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	EnableBootstrapTokenAuth        bool                                 `json:"enable-bootstrap-token-auth"`        // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	EncryptionProviderConfig        *K8sEncryptionProviderConfigFileMeta `json:"encryption-provider-config"`         // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	EtcdCafile                      *K8sCertFileMeta                     `json:"etcd-cafile"`                        // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	EtcdCertfile                    *K8sCertFileMeta                     `json:"etcd-certfile"`                      // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	EtcdKeyfile                     *K8sKeyFileMeta                      `json:"etcd-keyfile"`                       // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	FeatureGates                    string                               `json:"feature-gates"`                      // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	KubeletCertificateAuthority     *K8sCertFileMeta                     `json:"kubelet-certificate-authority"`      // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	KubeletClientCertificate        *K8sCertFileMeta                     `json:"kubelet-client-certificate"`         // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	KubeletClientKey                *K8sKeyFileMeta                      `json:"kubelet-client-key"`                 // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	Profiling                       bool                                 `json:"profiling"`                          // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ProxyClientCertFile             *K8sCertFileMeta                     `json:"proxy-client-cert-file"`             // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ProxyClientKeyFile              *K8sKeyFileMeta                      `json:"proxy-client-key-file"`              // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestTimeout                  time.Duration                        `json:"request-timeout"`                    // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestheaderAllowedNames       []string                             `json:"requestheader-allowed-names"`        // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestheaderClientCaFile       *K8sCertFileMeta                     `json:"requestheader-client-ca-file"`       // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestheaderExtraHeadersPrefix []string                             `json:"requestheader-extra-headers-prefix"` // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestheaderGroupHeaders       []string                             `json:"requestheader-group-headers"`        // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestheaderUsernameHeaders    []string                             `json:"requestheader-username-headers"`     // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	SecurePort                      int                                  `json:"secure-port"`                        // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ServiceAccountIssuer            string                               `json:"service-account-issuer"`             // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ServiceAccountKeyFile           *K8sKeyFileMeta                      `json:"service-account-key-file"`           // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ServiceAccountLookup            bool                                 `json:"service-account-lookup"`             // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ServiceAccountSigningKeyFile    *K8sKeyFileMeta                      `json:"service-account-signing-key-file"`   // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ServiceClusterIpRange           string                               `json:"service-cluster-ip-range"`           // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	TlsCertFile                     *K8sCertFileMeta                     `json:"tls-cert-file"`                      // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	TlsCipherSuites                 []string                             `json:"tls-cipher-suites"`                  // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	TlsPrivateKeyFile               *K8sKeyFileMeta                      `json:"tls-private-key-file"`               // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	TokenAuthFile                   *K8sTokenFileMeta                    `json:"token-auth-file"`                    // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	SkippedFlags                    map[string]string                    `json:"skippedFlags,omitempty"`
}

func (l *loader) newK8sKubeApiserverConfig(flags map[string]string) *K8sKubeApiserverConfig {
	if flags == nil {
		return nil
	}
	var res K8sKubeApiserverConfig
	if v, ok := flags["--admission-control-config-file"]; ok {
		delete(flags, "--admission-control-config-file")
		res.AdmissionControlConfigFile = l.loadAdmissionConfigFileMeta(v)
	}
	if v, ok := flags["--allow-privileged"]; ok {
		delete(flags, "--allow-privileged")
		res.AllowPrivileged = l.parseBool(v)
	}
	if v, ok := flags["--anonymous-auth"]; ok {
		delete(flags, "--anonymous-auth")
		res.AnonymousAuth = l.parseBool(v)
	} else {
		res.AnonymousAuth = l.parseBool("true")
	}
	if v, ok := flags["--audit-log-maxage"]; ok {
		delete(flags, "--audit-log-maxage")
		res.AuditLogMaxage = l.parseInt(v)
	} else {
		res.AuditLogMaxage = l.parseInt("0")
	}
	if v, ok := flags["--audit-log-maxbackup"]; ok {
		delete(flags, "--audit-log-maxbackup")
		res.AuditLogMaxbackup = l.parseInt(v)
	} else {
		res.AuditLogMaxbackup = l.parseInt("0")
	}
	if v, ok := flags["--audit-log-maxsize"]; ok {
		delete(flags, "--audit-log-maxsize")
		res.AuditLogMaxsize = l.parseInt(v)
	} else {
		res.AuditLogMaxsize = l.parseInt("0")
	}
	if v, ok := flags["--audit-log-path"]; ok {
		delete(flags, "--audit-log-path")
		res.AuditLogPath = v
	}
	if v, ok := flags["--audit-policy-file"]; ok {
		delete(flags, "--audit-policy-file")
		res.AuditPolicyFile = l.loadConfigFileMeta(v)
	}
	if v, ok := flags["--authorization-mode"]; ok {
		delete(flags, "--authorization-mode")
		res.AuthorizationMode = strings.Split(v, ",")
	} else {
		res.AuthorizationMode = strings.Split("AlwaysAllow", ",")
	}
	if v, ok := flags["--bind-address"]; ok {
		delete(flags, "--bind-address")
		res.BindAddress = v
	} else {
		res.BindAddress = "0.0.0.0"
	}
	if v, ok := flags["--client-ca-file"]; ok {
		delete(flags, "--client-ca-file")
		res.ClientCaFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--disable-admission-plugins"]; ok {
		delete(flags, "--disable-admission-plugins")
		res.DisableAdmissionPlugins = strings.Split(v, ",")
	}
	if v, ok := flags["--enable-admission-plugins"]; ok {
		delete(flags, "--enable-admission-plugins")
		res.EnableAdmissionPlugins = strings.Split(v, ",")
	}
	if v, ok := flags["--enable-bootstrap-token-auth"]; ok {
		delete(flags, "--enable-bootstrap-token-auth")
		res.EnableBootstrapTokenAuth = l.parseBool(v)
	}
	if v, ok := flags["--encryption-provider-config"]; ok {
		delete(flags, "--encryption-provider-config")
		res.EncryptionProviderConfig = l.loadEncryptionProviderConfigFileMeta(v)
	}
	if v, ok := flags["--etcd-cafile"]; ok {
		delete(flags, "--etcd-cafile")
		res.EtcdCafile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--etcd-certfile"]; ok {
		delete(flags, "--etcd-certfile")
		res.EtcdCertfile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--etcd-keyfile"]; ok {
		delete(flags, "--etcd-keyfile")
		res.EtcdKeyfile = l.loadKeyFileMeta(v)
	}
	if v, ok := flags["--feature-gates"]; ok {
		delete(flags, "--feature-gates")
		res.FeatureGates = v
	}
	if v, ok := flags["--kubelet-certificate-authority"]; ok {
		delete(flags, "--kubelet-certificate-authority")
		res.KubeletCertificateAuthority = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--kubelet-client-certificate"]; ok {
		delete(flags, "--kubelet-client-certificate")
		res.KubeletClientCertificate = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--kubelet-client-key"]; ok {
		delete(flags, "--kubelet-client-key")
		res.KubeletClientKey = l.loadKeyFileMeta(v)
	}
	if v, ok := flags["--profiling"]; ok {
		delete(flags, "--profiling")
		res.Profiling = l.parseBool(v)
	} else {
		res.Profiling = l.parseBool("true")
	}
	if v, ok := flags["--proxy-client-cert-file"]; ok {
		delete(flags, "--proxy-client-cert-file")
		res.ProxyClientCertFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--proxy-client-key-file"]; ok {
		delete(flags, "--proxy-client-key-file")
		res.ProxyClientKeyFile = l.loadKeyFileMeta(v)
	}
	if v, ok := flags["--request-timeout"]; ok {
		delete(flags, "--request-timeout")
		res.RequestTimeout = l.parseDuration(v)
	} else {
		res.RequestTimeout = l.parseDuration("1m0s")
	}
	if v, ok := flags["--requestheader-allowed-names"]; ok {
		delete(flags, "--requestheader-allowed-names")
		res.RequestheaderAllowedNames = strings.Split(v, ",")
	}
	if v, ok := flags["--requestheader-client-ca-file"]; ok {
		delete(flags, "--requestheader-client-ca-file")
		res.RequestheaderClientCaFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--requestheader-extra-headers-prefix"]; ok {
		delete(flags, "--requestheader-extra-headers-prefix")
		res.RequestheaderExtraHeadersPrefix = strings.Split(v, ",")
	}
	if v, ok := flags["--requestheader-group-headers"]; ok {
		delete(flags, "--requestheader-group-headers")
		res.RequestheaderGroupHeaders = strings.Split(v, ",")
	}
	if v, ok := flags["--requestheader-username-headers"]; ok {
		delete(flags, "--requestheader-username-headers")
		res.RequestheaderUsernameHeaders = strings.Split(v, ",")
	}
	if v, ok := flags["--secure-port"]; ok {
		delete(flags, "--secure-port")
		res.SecurePort = l.parseInt(v)
	} else {
		res.SecurePort = l.parseInt("6443")
	}
	if v, ok := flags["--service-account-issuer"]; ok {
		delete(flags, "--service-account-issuer")
		res.ServiceAccountIssuer = v
	}
	if v, ok := flags["--service-account-key-file"]; ok {
		delete(flags, "--service-account-key-file")
		res.ServiceAccountKeyFile = l.loadKeyFileMeta(v)
	}
	if v, ok := flags["--service-account-lookup"]; ok {
		delete(flags, "--service-account-lookup")
		res.ServiceAccountLookup = l.parseBool(v)
	} else {
		res.ServiceAccountLookup = l.parseBool("true")
	}
	if v, ok := flags["--service-account-signing-key-file"]; ok {
		delete(flags, "--service-account-signing-key-file")
		res.ServiceAccountSigningKeyFile = l.loadKeyFileMeta(v)
	}
	if v, ok := flags["--service-cluster-ip-range"]; ok {
		delete(flags, "--service-cluster-ip-range")
		res.ServiceClusterIpRange = v
	}
	if v, ok := flags["--tls-cert-file"]; ok {
		delete(flags, "--tls-cert-file")
		res.TlsCertFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--tls-cipher-suites"]; ok {
		delete(flags, "--tls-cipher-suites")
		res.TlsCipherSuites = strings.Split(v, ",")
	}
	if v, ok := flags["--tls-private-key-file"]; ok {
		delete(flags, "--tls-private-key-file")
		res.TlsPrivateKeyFile = l.loadKeyFileMeta(v)
	}
	if v, ok := flags["--token-auth-file"]; ok {
		delete(flags, "--token-auth-file")
		res.TokenAuthFile = l.loadTokenFileMeta(v)
	}
	if len(flags) > 0 {
		res.SkippedFlags = flags
	}
	return &res
}

type K8sKubeSchedulerConfig struct {
	AuthenticationKubeconfig        *K8sKubeconfigMeta `json:"authentication-kubeconfig"`          // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	AuthorizationKubeconfig         string             `json:"authorization-kubeconfig"`           // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	BindAddress                     string             `json:"bind-address"`                       // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ClientCaFile                    *K8sCertFileMeta   `json:"client-ca-file"`                     // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	Config                          *K8sConfigFileMeta `json:"config"`                             // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	FeatureGates                    string             `json:"feature-gates"`                      // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	Kubeconfig                      *K8sKubeconfigMeta `json:"kubeconfig"`                         // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	Profiling                       bool               `json:"profiling"`                          // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestheaderAllowedNames       []string           `json:"requestheader-allowed-names"`        // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestheaderClientCaFile       *K8sCertFileMeta   `json:"requestheader-client-ca-file"`       // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestheaderExtraHeadersPrefix []string           `json:"requestheader-extra-headers-prefix"` // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestheaderGroupHeaders       []string           `json:"requestheader-group-headers"`        // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestheaderUsernameHeaders    []string           `json:"requestheader-username-headers"`     // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	SecurePort                      int                `json:"secure-port"`                        // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	TlsCertFile                     *K8sCertFileMeta   `json:"tls-cert-file"`                      // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	TlsCipherSuites                 []string           `json:"tls-cipher-suites"`                  // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	TlsPrivateKeyFile               *K8sKeyFileMeta    `json:"tls-private-key-file"`               // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	SkippedFlags                    map[string]string  `json:"skippedFlags,omitempty"`
}

func (l *loader) newK8sKubeSchedulerConfig(flags map[string]string) *K8sKubeSchedulerConfig {
	if flags == nil {
		return nil
	}
	var res K8sKubeSchedulerConfig
	if v, ok := flags["--authentication-kubeconfig"]; ok {
		delete(flags, "--authentication-kubeconfig")
		res.AuthenticationKubeconfig = l.loadKubeconfigMeta(v)
	}
	if v, ok := flags["--authorization-kubeconfig"]; ok {
		delete(flags, "--authorization-kubeconfig")
		res.AuthorizationKubeconfig = v
	}
	if v, ok := flags["--bind-address"]; ok {
		delete(flags, "--bind-address")
		res.BindAddress = v
	} else {
		res.BindAddress = "0.0.0.0"
	}
	if v, ok := flags["--client-ca-file"]; ok {
		delete(flags, "--client-ca-file")
		res.ClientCaFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--config"]; ok {
		delete(flags, "--config")
		res.Config = l.loadConfigFileMeta(v)
	}
	if v, ok := flags["--feature-gates"]; ok {
		delete(flags, "--feature-gates")
		res.FeatureGates = v
	}
	if v, ok := flags["--kubeconfig"]; ok {
		delete(flags, "--kubeconfig")
		res.Kubeconfig = l.loadKubeconfigMeta(v)
	}
	if v, ok := flags["--profiling"]; ok {
		delete(flags, "--profiling")
		res.Profiling = l.parseBool(v)
	} else {
		res.Profiling = l.parseBool("true")
	}
	if v, ok := flags["--requestheader-allowed-names"]; ok {
		delete(flags, "--requestheader-allowed-names")
		res.RequestheaderAllowedNames = strings.Split(v, ",")
	}
	if v, ok := flags["--requestheader-client-ca-file"]; ok {
		delete(flags, "--requestheader-client-ca-file")
		res.RequestheaderClientCaFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--requestheader-extra-headers-prefix"]; ok {
		delete(flags, "--requestheader-extra-headers-prefix")
		res.RequestheaderExtraHeadersPrefix = strings.Split(v, ",")
	} else {
		res.RequestheaderExtraHeadersPrefix = strings.Split("x-remote-extra-", ",")
	}
	if v, ok := flags["--requestheader-group-headers"]; ok {
		delete(flags, "--requestheader-group-headers")
		res.RequestheaderGroupHeaders = strings.Split(v, ",")
	} else {
		res.RequestheaderGroupHeaders = strings.Split("x-remote-group", ",")
	}
	if v, ok := flags["--requestheader-username-headers"]; ok {
		delete(flags, "--requestheader-username-headers")
		res.RequestheaderUsernameHeaders = strings.Split(v, ",")
	} else {
		res.RequestheaderUsernameHeaders = strings.Split("x-remote-user", ",")
	}
	if v, ok := flags["--secure-port"]; ok {
		delete(flags, "--secure-port")
		res.SecurePort = l.parseInt(v)
	} else {
		res.SecurePort = l.parseInt("10259")
	}
	if v, ok := flags["--tls-cert-file"]; ok {
		delete(flags, "--tls-cert-file")
		res.TlsCertFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--tls-cipher-suites"]; ok {
		delete(flags, "--tls-cipher-suites")
		res.TlsCipherSuites = strings.Split(v, ",")
	}
	if v, ok := flags["--tls-private-key-file"]; ok {
		delete(flags, "--tls-private-key-file")
		res.TlsPrivateKeyFile = l.loadKeyFileMeta(v)
	}
	if len(flags) > 0 {
		res.SkippedFlags = flags
	}
	return &res
}

type K8sKubeControllerManagerConfig struct {
	AuthenticationKubeconfig        *K8sKubeconfigMeta `json:"authentication-kubeconfig"`          // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	AuthorizationKubeconfig         string             `json:"authorization-kubeconfig"`           // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	BindAddress                     string             `json:"bind-address"`                       // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ClientCaFile                    *K8sCertFileMeta   `json:"client-ca-file"`                     // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ClusterSigningCertFile          *K8sCertFileMeta   `json:"cluster-signing-cert-file"`          // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ClusterSigningKeyFile           *K8sKeyFileMeta    `json:"cluster-signing-key-file"`           // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	FeatureGates                    string             `json:"feature-gates"`                      // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	Kubeconfig                      *K8sKubeconfigMeta `json:"kubeconfig"`                         // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	Profiling                       bool               `json:"profiling"`                          // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestheaderAllowedNames       []string           `json:"requestheader-allowed-names"`        // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestheaderClientCaFile       *K8sCertFileMeta   `json:"requestheader-client-ca-file"`       // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestheaderExtraHeadersPrefix []string           `json:"requestheader-extra-headers-prefix"` // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestheaderGroupHeaders       []string           `json:"requestheader-group-headers"`        // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RequestheaderUsernameHeaders    []string           `json:"requestheader-username-headers"`     // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RootCaFile                      *K8sCertFileMeta   `json:"root-ca-file"`                       // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	SecurePort                      int                `json:"secure-port"`                        // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ServiceAccountPrivateKeyFile    *K8sKeyFileMeta    `json:"service-account-private-key-file"`   // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ServiceClusterIpRange           string             `json:"service-cluster-ip-range"`           // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	TerminatedPodGcThreshold        int                `json:"terminated-pod-gc-threshold"`        // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	TlsCertFile                     *K8sCertFileMeta   `json:"tls-cert-file"`                      // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	TlsCipherSuites                 []string           `json:"tls-cipher-suites"`                  // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	TlsPrivateKeyFile               *K8sKeyFileMeta    `json:"tls-private-key-file"`               // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	UseServiceAccountCredentials    bool               `json:"use-service-account-credentials"`    // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	SkippedFlags                    map[string]string  `json:"skippedFlags,omitempty"`
}

func (l *loader) newK8sKubeControllerManagerConfig(flags map[string]string) *K8sKubeControllerManagerConfig {
	if flags == nil {
		return nil
	}
	var res K8sKubeControllerManagerConfig
	if v, ok := flags["--authentication-kubeconfig"]; ok {
		delete(flags, "--authentication-kubeconfig")
		res.AuthenticationKubeconfig = l.loadKubeconfigMeta(v)
	}
	if v, ok := flags["--authorization-kubeconfig"]; ok {
		delete(flags, "--authorization-kubeconfig")
		res.AuthorizationKubeconfig = v
	}
	if v, ok := flags["--bind-address"]; ok {
		delete(flags, "--bind-address")
		res.BindAddress = v
	} else {
		res.BindAddress = "0.0.0.0"
	}
	if v, ok := flags["--client-ca-file"]; ok {
		delete(flags, "--client-ca-file")
		res.ClientCaFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--cluster-signing-cert-file"]; ok {
		delete(flags, "--cluster-signing-cert-file")
		res.ClusterSigningCertFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--cluster-signing-key-file"]; ok {
		delete(flags, "--cluster-signing-key-file")
		res.ClusterSigningKeyFile = l.loadKeyFileMeta(v)
	}
	if v, ok := flags["--feature-gates"]; ok {
		delete(flags, "--feature-gates")
		res.FeatureGates = v
	}
	if v, ok := flags["--kubeconfig"]; ok {
		delete(flags, "--kubeconfig")
		res.Kubeconfig = l.loadKubeconfigMeta(v)
	}
	if v, ok := flags["--profiling"]; ok {
		delete(flags, "--profiling")
		res.Profiling = l.parseBool(v)
	} else {
		res.Profiling = l.parseBool("true")
	}
	if v, ok := flags["--requestheader-allowed-names"]; ok {
		delete(flags, "--requestheader-allowed-names")
		res.RequestheaderAllowedNames = strings.Split(v, ",")
	}
	if v, ok := flags["--requestheader-client-ca-file"]; ok {
		delete(flags, "--requestheader-client-ca-file")
		res.RequestheaderClientCaFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--requestheader-extra-headers-prefix"]; ok {
		delete(flags, "--requestheader-extra-headers-prefix")
		res.RequestheaderExtraHeadersPrefix = strings.Split(v, ",")
	} else {
		res.RequestheaderExtraHeadersPrefix = strings.Split("x-remote-extra-", ",")
	}
	if v, ok := flags["--requestheader-group-headers"]; ok {
		delete(flags, "--requestheader-group-headers")
		res.RequestheaderGroupHeaders = strings.Split(v, ",")
	} else {
		res.RequestheaderGroupHeaders = strings.Split("x-remote-group", ",")
	}
	if v, ok := flags["--requestheader-username-headers"]; ok {
		delete(flags, "--requestheader-username-headers")
		res.RequestheaderUsernameHeaders = strings.Split(v, ",")
	} else {
		res.RequestheaderUsernameHeaders = strings.Split("x-remote-user", ",")
	}
	if v, ok := flags["--root-ca-file"]; ok {
		delete(flags, "--root-ca-file")
		res.RootCaFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--secure-port"]; ok {
		delete(flags, "--secure-port")
		res.SecurePort = l.parseInt(v)
	} else {
		res.SecurePort = l.parseInt("10257")
	}
	if v, ok := flags["--service-account-private-key-file"]; ok {
		delete(flags, "--service-account-private-key-file")
		res.ServiceAccountPrivateKeyFile = l.loadKeyFileMeta(v)
	}
	if v, ok := flags["--service-cluster-ip-range"]; ok {
		delete(flags, "--service-cluster-ip-range")
		res.ServiceClusterIpRange = v
	}
	if v, ok := flags["--terminated-pod-gc-threshold"]; ok {
		delete(flags, "--terminated-pod-gc-threshold")
		res.TerminatedPodGcThreshold = l.parseInt(v)
	} else {
		res.TerminatedPodGcThreshold = l.parseInt("12500")
	}
	if v, ok := flags["--tls-cert-file"]; ok {
		delete(flags, "--tls-cert-file")
		res.TlsCertFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--tls-cipher-suites"]; ok {
		delete(flags, "--tls-cipher-suites")
		res.TlsCipherSuites = strings.Split(v, ",")
	}
	if v, ok := flags["--tls-private-key-file"]; ok {
		delete(flags, "--tls-private-key-file")
		res.TlsPrivateKeyFile = l.loadKeyFileMeta(v)
	}
	if v, ok := flags["--use-service-account-credentials"]; ok {
		delete(flags, "--use-service-account-credentials")
		res.UseServiceAccountCredentials = l.parseBool(v)
	}
	if len(flags) > 0 {
		res.SkippedFlags = flags
	}
	return &res
}

type K8sKubeProxyConfig struct {
	BindAddress      string             `json:"bind-address"`      // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	Config           *K8sConfigFileMeta `json:"config"`            // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	FeatureGates     string             `json:"feature-gates"`     // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	HostnameOverride string             `json:"hostname-override"` // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	Kubeconfig       *K8sKubeconfigMeta `json:"kubeconfig"`        // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	Profiling        bool               `json:"profiling"`         // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	SkippedFlags     map[string]string  `json:"skippedFlags,omitempty"`
}

func (l *loader) newK8sKubeProxyConfig(flags map[string]string) *K8sKubeProxyConfig {
	if flags == nil {
		return nil
	}
	var res K8sKubeProxyConfig
	if v, ok := flags["--bind-address"]; ok {
		delete(flags, "--bind-address")
		res.BindAddress = v
	} else {
		res.BindAddress = "0.0.0.0"
	}
	if v, ok := flags["--config"]; ok {
		delete(flags, "--config")
		res.Config = l.loadConfigFileMeta(v)
	}
	if v, ok := flags["--feature-gates"]; ok {
		delete(flags, "--feature-gates")
		res.FeatureGates = v
	}
	if v, ok := flags["--hostname-override"]; ok {
		delete(flags, "--hostname-override")
		res.HostnameOverride = v
	}
	if v, ok := flags["--kubeconfig"]; ok {
		delete(flags, "--kubeconfig")
		res.Kubeconfig = l.loadKubeconfigMeta(v)
	}
	if v, ok := flags["--profiling"]; ok {
		delete(flags, "--profiling")
		res.Profiling = l.parseBool(v)
	}
	if len(flags) > 0 {
		res.SkippedFlags = flags
	}
	return &res
}

type K8sKubeletConfig struct {
	Address                        string             `json:"address"`                           // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	AnonymousAuth                  bool               `json:"anonymous-auth"`                    // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	AuthorizationMode              string             `json:"authorization-mode"`                // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ClientCaFile                   *K8sCertFileMeta   `json:"client-ca-file"`                    // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	Config                         *K8sConfigFileMeta `json:"config"`                            // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	EventBurst                     int                `json:"event-burst"`                       // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	EventQps                       int                `json:"event-qps"`                         // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	FeatureGates                   string             `json:"feature-gates"`                     // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	HostnameOverride               string             `json:"hostname-override"`                 // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ImageCredentialProviderBinDir  *K8sDirMeta        `json:"image-credential-provider-bin-dir"` // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ImageCredentialProviderConfig  *K8sConfigFileMeta `json:"image-credential-provider-config"`  // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	Kubeconfig                     *K8sKubeconfigMeta `json:"kubeconfig"`                        // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	MakeIptablesUtilChains         bool               `json:"make-iptables-util-chains"`         // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	MaxPods                        int                `json:"max-pods"`                          // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	PodMaxPids                     int                `json:"pod-max-pids"`                      // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ProtectKernelDefaults          bool               `json:"protect-kernel-defaults"`           // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	ReadOnlyPort                   int                `json:"read-only-port"`                    // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RotateCertificates             bool               `json:"rotate-certificates"`               // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	RotateServerCertificates       bool               `json:"rotate-server-certificates"`        // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	StreamingConnectionIdleTimeout time.Duration      `json:"streaming-connection-idle-timeout"` // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	TlsCertFile                    *K8sCertFileMeta   `json:"tls-cert-file"`                     // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	TlsCipherSuites                []string           `json:"tls-cipher-suites"`                 // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	TlsPrivateKeyFile              *K8sKeyFileMeta    `json:"tls-private-key-file"`              // versions: v1.27.3, v1.26.6, v1.25.11, v1.24.15, v1.23.17
	SkippedFlags                   map[string]string  `json:"skippedFlags,omitempty"`
}

func (l *loader) newK8sKubeletConfig(flags map[string]string) *K8sKubeletConfig {
	if flags == nil {
		return nil
	}
	var res K8sKubeletConfig
	if v, ok := flags["--address"]; ok {
		delete(flags, "--address")
		res.Address = v
	} else {
		res.Address = "0.0.0.0"
	}
	if v, ok := flags["--anonymous-auth"]; ok {
		delete(flags, "--anonymous-auth")
		res.AnonymousAuth = l.parseBool(v)
	} else {
		res.AnonymousAuth = l.parseBool("true")
	}
	if v, ok := flags["--authorization-mode"]; ok {
		delete(flags, "--authorization-mode")
		res.AuthorizationMode = v
	} else {
		res.AuthorizationMode = "AlwaysAllow"
	}
	if v, ok := flags["--client-ca-file"]; ok {
		delete(flags, "--client-ca-file")
		res.ClientCaFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--config"]; ok {
		delete(flags, "--config")
		res.Config = l.loadKubeletConfigFileMeta(v)
	}
	if v, ok := flags["--event-burst"]; ok {
		delete(flags, "--event-burst")
		res.EventBurst = l.parseInt(v)
	} else {
		res.EventBurst = l.parseInt("100")
	}
	if v, ok := flags["--event-qps"]; ok {
		delete(flags, "--event-qps")
		res.EventQps = l.parseInt(v)
	} else {
		res.EventQps = l.parseInt("50")
	}
	if v, ok := flags["--feature-gates"]; ok {
		delete(flags, "--feature-gates")
		res.FeatureGates = v
	}
	if v, ok := flags["--hostname-override"]; ok {
		delete(flags, "--hostname-override")
		res.HostnameOverride = v
	}
	if v, ok := flags["--image-credential-provider-bin-dir"]; ok {
		delete(flags, "--image-credential-provider-bin-dir")
		res.ImageCredentialProviderBinDir = l.loadDirMeta(v)
	}
	if v, ok := flags["--image-credential-provider-config"]; ok {
		delete(flags, "--image-credential-provider-config")
		res.ImageCredentialProviderConfig = l.loadConfigFileMeta(v)
	}
	if v, ok := flags["--kubeconfig"]; ok {
		delete(flags, "--kubeconfig")
		res.Kubeconfig = l.loadKubeconfigMeta(v)
	}
	if v, ok := flags["--make-iptables-util-chains"]; ok {
		delete(flags, "--make-iptables-util-chains")
		res.MakeIptablesUtilChains = l.parseBool(v)
	} else {
		res.MakeIptablesUtilChains = l.parseBool("true")
	}
	if v, ok := flags["--max-pods"]; ok {
		delete(flags, "--max-pods")
		res.MaxPods = l.parseInt(v)
	} else {
		res.MaxPods = l.parseInt("110")
	}
	if v, ok := flags["--pod-max-pids"]; ok {
		delete(flags, "--pod-max-pids")
		res.PodMaxPids = l.parseInt(v)
	} else {
		res.PodMaxPids = l.parseInt("-1")
	}
	if v, ok := flags["--protect-kernel-defaults"]; ok {
		delete(flags, "--protect-kernel-defaults")
		res.ProtectKernelDefaults = l.parseBool(v)
	}
	if v, ok := flags["--read-only-port"]; ok {
		delete(flags, "--read-only-port")
		res.ReadOnlyPort = l.parseInt(v)
	} else {
		res.ReadOnlyPort = l.parseInt("10255")
	}
	if v, ok := flags["--rotate-certificates"]; ok {
		delete(flags, "--rotate-certificates")
		res.RotateCertificates = l.parseBool(v)
	}
	if v, ok := flags["--rotate-server-certificates"]; ok {
		delete(flags, "--rotate-server-certificates")
		res.RotateServerCertificates = l.parseBool(v)
	}
	if v, ok := flags["--streaming-connection-idle-timeout"]; ok {
		delete(flags, "--streaming-connection-idle-timeout")
		res.StreamingConnectionIdleTimeout = l.parseDuration(v)
	} else {
		res.StreamingConnectionIdleTimeout = l.parseDuration("4h0m0s")
	}
	if v, ok := flags["--tls-cert-file"]; ok {
		delete(flags, "--tls-cert-file")
		res.TlsCertFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--tls-cipher-suites"]; ok {
		delete(flags, "--tls-cipher-suites")
		res.TlsCipherSuites = strings.Split(v, ",")
	}
	if v, ok := flags["--tls-private-key-file"]; ok {
		delete(flags, "--tls-private-key-file")
		res.TlsPrivateKeyFile = l.loadKeyFileMeta(v)
	}
	if len(flags) > 0 {
		res.SkippedFlags = flags
	}
	return &res
}

type K8sEtcdConfig struct {
	AutoTls            bool              `json:"auto-tls"`              // versions: v3.5.7, v3.4.18, v3.3.17, v3.2.32
	CertFile           *K8sCertFileMeta  `json:"cert-file"`             // versions: v3.5.7, v3.4.18, v3.3.17, v3.2.32
	ClientCertAuth     bool              `json:"client-cert-auth"`      // versions: v3.5.7, v3.4.18, v3.3.17, v3.2.32
	DataDir            *K8sDirMeta       `json:"data-dir"`              // versions: v3.5.7, v3.4.18, v3.3.17, v3.2.32
	KeyFile            *K8sKeyFileMeta   `json:"key-file"`              // versions: v3.5.7, v3.4.18, v3.3.17, v3.2.32
	PeerAutoTls        bool              `json:"peer-auto-tls"`         // versions: v3.5.7, v3.4.18, v3.3.17, v3.2.32
	PeerCertFile       *K8sCertFileMeta  `json:"peer-cert-file"`        // versions: v3.5.7, v3.4.18, v3.3.17, v3.2.32
	PeerClientCertAuth bool              `json:"peer-client-cert-auth"` // versions: v3.5.7, v3.4.18, v3.3.17, v3.2.32
	PeerKeyFile        *K8sKeyFileMeta   `json:"peer-key-file"`         // versions: v3.5.7, v3.4.18, v3.3.17, v3.2.32
	PeerTrustedCaFile  *K8sCertFileMeta  `json:"peer-trusted-ca-file"`  // versions: v3.5.7, v3.4.18, v3.3.17, v3.2.32
	TrustedCaFile      *K8sCertFileMeta  `json:"trusted-ca-file"`       // versions: v3.5.7, v3.4.18, v3.3.17, v3.2.32
	SkippedFlags       map[string]string `json:"skippedFlags,omitempty"`
}

func (l *loader) newK8sEtcdConfig(flags map[string]string) *K8sEtcdConfig {
	if flags == nil {
		return nil
	}
	var res K8sEtcdConfig
	if v, ok := flags["--auto-tls"]; ok {
		delete(flags, "--auto-tls")
		res.AutoTls = l.parseBool(v)
	}
	if v, ok := flags["--cert-file"]; ok {
		delete(flags, "--cert-file")
		res.CertFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--client-cert-auth"]; ok {
		delete(flags, "--client-cert-auth")
		res.ClientCertAuth = l.parseBool(v)
	}
	if v, ok := flags["--data-dir"]; ok {
		delete(flags, "--data-dir")
		res.DataDir = l.loadDirMeta(v)
	}
	if v, ok := flags["--key-file"]; ok {
		delete(flags, "--key-file")
		res.KeyFile = l.loadKeyFileMeta(v)
	}
	if v, ok := flags["--peer-auto-tls"]; ok {
		delete(flags, "--peer-auto-tls")
		res.PeerAutoTls = l.parseBool(v)
	}
	if v, ok := flags["--peer-cert-file"]; ok {
		delete(flags, "--peer-cert-file")
		res.PeerCertFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--peer-client-cert-auth"]; ok {
		delete(flags, "--peer-client-cert-auth")
		res.PeerClientCertAuth = l.parseBool(v)
	}
	if v, ok := flags["--peer-key-file"]; ok {
		delete(flags, "--peer-key-file")
		res.PeerKeyFile = l.loadKeyFileMeta(v)
	}
	if v, ok := flags["--peer-trusted-ca-file"]; ok {
		delete(flags, "--peer-trusted-ca-file")
		res.PeerTrustedCaFile = l.loadCertFileMeta(v)
	}
	if v, ok := flags["--trusted-ca-file"]; ok {
		delete(flags, "--trusted-ca-file")
		res.TrustedCaFile = l.loadCertFileMeta(v)
	}
	if len(flags) > 0 {
		res.SkippedFlags = flags
	}
	return &res
}
