// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package k8sconfig is a compliance submodule that is able to parse the
// Kubernetes components configurations and export it as a log.
package k8sconfig

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance/utils"
	"github.com/shirou/gopsutil/v3/process"
	"gopkg.in/yaml.v3"
)

const version = "202403"

const (
	k8sManifestsDir   = "/etc/kubernetes/manifests"
	k8sKubeconfigsDir = "/etc/kubernetes"
)

type procsLoader func(ctx context.Context) []proc
type proc struct {
	name  string
	flags map[string]string
}

type loader struct {
	hostroot string
	errs     []error
}

// LoadConfiguration extracts a complete summary of all current Kubernetes
// node configuration. It does so by first looking at the running processes,
// looking up for Kubernetes related processes. For each component's process
// that were find, it collects the command line flags and associated files.
// The knowledge of each components specificities is based on the
// k8s_types_generator.go utility that encodes every relevant flags
// specificities (see types_generated.go).
func LoadConfiguration(ctx context.Context, hostroot string) (string, *K8sNodeConfig) {
	l := &loader{hostroot: hostroot}
	return l.load(ctx, l.loadProcesses)
}

// NOTE(jinroh): the reason we rely on the loadProcesses argument is to simplify
// our testing to mock the process table. see loader_test.go
func (l *loader) load(ctx context.Context, loadProcesses procsLoader) (string, *K8sNodeConfig) {
	node := K8sNodeConfig{Version: version}

	node.KubeletService = l.loadServiceFileMeta([]string{
		"/etc/systemd/system/kubelet.service.d/kubelet.conf",
		"/etc/systemd/system/kubelet.service.d/10-kubeadm.conf",
		"/etc/systemd/system/kubelet.service.d/10-kubelet-args.conf",
		"/etc/systemd/system/kubelet.service",
		"/usr/lib/systemd/system/kubelet.service",
		"/lib/systemd/system/kubelet.service",
	})

	if c, ok := l.loadKubeconfigMeta(filepath.Join(k8sKubeconfigsDir, "admin.conf")); ok {
		node.AdminKubeconfig = c
	}
	if c, ok := l.loadConfigFileMeta(filepath.Join(k8sManifestsDir, "kube-apiserver.yaml")); ok {
		node.Manifests.KubeApiserver = c
	}
	if c, ok := l.loadConfigFileMeta(filepath.Join(k8sManifestsDir, "kube-controller-manager.yaml")); ok {
		node.Manifests.KubeContollerManager = c
	}
	if c, ok := l.loadConfigFileMeta(filepath.Join(k8sManifestsDir, "kube-scheduler.yaml")); ok {
		node.Manifests.KubeScheduler = c
	}
	if c, ok := l.loadConfigFileMeta(filepath.Join(k8sManifestsDir, "etcd.yaml")); ok {
		node.Manifests.Etcd = c
	}

	for _, proc := range loadProcesses(ctx) {
		switch proc.name {
		case "etcd":
			node.Components.Etcd = l.newK8sEtcdConfig(proc.flags)
		case "apiserver":
			// excludes apiserver process that is running on Bottlerocket OS.
			// identitied via --datastore-path flag that is not a valid flag of K8s's apiserver
			if _, ok := proc.flags["--datastore-path"]; !ok {
				node.Components.KubeApiserver = l.newK8sKubeApiserverConfig(proc.flags)
			}
		case "kube-apiserver":
			node.Components.KubeApiserver = l.newK8sKubeApiserverConfig(proc.flags)
		case "kube-controller-manager", "kube-controller", "controller-manager":
			node.Components.KubeControllerManager = l.newK8sKubeControllerManagerConfig(proc.flags)
		case "kube-scheduler":
			node.Components.KubeScheduler = l.newK8sKubeSchedulerConfig(proc.flags)
		case "kubelet":
			node.Components.Kubelet = l.newK8sKubeletConfig(proc.flags)
			node.ManagedEnvironment = l.detectManagedEnvironment(proc.flags, node.Components.Kubelet)
		case "kube-proxy":
			node.Components.KubeProxy = l.newK8sKubeProxyConfig(proc.flags)
		}
	}

	if len(l.errs) > 0 {
		node.Errors = make([]string, 0, len(l.errs))
		for _, err := range l.errs {
			node.Errors = append(node.Errors, err.Error())
		}
	}

	resourceType := "kubernetes_worker_node"
	if managedEnv := node.ManagedEnvironment; managedEnv != nil {
		switch managedEnv.Name {
		case "eks":
			resourceType = "aws_eks_worker_node"
		case "gke":
			resourceType = "gcp_gke_worker_node"
		case "aks":
			resourceType = "azure_aks_worker_node"
		}
	} else if node.Components.KubeApiserver != nil ||
		node.Components.Etcd != nil ||
		node.Components.KubeControllerManager != nil ||
		node.Components.KubeScheduler != nil {
		resourceType = "kubernetes_master_node"
	}

	return resourceType, &node
}

func (l *loader) detectManagedEnvironment(flags map[string]string, kubelet *K8sKubeletConfig) *K8sManagedEnvConfig {
	nodeLabels, ok := flags["--node-labels"]
	if ok {
		for _, label := range strings.Split(nodeLabels, ",") {
			label = strings.TrimSpace(label)
			switch {
			case strings.HasPrefix(label, "cloud.google.com/gke"):
				return &K8sManagedEnvConfig{
					Name: "gke",
				}
			case strings.HasPrefix(label, "eks.amazonaws.com/"):
				env := &K8sManagedEnvConfig{
					Name: "eks",
				}
				eksMeta, ok := l.loadConfigFileMeta("/etc/eks/release")
				if ok {
					env.Metadata = eksMeta.Content
				}
				return env
			case strings.HasPrefix(label, "kubernetes.azure.com/"):
				return &K8sManagedEnvConfig{
					Name: "aks",
				}
			}
		}
	}
	if kubelet != nil && kubelet.Kubeconfig != nil && kubelet.Kubeconfig.Kubeconfig != nil {
		// If we did not find any node-label with clear indication, we check
		// for the kubelet's kubeconfig server to see if it uses a managed
		// control-plane. This may be useful for custom images like
		// Bottlerocket running in EKS for instance.
		for _, cluster := range kubelet.Kubeconfig.Kubeconfig.Clusters {
			if clusterURL, err := url.Parse(cluster.Server); err == nil {
				switch {
				case strings.HasSuffix(clusterURL.Hostname(), ".eks.amazonaws.com"):
					return &K8sManagedEnvConfig{
						Name: "eks",
					}
				case strings.HasSuffix(clusterURL.Hostname(), ".azmk8s.io"):
					return &K8sManagedEnvConfig{
						Name: "aks",
					}
				}
			}
		}
		// For GKE, if we did not find any node-label, we check for the exec
		// command of the kubeconfig, relying on the gke-exec-auth-plugin
		// binary.
		for _, user := range kubelet.Kubeconfig.Kubeconfig.Users {
			if strings.HasSuffix(user.Exec.Command, "gke-exec-auth-plugin") {
				return &K8sManagedEnvConfig{
					Name: "gke",
				}
			}
		}
	}
	return nil
}

func (l *loader) loadMeta(name string, loadContent bool) (string, os.FileInfo, []byte, bool) {
	name = filepath.Join(l.hostroot, name)
	info, err := os.Stat(name)
	if err != nil {
		l.pushError(err)
		return name, nil, nil, false
	}
	if loadContent && info.IsDir() {
		return name, nil, nil, false
	}
	var b []byte
	const maxSize = 512 * 1024
	if loadContent && info.Size() < maxSize {
		f, err := os.Open(name)
		if err != nil {
			l.pushError(err)
		} else {
			defer f.Close()
			b, err = io.ReadAll(io.LimitReader(f, maxSize))
			if err != nil {
				l.pushError(err)
			}
		}
	}
	return name, info, b, true
}

func (l *loader) loadDirMeta(name string) *K8sDirMeta {
	_, info, _, ok := l.loadMeta(name, false)
	if !ok {
		return &K8sDirMeta{Path: name}
	}
	return &K8sDirMeta{
		Path:  name,
		User:  utils.GetFileUser(info),
		Group: utils.GetFileGroup(info),
		Mode:  uint32(info.Mode()),
	}
}

func (l *loader) loadServiceFileMeta(names []string) *K8sConfigFileMeta {
	for _, name := range names {
		meta, ok := l.loadConfigFileMeta(name)
		if ok {
			return meta
		}
	}
	return nil
}

func (l *loader) loadConfigFileMeta(name string) (*K8sConfigFileMeta, bool) {
	_, info, b, ok := l.loadMeta(name, true)
	if !ok {
		return &K8sConfigFileMeta{Path: name}, false
	}

	var content interface{}
	erru := json.Unmarshal(b, &content)
	if erru != nil {
		erru = yaml.Unmarshal(b, &content)
	}
	if erru != nil {
		content = string(b)
	}

	return &K8sConfigFileMeta{
		Path:    name,
		User:    utils.GetFileUser(info),
		Group:   utils.GetFileGroup(info),
		Mode:    uint32(info.Mode()),
		Content: content,
	}, true
}

func (l *loader) getConfigFromPath(meta *K8sConfigFileMeta, path string) (map[string]interface{}, string, bool) {
	if meta == nil || meta.Content == nil {
		return nil, "", false
	}
	content, ok := meta.Content.(map[string]interface{})
	if !ok {
		return nil, "", false
	}
	fields := strings.Split(path, ".")
	if len(fields) == 0 {
		return nil, "", false
	}
	if len(fields) > 1 {
		for _, field := range fields[:len(fields)-1] {
			content, ok = content[field].(map[string]interface{})
			if !ok {
				return nil, "", false
			}
		}
	}
	return content, fields[len(fields)-1], true
}

func (l *loader) configFileMetaHasField(meta *K8sConfigFileMeta, path string) bool {
	content, lastField, ok := l.getConfigFromPath(meta, path)
	if ok {
		_, hasField := content[lastField]
		return hasField
	}
	return false
}

func (l *loader) loadKubeletConfigFileMeta(name string) *K8sConfigFileMeta {
	meta, ok := l.loadConfigFileMeta(name)
	if !ok {
		return &K8sConfigFileMeta{Path: name}
	}
	content, ok := meta.Content.(map[string]interface{})
	if !ok {
		l.pushError(fmt.Errorf("kubelet configuration loaded from %q is not a valid configuration", name))
		return &K8sConfigFileMeta{Path: name}
	}
	if kind := content["kind"]; kind != "KubeletConfiguration" {
		l.pushError(fmt.Errorf(`kubelet configuration loaded from %q is expected to be of kind "KubeletConfiguration"`, name))
		return &K8sConfigFileMeta{Path: name}
	}
	// specifically parse key/cert files path to load their associated meta info.
	if keyPath, ok := content["tlsPrivateKeyFile"].(string); ok {
		content["tlsPrivateKeyFile"] = l.loadKeyFileMeta(keyPath)
	}
	if certPath, ok := content["tlsCertFile"].(string); ok {
		content["tlsCertFile"] = l.loadCertFileMeta(certPath)
	}
	if authentication, ok := content["authentication"].(map[string]interface{}); ok {
		if x509, ok := authentication["x509"].(map[string]interface{}); ok {
			if clientCAFile, ok := x509["clientCAFile"].(string); ok {
				x509["clientCAFile"] = l.loadCertFileMeta(clientCAFile)
			}
		}
	}
	return meta
}

func (l *loader) loadAdmissionConfigFileMeta(name string) *K8sAdmissionConfigFileMeta {
	_, info, b, ok := l.loadMeta(name, true)
	if !ok {
		return &K8sAdmissionConfigFileMeta{Path: name}
	}
	var content k8sAdmissionConfigSource
	if err := yaml.Unmarshal(b, &content); err != nil {
		l.pushError(err)
		return &K8sAdmissionConfigFileMeta{Path: name}
	}
	var result K8sAdmissionConfigFileMeta
	for _, plugin := range content.Plugins {
		added := &K8sAdmissionPluginConfigMeta{Name: plugin.Name}
		if plugin.Configuration != nil {
			added.Configuration = plugin.Configuration
		} else if plugin.Path != "" {
			if c, ok := l.loadConfigFileMeta(plugin.Path); ok {
				added.Configuration = c
			}
		}
		result.Plugins = append(result.Plugins, added)
	}
	result.Path = name
	result.User = utils.GetFileUser(info)
	result.Group = utils.GetFileGroup(info)
	result.Mode = uint32(info.Mode())
	return &result
}

func (l *loader) loadEncryptionProviderConfigFileMeta(name string) *K8sEncryptionProviderConfigFileMeta {
	_, info, b, ok := l.loadMeta(name, true)
	if !ok {
		return &K8sEncryptionProviderConfigFileMeta{Path: name}
	}
	var content K8sEncryptionProviderConfigFileMeta
	if err := yaml.Unmarshal(b, &content); err != nil {
		l.pushError(err)
		return &K8sEncryptionProviderConfigFileMeta{Path: name}
	}
	content.Path = name
	content.User = utils.GetFileUser(info)
	content.Group = utils.GetFileGroup(info)
	content.Mode = uint32(info.Mode())
	return &content
}

func (l *loader) loadTokenFileMeta(name string) *K8sTokenFileMeta {
	_, info, _, ok := l.loadMeta(name, false)
	if !ok {
		return &K8sTokenFileMeta{Path: name}
	}
	return &K8sTokenFileMeta{
		Path:  name,
		User:  utils.GetFileUser(info),
		Group: utils.GetFileGroup(info),
		Mode:  uint32(info.Mode()),
	}
}

func (l *loader) loadKeyFileMeta(name string) *K8sKeyFileMeta {
	_, info, _, ok := l.loadMeta(name, false)
	if !ok {
		return &K8sKeyFileMeta{Path: name}
	}
	var meta K8sKeyFileMeta
	meta.Path = name
	meta.User = utils.GetFileUser(info)
	meta.Group = utils.GetFileGroup(info)
	meta.Mode = uint32(info.Mode())
	return &meta
}

// https://github.com/kubernetes/kubernetes/blob/ad18954259eae3db51bac2274ed4ca7304b923c4/cmd/kubeadm/test/kubeconfig/util.go#L77-L87
func (l *loader) loadCertFileMeta(name string) *K8sCertFileMeta {
	fullpath, info, certData, ok := l.loadMeta(name, true)
	if !ok {
		return &K8sCertFileMeta{
			Path: name,
		}
	}
	meta := l.extractCertData(certData)
	if meta == nil {
		return &K8sCertFileMeta{
			Path: name,
		}
	}
	meta.Path = name
	meta.User = utils.GetFileUser(info)
	meta.Group = utils.GetFileGroup(info)
	meta.Mode = uint32(info.Mode())
	dir := filepath.Dir(fullpath)
	if dirInfo, err := os.Stat(dir); err == nil {
		meta.DirMode = uint32(dirInfo.Mode())
		meta.DirUser = utils.GetFileUser(dirInfo)
		meta.DirGroup = utils.GetFileGroup(dirInfo)
	}
	return meta
}

func (l *loader) extractCertData(certData []byte) *K8sCertFileMeta {
	const CertificateBlockType = "CERTIFICATE"
	certPemBlock, _ := pem.Decode(certData)
	if certPemBlock == nil {
		l.pushError(fmt.Errorf("could not PEM decode certificate data"))
		return nil
	}
	if certPemBlock.Type != CertificateBlockType {
		l.pushError(fmt.Errorf("decoded PEM does not start with correct block type"))
		return nil
	}
	c, err := x509.ParseCertificate(certPemBlock.Bytes)
	if err != nil {
		l.pushError(err)
		return nil
	}
	sn := c.SerialNumber.String()
	if sn == "0" {
		sn = ""
	}

	h256 := sha256.New()
	h256.Write(certPemBlock.Bytes)

	var data K8sCertFileMeta
	data.Certificate.Fingerprint = printSHA256Fingerprint(h256.Sum(nil))
	data.Certificate.SerialNumber = sn
	data.Certificate.SubjectKeyId = printColumnSeparatedHex(c.SubjectKeyId)
	data.Certificate.AuthorityKeyId = printColumnSeparatedHex(c.AuthorityKeyId)
	data.Certificate.CommonName = c.Subject.CommonName
	data.Certificate.Organization = c.Subject.Organization
	data.Certificate.DNSNames = c.DNSNames
	data.Certificate.IPAddresses = c.IPAddresses
	data.Certificate.NotAfter = &c.NotAfter
	data.Certificate.NotBefore = &c.NotBefore
	return &data
}

func (l *loader) loadKubeconfigMeta(name string) (*K8sKubeconfigMeta, bool) {
	_, info, b, ok := l.loadMeta(name, true)
	if !ok {
		return &K8sKubeconfigMeta{Path: name}, false
	}

	var source k8SKubeconfigSource
	erru := json.Unmarshal(b, &source)
	if erru != nil {
		erru = yaml.Unmarshal(b, &source)
	}
	if erru != nil {
		l.pushError(erru)
		return &K8sKubeconfigMeta{Path: name}, false
	}

	content := &K8SKubeconfig{
		Clusters: make(map[string]*K8sKubeconfigCluster),
		Users:    make(map[string]*K8sKubeconfigUser),
		Contexts: make(map[string]*K8sKubeconfigContext),
	}
	for _, cluster := range source.Clusters {
		var certAuth *K8sCertFileMeta
		if certAuthDataB64 := cluster.Cluster.CertificateAuthorityData; certAuthDataB64 != "" {
			certAuthData, err := base64.StdEncoding.DecodeString(certAuthDataB64)
			if err != nil {
				l.pushError(err)
			} else {
				certAuth = l.extractCertData(certAuthData)
			}
		} else if certAuthFile := cluster.Cluster.CertificateAuthority; certAuthFile != "" {
			certAuth = l.loadCertFileMeta(certAuthFile)
		}
		content.Clusters[cluster.Name] = &K8sKubeconfigCluster{
			Server:                cluster.Cluster.Server,
			TLSServerName:         cluster.Cluster.TLSServerName,
			InsecureSkipTLSVerify: cluster.Cluster.InsecureSkipTLSVerify,
			CertificateAuthority:  certAuth,
			ProxyURL:              cluster.Cluster.ProxyURL,
			DisableCompression:    cluster.Cluster.DisableCompression,
		}
	}
	for _, user := range source.Users {
		var clientCert *K8sCertFileMeta
		var clientKey *K8sKeyFileMeta
		if clientCertDataB64 := user.User.ClientCertificateData; clientCertDataB64 != "" {
			clientCertDataB64, err := base64.StdEncoding.DecodeString(clientCertDataB64)
			if err != nil {
				l.pushError(err)
			} else {
				clientCert = l.extractCertData(clientCertDataB64)
			}
		} else if clientCertFile := user.User.ClientCertificate; clientCertFile != "" {
			clientCert = l.loadCertFileMeta(clientCertFile)
		}
		if clientKeyFile := user.User.ClientKey; clientKeyFile != "" {
			clientKey = l.loadKeyFileMeta(clientKeyFile)
		}
		userTgt := &K8sKubeconfigUser{
			UseToken:          user.User.TokenFile != "" || user.User.Token != "",
			UsePassword:       user.User.Password != "",
			ClientCertificate: clientCert,
			ClientKey:         clientKey,
		}
		if exec := user.User.Exec; exec != nil {
			userTgt.Exec.APIVersion = exec.APIVersion
			userTgt.Exec.Command = exec.Command
			userTgt.Exec.Args = exec.Args
		}
		content.Users[user.Name] = userTgt
	}
	for _, context := range source.Contexts {
		content.Contexts[context.Name] = &K8sKubeconfigContext{
			Cluster:   context.Context.Cluster,
			User:      context.Context.User,
			Namespace: context.Context.Namespace,
		}
	}

	return &K8sKubeconfigMeta{
		Path:       name,
		User:       utils.GetFileUser(info),
		Group:      utils.GetFileGroup(info),
		Mode:       uint32(info.Mode()),
		Kubeconfig: content,
	}, true
}

// in OpenSSH >= 2.6, a fingerprint is now displayed as base64 SHA256.
func printSHA256Fingerprint(f []byte) string {
	return fmt.Sprintf("SHA256:%s", strings.TrimSuffix(base64.StdEncoding.EncodeToString(f), "="))
}

func printColumnSeparatedHex(d []byte) string {
	h := strings.ToUpper(hex.EncodeToString(d))
	var sb strings.Builder
	for i, r := range h {
		sb.WriteRune(r)
		if i%2 == 1 && i != len(h)-1 {
			sb.WriteRune(':')
		}
	}
	return sb.String()
}

func (l *loader) loadProcesses(ctx context.Context) []proc {
	var procs []proc
	processes, err := process.ProcessesWithContext(ctx)
	if err != nil {
		l.pushError(err)
		return nil
	}
	for _, p := range processes {
		name, err := p.Name()
		if err != nil {
			l.pushError(err)
			continue
		}
		switch name {
		case "etcd",
			"kube-apiserver", "apiserver",
			"kube-controller-manager", "kube-controller", "controller-manager",
			"kube-scheduler", "kubelet", "kube-proxy":
			cmdline, err := p.CmdlineSlice()
			if err != nil {
				l.pushError(err)
			} else {
				procs = append(procs, buildProc(name, cmdline))
			}
		}
	}
	return procs
}

func (l *loader) pushError(err error) {
	if err != nil && !os.IsNotExist(err) {
		l.errs = append(l.errs, err)
	}
}

func (l *loader) parseBool(v string) *bool {
	if v == "" {
		return nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.pushError(err)
		return nil
	}
	return &b
}

//nolint:unused,deadcode
func (l *loader) parseFloat(v string) *float64 {
	if v == "" {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		l.pushError(err)
		return nil
	}
	return &f
}

func (l *loader) parseInt(v string) *int {
	if v == "" {
		return nil
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		l.pushError(err)
		return nil
	}
	return &i
}

func (l *loader) parseDuration(v string) *time.Duration {
	if v == "" {
		return nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.pushError(err)
		return nil
	}
	return &d
}

func buildProc(name string, cmdline []string) proc {
	p := proc{name: name}
	if len(cmdline) > 1 {
		cmdline = cmdline[1:]
		p.flags = make(map[string]string)
		pendingFlagValue := false
		for i, arg := range cmdline {
			if strings.HasPrefix(arg, "-") {
				parts := strings.SplitN(arg, "=", 2)
				if len(parts) == 2 {
					p.flags[parts[0]] = parts[1]
				} else {
					p.flags[parts[0]] = ""
					pendingFlagValue = true
				}
			} else {
				if pendingFlagValue {
					p.flags[cmdline[i-1]] = arg
				} else {
					p.flags[arg] = ""
				}
			}
		}
	}
	return p
}
