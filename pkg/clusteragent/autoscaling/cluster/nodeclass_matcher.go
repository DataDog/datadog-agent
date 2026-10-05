// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func attemptNodeClassMatch(ncList []unstructured.Unstructured, knp *karpenterv1.NodePool) (string, bool) {
	// Extract the desired OS and architecture from the nodepool. Only requirements that pin
	// down a single desired value are usable here: NotIn/Exists/Gt/etc. don't tell us which
	// value the NodeClass name should contain, and a requirement accepting several values (e.g.
	// arch In [amd64, arm64]) doesn't tell us which one a matched NodeClass must cover, so
	// guessing one would risk silently binding the NodePool to a NodeClass that doesn't support
	// the other accepted values. Multiple requirements on the same key are ANDed together (a
	// node must satisfy all of them), so the set of values a NodeClass may legitimately
	// advertise for that key is the intersection across all matching requirements, not their
	// union; singleValue then requires that intersection to boil down to exactly one value.
	var osGroups, archGroups [][]string
	for _, req := range knp.Spec.Template.Spec.Requirements {
		if req.Operator != corev1.NodeSelectorOpIn || len(req.Values) == 0 {
			continue
		}
		switch req.Key {
		case corev1.LabelOSStable:
			osGroups = append(osGroups, req.Values)
		case corev1.LabelArchStable:
			archGroups = append(archGroups, req.Values)
		}
	}
	os, osOK := singleValue(intersectValues(osGroups))
	arch, archOK := singleValue(intersectValues(archGroups))
	if !osOK && !archOK {
		return "", false
	}

	// tokenizeNames is used by every matching strategy below, so it only needs to run once.
	names := tokenizeNames(ncList)
	// conflictingOS/conflictingArch are used by every contradiction check below, so they only
	// need to be computed once per dimension per call.
	conflictingOS := conflictingValues(knownOSValues, os)
	conflictingArch := conflictingValues(knownArchValues, arch)

	// Prefer the NodeClass's own kubernetes.io/os and kubernetes.io/arch labels when present: an
	// exact label match carries no risk of the name-parsing false positives the token-based
	// matching below is prone to. This only kicks in for NodeClasses that were actually labeled
	// this way; it falls through to name-based matching otherwise. If both a label match and a
	// name match exist but disagree, that's a genuine ambiguity (e.g. a stale label on a
	// neutrally-named NodeClass): trust neither over the other and let the caller see ambiguity
	// rather than silently returning the wrong NodeClass. A label match that is itself ambiguous
	// (more than one NodeClass carries the matching labels) is a hard stop too: a merely
	// coincidental unique name match must not override genuine label-based ambiguity. But the
	// converse isn't true: a name match that's ambiguous doesn't get to override a clean label
	// match either, since name-token parsing is the less trustworthy signal (see reconcileTrusted).
	labelName, labelOK, labelAmbiguous := attemptLabelMatch(names, os, osOK, conflictingOS, arch, archOK, conflictingArch)

	// Require a name match against every known dimension at once: nameMatchesAllGroups treats an
	// unset (nil) dimension as vacuously satisfied, so this naturally degrades to matching on
	// arch alone or os alone when only one of them is known.
	nameName, nameOK, nameAmbiguous := uniqueNameMatch(names, [][]string{asGroup(arch, archOK), asGroup(os, osOK)})
	combinedName, combinedOK, combinedAmbiguous := reconcileTrusted(labelName, labelOK, labelAmbiguous, nameName, nameOK, nameAmbiguous)
	if combinedOK {
		return combinedName, true
	}
	if combinedAmbiguous {
		return "", false
	}

	// If both dimensions are known but no NodeClass name satisfies both, fall back to matching a
	// single dimension alone -- but only among NodeClasses whose name or label doesn't explicitly
	// name a conflicting value for the *other* known dimension (e.g. a NodeClass named
	// "windows-amd64", or one labeled kubernetes.io/os=windows, must not be picked via arch alone
	// when os=linux is required). A NodeClass that simply doesn't mention the other dimension at
	// all (e.g. "ec2nodeclass-amd64", unlabeled) is fine to fall back on, since it doesn't
	// contradict anything.
	if archOK && osOK {
		archCandidates := excludingConflicts(names, corev1.LabelOSStable, conflictingOS)
		osCandidates := excludingConflicts(names, corev1.LabelArchStable, conflictingArch)

		archName, archMatched, archAmbiguous := matchDimension(archCandidates, corev1.LabelArchStable, arch, conflictingArch)
		osName, osMatched, osAmbiguous := matchDimension(osCandidates, corev1.LabelOSStable, os, conflictingOS)
		if archAmbiguous || osAmbiguous {
			return "", false
		}
		name, ok, _ := reconcileTrusted(archName, archMatched, false, osName, osMatched, false)
		return name, ok
	}

	return "", false
}

// reconcileTrusted combines a trusted signal (e.g. a label match, which "carries no risk of the
// name-parsing false positives" the token-based matching is prone to) with a less-trusted fallback
// signal into one decision, using the same (name, ok, ambiguous) tri-state shape as the rest of
// this file: ok means resolved, ambiguous means a hard stop, and both false means neither signal
// had an opinion. The trusted signal wins whenever it resolves cleanly, even if the fallback
// signal happens to be ambiguous elsewhere -- an ambiguous fallback carries no information, so it
// must not veto a clean trusted resolution. If the trusted signal is itself ambiguous, or the two
// signals both resolve but disagree, that's a genuine ambiguity and a hard stop regardless of the
// fallback. Only when the trusted signal has no opinion at all does the fallback's own resolution
// or ambiguity take over.
func reconcileTrusted(trustedName string, trustedOK, trustedAmbiguous bool, fallbackName string, fallbackOK, fallbackAmbiguous bool) (name string, ok bool, ambiguous bool) {
	switch {
	case trustedAmbiguous:
		return "", false, true
	case trustedOK && fallbackOK:
		if trustedName == fallbackName {
			return trustedName, true, false
		}
		return "", false, true
	case trustedOK:
		return trustedName, true, false
	default:
		return fallbackName, fallbackOK, fallbackAmbiguous
	}
}

// matchDimension resolves the single NodeClass among candidates that satisfies one dimension
// (os or arch alone), preferring an exact labelKey label match and falling back to a name-token
// match on want (see reconcileTrusted for how the two are combined). conflicting excludes a
// candidate from the label match when its own name contradicts the label it matched on (see
// matchByLabel). ambiguous reports whether resolution hit a genuine conflict (as opposed to
// neither signal having an opinion), since that's evidence too strong to let the caller guess via
// another dimension instead.
func matchDimension(candidates []namedTokens, labelKey, want string, conflicting []string) (name string, ok bool, ambiguous bool) {
	labelName, labelOK, labelAmbiguous := matchByLabel(candidates, labelKey, want, conflicting)
	nameName, nameOK, nameAmbiguous := uniqueNameMatch(candidates, [][]string{{want}})
	return reconcileTrusted(labelName, labelOK, labelAmbiguous, nameName, nameOK, nameAmbiguous)
}

// matchByLabel returns the name of the single namedTokens entry whose labelKey label
// case-insensitively equals want, excluding any entry whose own name explicitly names a
// conflicting value (e.g. a NodeClass named "arm64-nodeclass" but stale-labeled
// kubernetes.io/arch=amd64 shouldn't be trusted just because of its label). ok is false if zero or
// more than one entry matches; ambiguous distinguishes the "more than one" case (a real conflict)
// from "zero" (no opinion).
func matchByLabel(names []namedTokens, labelKey, want string, conflicting []string) (name string, ok bool, ambiguous bool) {
	var matched []string
	for _, n := range names {
		if !strings.EqualFold(n.labels[labelKey], want) {
			continue
		}
		if nameMatchesAnyToken(n.parts, conflicting) {
			continue
		}
		matched = append(matched, n.name)
	}
	return resolveUnique(matched)
}

// attemptLabelMatch returns the name of the single NodeClass whose kubernetes.io/os and
// kubernetes.io/arch labels match the known os/arch dimensions. ok is false if zero or more than
// one NodeClass matches; ambiguous distinguishes the "more than one" case (a real conflict) from
// "zero" (no opinion). A NodeClass with no such label naturally fails to match a known dimension,
// since the label lookup returns "" which never equals a real os/arch value. A NodeClass whose own
// name explicitly names a *different*, known os/arch than the dimension it matched on is excluded
// too: a coincidental or stale label shouldn't override an explicit naming contradiction.
func attemptLabelMatch(names []namedTokens, os string, osOK bool, conflictingOS []string, arch string, archOK bool, conflictingArch []string) (name string, ok bool, ambiguous bool) {
	var matched []string
	for _, n := range names {
		if osOK && !strings.EqualFold(n.labels[corev1.LabelOSStable], os) {
			continue
		}
		if archOK && !strings.EqualFold(n.labels[corev1.LabelArchStable], arch) {
			continue
		}
		if osOK && nameMatchesAnyToken(n.parts, conflictingOS) {
			continue
		}
		if archOK && nameMatchesAnyToken(n.parts, conflictingArch) {
			continue
		}
		matched = append(matched, n.name)
	}
	return resolveUnique(matched)
}

// knownOSValues and knownArchValues are the os/arch values Kubernetes nodes report via the
// kubernetes.io/os and kubernetes.io/arch labels -- i.e. the OS/architecture combinations
// Kubernetes itself ships release binaries for, since a node can only report a value its
// kubelet binary actually runs on. Used to tell a NodeClass name that explicitly names a
// *different* os/arch (a real conflict) apart from one that simply doesn't mention that
// dimension at all (not a conflict).
var (
	knownOSValues   = []string{"linux", "windows"}
	knownArchValues = []string{"386", "amd64", "arm", "arm64", "ppc64le", "s390x"}
)

// conflictingValues returns the values in knownValues other than want, case-insensitively.
func conflictingValues(knownValues []string, want string) []string {
	conflicting := make([]string, 0, len(knownValues))
	for _, v := range knownValues {
		if !strings.EqualFold(v, want) {
			conflicting = append(conflicting, v)
		}
	}
	return conflicting
}

// excludingConflicts returns the subset of names that don't contradict any value in conflicting,
// whether by a name-token segment matching one of them or by the labelKey label case-insensitively
// matching one of them. A name with no such label, or an unrelated label value, isn't a conflict.
func excludingConflicts(names []namedTokens, labelKey string, conflicting []string) []namedTokens {
	filtered := make([]namedTokens, 0, len(names))
	for _, n := range names {
		if nameMatchesAnyToken(n.parts, conflicting) {
			continue
		}
		if nameMatchesAnyToken([]string{n.labels[labelKey]}, conflicting) {
			continue
		}
		filtered = append(filtered, n)
	}
	return filtered
}

// otherNames returns the names in ncList other than exclude.
func otherNames(ncList []unstructured.Unstructured, exclude string) []string {
	others := make([]string, 0, len(ncList))
	for _, nc := range ncList {
		if name := nc.GetName(); name != exclude {
			others = append(others, name)
		}
	}
	return others
}

// singleValue returns values[0] and true if values contains exactly one distinct, non-empty
// value (case-insensitively), or "", false otherwise. Values are compared case-insensitively for
// consistency with the case-insensitive label/name matching elsewhere in this file -- this is why
// this intersection isn't done via sigs.k8s.io/karpenter/pkg/scheduling.Requirements, whose
// Requirement.Intersection is case-sensitive and would treat e.g. "amd64" and "AMD64" as distinct
// values that never intersect. An empty string is never treated as a legitimate os/arch value,
// since Kubernetes label validation permits it and it would otherwise spuriously match any
// NodeClass that has no such label set.
func singleValue(values []string) (string, bool) {
	distinct := make(map[string]struct{}, len(values))
	for _, v := range values {
		if v == "" {
			return "", false
		}
		distinct[strings.ToLower(v)] = struct{}{}
	}
	if len(distinct) != 1 {
		return "", false
	}
	return values[0], true
}

// intersectValues returns the case-insensitive intersection of every group in groups: the values
// that appear in all of them. Returns nil if groups is empty.
func intersectValues(groups [][]string) []string {
	if len(groups) == 0 {
		return nil
	}
	counts := make(map[string]int)
	firstSeen := make(map[string]string)
	for _, group := range groups {
		seen := make(map[string]struct{}, len(group))
		for _, v := range group {
			key := strings.ToLower(v)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			counts[key]++
			if _, ok := firstSeen[key]; !ok {
				firstSeen[key] = v
			}
		}
	}
	intersection := make([]string, 0, len(counts))
	for key, count := range counts {
		if count == len(groups) {
			intersection = append(intersection, firstSeen[key])
		}
	}
	return intersection
}

// asGroup wraps v in a single-element token group if ok, or returns nil (a vacuously-satisfied
// group, per uniqueNameMatch) otherwise.
func asGroup(v string, ok bool) []string {
	if !ok {
		return nil
	}
	return []string{v}
}

// nameSeparators are the characters commonly used to delimit tokens in a NodeClass name (e.g. "linux-amd64-nodeclass").
func nameSeparators(r rune) bool {
	return r == '-' || r == '_' || r == '.'
}

// namedTokens pairs a NodeClass name and labels with its name segments (split on
// nameSeparators), so the split only needs to happen once per name even though uniqueNameMatch
// and attemptLabelMatch may each be called against the same NodeClass list.
type namedTokens struct {
	name   string
	parts  []string
	labels map[string]string
}

// tokenizeNames splits each NodeClass name in ncList into segments on nameSeparators.
func tokenizeNames(ncList []unstructured.Unstructured) []namedTokens {
	names := make([]namedTokens, len(ncList))
	for i, nc := range ncList {
		name := nc.GetName()
		names[i] = namedTokens{name: name, parts: strings.FieldsFunc(name, nameSeparators), labels: nc.GetLabels()}
	}
	return names
}

// uniqueNameMatch returns the name of the single NodeClass whose name contains, for every
// non-empty group in tokenGroups, a segment matching (case-insensitively) at least one token
// in that group (segments are produced by splitting on nameSeparators, so a NodeClass named
// e.g. "team-amd64x-shared" doesn't incorrectly match the token "amd64"). ok is false if zero or
// more than one NodeClass matches; ambiguous distinguishes the "more than one" case (a real
// conflict) from "zero" (no opinion) -- see resolveUnique.
func uniqueNameMatch(names []namedTokens, tokenGroups [][]string) (name string, ok bool, ambiguous bool) {
	var matched []string
	for _, n := range names {
		if nameMatchesAllGroups(n.parts, tokenGroups) {
			matched = append(matched, n.name)
		}
	}
	return resolveUnique(matched)
}

// resolveUnique resolves matched to a single value: ok is true if it holds exactly one entry;
// ambiguous is true if it holds more than one (a real conflict, as opposed to zero, which is "no
// opinion").
func resolveUnique(matched []string) (name string, ok bool, ambiguous bool) {
	switch len(matched) {
	case 0:
		return "", false, false
	case 1:
		return matched[0], true, false
	default:
		return "", false, true
	}
}

// nameMatchesAllGroups reports whether parts has, for every non-empty group in tokenGroups, at
// least one segment matching (case-insensitively) one of that group's tokens.
func nameMatchesAllGroups(parts []string, tokenGroups [][]string) bool {
	for _, tokens := range tokenGroups {
		if len(tokens) == 0 {
			continue
		}
		if !nameMatchesAnyToken(parts, tokens) {
			return false
		}
	}
	return true
}

// nameMatchesAnyToken reports whether any of parts case-insensitively equals any of tokens.
func nameMatchesAnyToken(parts, tokens []string) bool {
	for _, part := range parts {
		for _, token := range tokens {
			if strings.EqualFold(part, token) {
				return true
			}
		}
	}
	return false
}
