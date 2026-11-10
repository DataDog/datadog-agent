// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
)

// CacheKey identifies a cached value by normalized path and gNMI path keys.
type CacheKey struct {
	Path string
	// Keys is the legacy last-occurrence projection, used for metric tags.
	Keys map[string]string
	// ElementKeys preserves ordered, segment-scoped identity. Positions follow
	// CompilePath elements (the legacy synthetic OpenConfig wrapper is omitted).
	// Nil remains supported for older snapshots with unambiguous flat keys.
	ElementKeys []map[string]string
}

func (k CacheKey) cacheKey() string {
	return strconv.Quote(k.Path) + "{" + formatQuotedKeys(k.Keys) + "}" + k.elementKeysID()
}

// String returns a stable string representation for logging and debugging.
func (k CacheKey) String() string {
	return fmt.Sprintf("%s{%s}", k.Path, k.KeysIdentity())
}

// KeysIdentity distinguishes repeated same-named keys without changing legacy
// identities for the common case where each key name occurs only once.
func (k CacheKey) KeysIdentity() string {
	seen := make(map[string]bool)
	for _, keys := range k.ElementKeys {
		for name := range keys {
			if seen[name] {
				return "scoped:" + k.elementKeysID()
			}
			seen[name] = true
		}
	}
	return formatKeys(k.Keys)
}

func (k CacheKey) elementKeysID() string {
	var id strings.Builder
	for i, keys := range k.ElementKeys {
		if len(keys) > 0 {
			fmt.Fprintf(&id, "[%d{%s}]", i, formatQuotedKeys(keys))
		}
	}
	return id.String()
}

// MatchesSelectorKeys applies predicates at their ordered schema elements.
// An older flat snapshot cannot prove a selector with repeated key names.
func (k CacheKey) MatchesSelectorKeys(selector CacheKey) bool {
	seen := make(map[string]bool)
	for i, predicates := range selector.ElementKeys {
		for name, value := range predicates {
			actual, exists := k.Keys[name]
			if len(k.ElementKeys) > 0 {
				exists = false
				if i < len(k.ElementKeys) {
					actual, exists = k.ElementKeys[i][name]
				}
			} else if seen[name] {
				return false
			}
			seen[name] = true
			if !exists || (value != "*" && actual != value) {
				return false
			}
		}
	}
	return true
}

// MatchesLookupKeys merges caller keys with selector predicates. Caller keys
// retain their legacy meaning (the last occurrence); predicates bind parents
// at their own positions. Every sample key must be accounted for, and callers
// cannot override a conflicting literal predicate.
func (k CacheKey) MatchesLookupKeys(selector CacheKey, caller map[string]string) bool {
	if !k.MatchesSelectorKeys(selector) {
		return false
	}
	for name, value := range caller {
		if actual, exists := k.Keys[name]; !exists || actual != value {
			return false
		}
	}
	if len(k.ElementKeys) == 0 {
		for name := range k.Keys {
			if _, supplied := caller[name]; !supplied {
				if _, bound := selector.Keys[name]; !bound {
					return false
				}
			}
		}
		return true
	}
	last := make(map[string]int)
	for i, keys := range k.ElementKeys {
		for name := range keys {
			last[name] = i
		}
	}
	for i, keys := range k.ElementKeys {
		for name := range keys {
			if i < len(selector.ElementKeys) {
				if _, bound := selector.ElementKeys[i][name]; bound {
					continue
				}
			}
			if _, supplied := caller[name]; !supplied || last[name] != i {
				return false
			}
		}
	}
	return true
}

func formatKeys(keys map[string]string) string {
	return formatKeysWith(keys, false)
}

func formatQuotedKeys(keys map[string]string) string {
	return formatKeysWith(keys, true)
}

func formatKeysWith(keys map[string]string, quoted bool) string {
	if len(keys) == 0 {
		return ""
	}

	names := make([]string, 0, len(keys))
	for name := range keys {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		value := keys[name]
		if quoted {
			name = strconv.Quote(name)
			value = strconv.Quote(value)
		}
		parts = append(parts, name+"="+value)
	}
	return strings.Join(parts, ",")
}

type pathElement struct {
	name string
	keys map[string]string
}

type normalizedPath struct {
	origin   string
	target   string
	elements []pathElement
}

func normalizedPathFromGNMIPath(path *gnmipb.Path) normalizedPath {
	if path == nil {
		return normalizedPath{}
	}

	elements := make([]pathElement, 0, len(path.GetElem()))
	for _, elem := range path.GetElem() {
		elements = append(elements, pathElement{
			name: elem.GetName(),
			keys: cloneKeys(elem.GetKey()),
		})
	}
	return normalizedPath{
		origin:   path.GetOrigin(),
		target:   path.GetTarget(),
		elements: elements,
	}
}

func (p normalizedPath) id() string {
	if elements, ok := p.openConfigElements(); ok {
		p.elements = elements
	}
	var builder strings.Builder
	builder.WriteString("origin=")
	builder.WriteString(strconv.Quote(p.origin))
	builder.WriteString(",target=")
	builder.WriteString(strconv.Quote(p.target))
	for _, elem := range p.elements {
		builder.WriteByte('/')
		builder.WriteString(strconv.Quote(elem.name))
		builder.WriteByte('{')
		builder.WriteString(formatQuotedKeys(elem.keys))
		builder.WriteByte('}')
	}
	return builder.String()
}

func (p normalizedPath) cacheKey() CacheKey {
	segments := make([]string, 0, len(p.elements))
	for _, elem := range p.elements {
		segments = append(segments, elem.name)
	}
	keys := make(map[string]string)
	elements := p.elements
	if p.origin == "" && len(elements) > 0 && elements[0].name == "openconfig" && len(elements[0].keys) == 0 {
		elements = elements[1:]
	}
	var elementKeys []map[string]string
	for _, elem := range p.elements {
		for name, value := range elem.keys {
			keys[name] = value
		}
	}
	if len(keys) == 0 {
		keys = nil
	} else {
		elementKeys = make([]map[string]string, len(elements))
		for i, elem := range elements {
			elementKeys[i] = cloneKeys(elem.keys)
		}
	}
	if len(segments) == 0 && p.origin == "" {
		return CacheKey{Keys: keys}
	}
	path := "/" + strings.Join(segments, "/")
	if p.origin != "" {
		path = p.origin + ":" + path
	}
	return CacheKey{Path: path, Keys: keys, ElementKeys: elementKeys}
}

func (p normalizedPath) matchesPrefix(prefix normalizedPath) bool {
	if p.origin != prefix.origin {
		return false
	}
	if prefix.target != "" && p.target != prefix.target {
		return false
	}
	if elements, ok := p.openConfigElements(); ok {
		if prefixElements, prefixOK := prefix.openConfigElements(); prefixOK {
			p.elements, prefix.elements = elements, prefixElements
		}
	}
	if len(prefix.elements) > len(p.elements) {
		return false
	}
	for i, prefixElem := range prefix.elements {
		elem := p.elements[i]
		if elem.name != prefixElem.name {
			return false
		}
		for name, value := range prefixElem.keys {
			if elem.keys[name] != value {
				return false
			}
		}
	}
	return true
}

// openConfigElements is the same finite compatibility surface used by lookup,
// applied structurally for update identity and subtree deletion. Origins are
// deliberately not rewritten, so originless deletes remain origin-scoped.
func (p normalizedPath) openConfigElements() ([]pathElement, bool) {
	if p.origin != "" && p.origin != "openconfig" {
		return nil, false
	}
	elements := p.elements
	if p.origin == "" && len(elements) > 0 && elements[0].name == "openconfig" && len(elements[0].keys) == 0 {
		elements = elements[1:]
	}
	out := make([]pathElement, len(elements))
	for i, elem := range elements {
		if module, local, qualified := splitModuleQualifiedName(elem.name); qualified {
			if module != "openconfig" && !strings.HasPrefix(module, "openconfig-") {
				return nil, false
			}
			elem.name = local
		}
		out[i] = elem
	}
	if len(out) == 0 || !isOpenConfigRootSegment(out[0].name) {
		return nil, false
	}
	return out, true
}

func cacheKeyFromGNMIPath(path *gnmipb.Path) CacheKey {
	return normalizedPathFromGNMIPath(path).cacheKey()
}

func joinGNMIPaths(prefix, path *gnmipb.Path) *gnmipb.Path {
	joined := &gnmipb.Path{}
	if prefix != nil {
		joined.Origin = prefix.GetOrigin()
		joined.Target = prefix.GetTarget()
		joined.Elem = append(joined.Elem, prefix.GetElem()...)
	}
	if path != nil {
		if path.GetOrigin() != "" {
			joined.Origin = path.GetOrigin()
		}
		if path.GetTarget() != "" {
			joined.Target = path.GetTarget()
		}
		joined.Elem = append(joined.Elem, path.GetElem()...)
	}
	return joined
}

// SelectorKey compiles a configured selector to its cache path and predicates.
// Wildcard predicates remain wildcards; callers decide whether to require an
// exact key set (Get) or select matching samples (reporting).
func SelectorKey(value string) (CacheKey, error) {
	path, err := config.CompilePath(value, nil)
	if err != nil {
		return CacheKey{}, err
	}
	return cacheKeyFromGNMIPath(path), nil
}

// PathAliases retains the historical finite OpenConfig compatibility surface.
// Raw module-qualified names are always preserved. Native origins never acquire
// aliases, and only OpenConfig module qualifiers can be elided for compatibility.
func PathAliases(value string) []string {
	path, err := config.CompilePath(value, nil)
	if err != nil {
		return []string{value}
	}
	raw := cacheKeyFromGNMIPath(path).Path
	aliases := []string{raw}
	elements, ok := normalizedPathFromGNMIPath(path).openConfigElements()
	if !ok {
		return aliases
	}
	segments := make([]string, 0, len(elements))
	for _, elem := range elements {
		segments = append(segments, elem.name)
	}
	local := "/" + strings.Join(segments, "/")
	for _, alias := range []string{"openconfig:" + local, "/openconfig" + local, local} {
		if alias != raw {
			aliases = append(aliases, alias)
		}
	}
	return aliases
}

func subscribePathFromMetric(metric config.MetricConfig) (*gnmipb.Path, error) {
	return config.CompilePath(metric.Path, metric.SubscriptionKeys())
}
