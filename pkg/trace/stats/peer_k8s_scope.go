// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"sort"
	"strings"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
)

const (
	peerK8sClusterUIDTagPrefix = string(semantics.ConceptPeerK8sClusterUID) + ":"
	peerK8sNamespaceTagPrefix  = string(semantics.ConceptPeerK8sNamespace) + ":"
	peerK8sTagPrefix           = "peer.k8s."
	kubeNamespaceTagPrefix     = "kube_namespace:"

	// defaultClusterDomain is the Kubernetes DNS domain. The Agent does not know
	// the cluster's domain, so fully qualified names under another domain get no tags.
	defaultClusterDomain = "cluster.local"
	svcSuffix            = ".svc"
	svcFQDNSuffix        = svcSuffix + "." + defaultClusterDomain

	// blockedIPAddress is what obfuscate.QuantizePeerIPAddresses replaces IP addresses with.
	blockedIPAddress = "blocked-ip-address"
)

// peerK8sScope adds peer.k8s.cluster.uid and peer.k8s.namespace to the peer tags
// of stats whose peer hostname is a fully qualified in-cluster name or a short name
// that resolves in the cluster under standard DNS: the candidate scope for a
// best-effort lookup of the Kubernetes service. It is built once per payload.
// The zero value is inactive.
type peerK8sScope struct {
	// clusterUIDTag is the peer.k8s.cluster.uid tag, empty when inactive.
	clusterUIDTag string
	// callerNamespace is the kube_namespace of the container that sent the payload,
	// used to resolve bare service names. May be empty.
	callerNamespace string
	// hostnamePrefixes are the "key:" prefixes of the peer.hostname sources, by precedence.
	hostnamePrefixes []string
}

// newPeerK8sScope returns an active scope, or an inactive one when clusterUIDTag is empty.
func newPeerK8sScope(clusterUIDTag, callerNamespace string) peerK8sScope {
	if clusterUIDTag == "" {
		return peerK8sScope{}
	}
	return peerK8sScope{clusterUIDTag: clusterUIDTag, callerNamespace: callerNamespace, hostnamePrefixes: peerHostnamePrefixes()}
}

// peerHostnameSources caches the peer.hostname source prefixes of a registry.
type peerHostnameSources struct {
	fingerprint string
	prefixes    []string
}

var peerHostnameSourcesCache atomic.Pointer[peerHostnameSources]

// peerHostnamePrefixes returns the "key:" prefixes of the peer tags that are
// sources of peer.hostname, by precedence. Stats peer tags are keyed by source
// attribute, so a span may carry out.host, server.address... instead of
// peer.hostname. The list is rebuilt when the live registry changes.
func peerHostnamePrefixes() []string {
	r := semantics.DefaultRegistry()
	fingerprint := r.Fingerprint()
	if s := peerHostnameSourcesCache.Load(); s != nil && s.fingerprint == fingerprint {
		return s.prefixes
	}
	infos := r.GetAttributePrecedence(semantics.ConceptPeerHostname)
	prefixes := make([]string, 0, len(infos))
	for _, info := range infos {
		prefixes = append(prefixes, info.Name+":")
	}
	peerHostnameSourcesCache.Store(&peerHostnameSources{fingerprint: fingerprint, prefixes: prefixes})
	return prefixes
}

// peerHostname returns the first non-empty value of the peer.hostname sources in
// peerTags, by precedence, like the backend.
func peerHostname(peerTags, prefixes []string) (string, bool) {
	for _, p := range prefixes {
		for _, t := range peerTags {
			if v, ok := strings.CutPrefix(t, p); ok && v != "" {
				return v, true
			}
		}
	}
	return "", false
}

// namespace returns the namespace for the peer.k8s.namespace tag of peerTags.
func (s peerK8sScope) namespace(peerTags []string) (string, bool) {
	if s.clusterUIDTag == "" || len(peerTags) == 0 {
		return "", false
	}
	for _, t := range peerTags {
		if strings.HasPrefix(t, peerK8sTagPrefix) {
			// already scoped, e.g. by apm_config.peer_tags
			return "", false
		}
	}
	host, ok := peerHostname(peerTags, s.hostnamePrefixes)
	if !ok {
		return "", false
	}
	// A lower precedence source is never used: the peer is the one the backend sees.
	return peerK8sNamespace(host, s.callerNamespace)
}

// appendTags returns peerTags with the peer.k8s.* tags added when the peer
// hostname is an in-cluster name, sorted. It may modify peerTags and reuse
// its backing array. It returns peerTags unchanged, without allocating, otherwise.
func (s peerK8sScope) appendTags(peerTags []string) []string {
	namespace, ok := s.namespace(peerTags)
	if !ok {
		return peerTags
	}
	peerTags = append(peerTags, s.clusterUIDTag, peerK8sNamespaceTagPrefix+namespace)
	sort.Strings(peerTags)
	return peerTags
}

// appendTagsCopy is appendTags that leaves peerTags untouched, for slices owned by the caller.
func (s peerK8sScope) appendTagsCopy(peerTags []string) []string {
	namespace, ok := s.namespace(peerTags)
	if !ok {
		return peerTags
	}
	scoped := make([]string, len(peerTags), len(peerTags)+2)
	copy(scoped, peerTags)
	scoped = append(scoped, s.clusterUIDTag, peerK8sNamespaceTagPrefix+namespace)
	sort.Strings(scoped)
	return scoped
}

// needsCallerNamespace reports whether the peer hostname of peerTags is a bare
// name, which resolves in the caller's namespace.
func (s peerK8sScope) needsCallerNamespace(peerTags []string) bool {
	host, ok := peerHostname(peerTags, s.hostnamePrefixes)
	return ok && isBareServiceName(strings.ToLower(stripPort(host)))
}

// isBareServiceName reports whether the lowercase, port-less host is a service
// name without namespace, which resolves in the caller's namespace.
func isBareServiceName(host string) bool {
	return host != "localhost" && !strings.Contains(host, blockedIPAddress) && isDNS1123Label(host)
}

// peerK8sNamespace returns the candidate Kubernetes namespace of host. Kubernetes
// DNS defines what fully qualified in-cluster names mean:
//   - "a.b.svc.cluster.local" and "a.b.svc" name service a in namespace b,
//   - "p.a.b.svc.cluster.local" and "p.a.b.svc" name pod p of headless service a in namespace b.
//
// Short names resolve through the search path of a pod in callerNamespace under
// standard cluster DNS. Custom DNS policies or searches and host networking can
// resolve them elsewhere, so their namespace is only a hint:
//   - "a.b" resolves to service a in namespace b,
//   - a bare "a" resolves to service a in callerNamespace.
//
// A trailing ":<port>" is ignored. A trailing dot makes host absolute: only the
// ".svc.cluster.local." forms apply. It returns false for any other name, IP
// addresses, localhost, and names with labels that are not DNS-1123 labels, which
// includes names with a scheme. It does not check that the service exists.
func peerK8sNamespace(host, callerNamespace string) (string, bool) {
	host = strings.ToLower(stripPort(host))
	if strings.Contains(host, blockedIPAddress) {
		return "", false
	}
	host, absolute := strings.CutSuffix(host, ".")
	if rest, ok := strings.CutSuffix(host, svcFQDNSuffix); ok {
		if ns, ok := svcNamespace(rest); ok {
			return ns, true
		}
	}
	if absolute {
		return "", false
	}
	if rest, ok := strings.CutSuffix(host, svcSuffix); ok {
		if ns, ok := svcNamespace(rest); ok {
			return ns, true
		}
	}
	// IPv4 addresses have three dots and IPv6 addresses contain colons, so the
	// shapes below never match them.
	switch strings.Count(host, ".") {
	case 0:
		if !isBareServiceName(host) || !isDNS1123Label(callerNamespace) {
			return "", false
		}
		return callerNamespace, true
	case 1:
		return serviceNamespace(host)
	}
	return "", false
}

// stripPort removes a trailing ":<digits>" from host.
func stripPort(host string) string {
	i := strings.LastIndexByte(host, ':')
	if i < 0 || i == len(host)-1 {
		return host
	}
	for j := i + 1; j < len(host); j++ {
		if host[j] < '0' || host[j] > '9' {
			return host
		}
	}
	return host[:i]
}

// svcNamespace returns b for the "a.b" and "p.a.b" names in front of ".svc".
func svcNamespace(name string) (string, bool) {
	if ns, ok := serviceNamespace(name); ok {
		return ns, true
	}
	pod, service, ok := strings.Cut(name, ".")
	if !ok || !isDNS1123Label(pod) {
		return "", false
	}
	return serviceNamespace(service)
}

// serviceNamespace returns b for "a.b" when a and b are DNS-1123 labels.
func serviceNamespace(name string) (string, bool) {
	service, namespace, ok := strings.Cut(name, ".")
	if !ok || !isDNS1123Label(service) || !isDNS1123Label(namespace) {
		return "", false
	}
	return namespace, true
}

// isDNS1123Label reports whether s is a lowercase RFC 1123 label, the format of
// Kubernetes namespace names.
func isDNS1123Label(s string) bool {
	if len(s) == 0 || len(s) > 63 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-' && i > 0 && i < len(s)-1:
		default:
			return false
		}
	}
	return true
}

// KubeNamespaceFromTags returns the kube_namespace tag value from container tags.
func KubeNamespaceFromTags(tags []string) string {
	for _, t := range tags {
		if v, ok := strings.CutPrefix(t, kubeNamespaceTagPrefix); ok {
			return v
		}
	}
	return ""
}

// peerK8sClusterUIDTag caches the peer.k8s.cluster.uid tag built from the last
// cluster UID, so building a peerK8sScope does not allocate per payload.
type peerK8sClusterUIDTag struct {
	last atomic.Pointer[[2]string] // {uid, tag}
}

// get returns the peer.k8s.cluster.uid tag, or "" when the k8s scope is disabled
// or the cluster UID is not known (yet).
func (c *peerK8sClusterUIDTag) get(conf *config.AgentConfig) string {
	if !conf.PeerTagsAggregation || !conf.PeerTagsK8sScope || conf.KubeClusterUID == nil {
		return ""
	}
	uid := conf.KubeClusterUID()
	if uid == "" {
		return ""
	}
	if last := c.last.Load(); last != nil && last[0] == uid {
		return last[1]
	}
	tag := peerK8sClusterUIDTagPrefix + uid
	c.last.Store(&[2]string{uid, tag})
	return tag
}
