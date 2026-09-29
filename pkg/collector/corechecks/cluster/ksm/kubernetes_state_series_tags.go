// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"slices"
	"strings"
	"sync"

	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
)

// Precomputed series tags (precompute_series_tags).
//
// hostnameAndTags rebuilds the tags of every series at every run, although
// almost all of them only depend on the object the series comes from, and the
// store only regenerates an object's series when that object changes. With the
// option on, the stores call computeSeriesTags when an object is added or
// updated, and a run only copies the result and adds what depends on other
// objects:
//
//   - label joins that read the object's own families and match on its identity
//     (e.g. kube_pod_labels on namespace + pod for pod series) are resolved in
//     computeSeriesTags, from the object's families;
//   - any other join that matches the object's series (e.g. node joins on the
//     node label of pod container series) is recorded, and resolved per run;
//   - namespace tags come from the tagger, once per namespace per run.
//
// Series the precomputation cannot reproduce exactly keep the per-run path
// (hostnameAndTags with every join): objects whose series disagree on
// hostname or namespace, kinds whose cross-object joins bring owner or Argo
// Rollout labels, and stores of unknown kinds (unstructured custom resources).

// seriesTagsPlan classifies the label joins per resource kind, and records
// which cross-object joins actually match series of each kind.
type seriesTagsPlan struct {
	joins        map[string]*joinsConfig
	forAggregate map[string]struct{} // joins the aggregators can match

	mu      sync.RWMutex
	cross   map[string]map[string]struct{} // kind -> joins seen matching its series, resolved per run
	legacy  map[string]struct{}            // kinds that must keep the per-run path
	version int                            // bumped when cross or legacy change
}

// newSeriesTagsPlan classifies the joins. aggregatorLabels are the labels the
// aggregators keep; nil means unknown, and keeps every join for them.
func newSeriesTagsPlan(joins map[string]*joinsConfig, aggregatorLabels map[string]struct{}) *seriesTagsPlan {
	p := &seriesTagsPlan{
		joins:        joins,
		forAggregate: map[string]struct{}{},
		cross:        map[string]map[string]struct{}{},
		legacy:       map[string]struct{}{},
	}
	for name, config := range joins {
		matchable := aggregatorLabels == nil // unknown aggregators: keep every join
		if !matchable {
			matchable = true
			for _, l := range config.labelsToMatch {
				if _, found := aggregatorLabels[l]; !found {
					matchable = false
					break
				}
			}
		}
		if matchable {
			p.forAggregate[name] = struct{}{}
		}
	}
	return p
}

// intraJoins returns the joins resolved when an object of this kind changes:
// those reading the kind's own families and matching on its identity labels
// (e.g. kube_pod_labels on namespace + pod), which can only match the series of
// the object they come from.
func (p *seriesTagsPlan) intraJoins(kind string) map[string]*joinsConfig {
	identity := getLabelToMatchForKind(kind)
	intra := map[string]*joinsConfig{}
	for name, config := range p.joins {
		if strings.HasPrefix(name, "kube_"+kind+"_") && len(config.labelsToMatch) > 0 && slices.Equal(config.labelsToMatch, identity) {
			intra[name] = config
		}
	}
	return intra
}

// kindOfStore maps a store's resource type ("*v1.Pod") to the kind used in
// KSM family names and label joins ("pod").
func kindOfStore(resourceType string) string {
	t := strings.TrimPrefix(resourceType, "*")
	if i := strings.LastIndexByte(t, '.'); i >= 0 {
		t = t[i+1:]
	}
	return strings.ToLower(t)
}

// crossJoinBringsSpecialLabels reports whether a join can bring labels that
// the per-run path interprets (owner, Argo Rollout, namespace, host): such a
// join cannot be applied on top of precomputed tags.
func crossJoinBringsSpecialLabels(config *joinsConfig) bool {
	if config.getAllLabels {
		return true
	}
	for key, tag := range config.labelsToGet {
		for _, special := range []string{createdByKindKey, createdByNameKey, ownerKindKey, ownerNameKey, argoRolloutLabelName, namespaceKey} {
			if key == special || tag == special {
				return true
			}
		}
		if tag == "host" || tag == "node" {
			return true
		}
	}
	return false
}

// noteCrossJoins records the joins that are not resolved with the object and
// that match these series' labels.
func (p *seriesTagsPlan) noteCrossJoins(kind string, seen map[string]struct{}) {
	if len(seen) == 0 {
		return
	}
	p.mu.RLock()
	known := true
	for name := range seen {
		if _, found := p.cross[kind][name]; !found {
			known = false
			break
		}
	}
	p.mu.RUnlock()
	if known {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cross[kind] == nil {
		p.cross[kind] = map[string]struct{}{}
	}
	for name := range seen {
		if _, found := p.cross[kind][name]; !found {
			p.cross[kind][name] = struct{}{}
			p.version++
		}
		if crossJoinBringsSpecialLabels(p.joins[name]) {
			p.legacy[kind] = struct{}{}
		}
	}
}

// runJoins returns the joins a run must resolve: those the aggregators can
// match and the cross-object joins seen so far, and the plan's version they
// were read at.
func (p *seriesTagsPlan) runJoins() (map[string]*joinsConfig, int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	joins := map[string]*joinsConfig{}
	for name := range p.forAggregate {
		joins[name] = p.joins[name]
	}
	for _, names := range p.cross {
		for name := range names {
			joins[name] = p.joins[name]
		}
	}
	return joins, p.version
}

// crossJoins returns the cross-object joins seen for a kind, and whether that
// kind must keep the per-run path. It also keeps the per-run path when the
// plan changed since the run took its joins (version), because the run's
// joiner may lack a join an object was just found to need.
func (p *seriesTagsPlan) crossJoins(kind string, version int) (cross map[string]struct{}, legacy bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, legacy = p.legacy[kind]
	return p.cross[kind], legacy || p.version != version
}

// seriesTagsFuncs returns, for the builder, the store hook of each resource
// type, or nil for kinds the precomputation does not handle. The hooks keep
// their plan: the stores they feed are built for it, and run in informer
// goroutines while a rebuild may replace k.seriesTags.
func (k *KSMCheck) seriesTagsFuncs(plan *seriesTagsPlan) func(resourceType string) ksmstore.SeriesTagsFunc {
	return func(resourceType string) ksmstore.SeriesTagsFunc {
		kind := kindOfStore(resourceType)
		if kind == "" || kind == "unstructured" {
			return nil
		}
		intra := plan.intraJoins(kind)
		return func(families []ksmstore.DDMetricsFam) *ksmstore.ObjectTags {
			return k.computeSeriesTags(plan, kind, intra, families)
		}
	}
}

// emitted reports whether a family is sent as-is or through a transformer,
// i.e. whether its series need tags.
func (k *KSMCheck) emitted(family string) bool {
	if _, found := k.metricTransformers[family]; found {
		return true
	}
	_, found := k.metricNamesMapper[family]
	return found
}

// computeSeriesTags is the store hook: it computes the tags of the series of
// one object. It returns nil when the series cannot share one hostname and
// namespace, so that they keep the per-run path.
func (k *KSMCheck) computeSeriesTags(plan *seriesTagsPlan, kind string, intra map[string]*joinsConfig, families []ksmstore.DDMetricsFam) *ksmstore.ObjectTags {
	// Joins resolved with the object, from its own families.
	var joiner *labelJoiner
	if len(intra) > 0 {
		joiner = newLabelJoiner(intra)
		for _, f := range families {
			if join, found := joiner.metricsToJoin[f.Name]; found {
				for _, m := range f.ListMetrics {
					if k.metricFilter(m) {
						joiner.insertMetric(m, join.config, join.tree)
					}
				}
			}
		}
	}

	type seriesRef struct {
		family, metric int
		tags           []string
	}
	var (
		object *ksmstore.ObjectTags
		common []string
		series []seriesRef
		cross  map[string]struct{}
	)
	for fi := range families {
		name := families[fi].Name
		if !k.emitted(name) {
			continue
		}
		override := labelsMapperOverride(name)
		for mi := range families[fi].ListMetrics {
			labels := families[fi].ListMetrics[mi].Labels
			var labelsToAdd []label
			if joiner != nil {
				labelsToAdd = joiner.getLabelsToAdd(labels)
			}
			hostname, tags, namespace := k.staticHostnameAndTags(labels, labelsToAdd, override)

			for joinName, config := range plan.joins {
				if _, isIntra := intra[joinName]; isIntra {
					continue
				}
				if matchesJoin(labels, config) {
					if cross == nil {
						cross = map[string]struct{}{}
					}
					cross[joinName] = struct{}{}
				}
			}

			if object == nil {
				object = &ksmstore.ObjectTags{Hostname: hostname, Namespace: namespace}
				common = slices.Clone(tags)
			} else {
				if hostname != object.Hostname || namespace != object.Namespace {
					plan.noteCrossJoins(kind, cross)
					return nil
				}
				common = intersectTags(common, tags)
			}
			series = append(series, seriesRef{family: fi, metric: mi, tags: tags})
		}
	}
	plan.noteCrossJoins(kind, cross)
	if object == nil {
		return nil
	}

	for _, s := range series {
		families[s.family].ListMetrics[s.metric].ExtraTags = subtractTags(s.tags, common)
	}
	object.Tags = slices.Clip(common)
	return object
}

// intersectTags returns the tags of a that b also has, counting duplicates: a
// tag present twice in both is kept twice. It reuses a's storage.
func intersectTags(a, b []string) []string {
	left := make(map[string]int, len(b))
	for _, t := range b {
		left[t]++
	}
	return slices.DeleteFunc(a, func(t string) bool {
		if left[t] == 0 {
			return true
		}
		left[t]--
		return false
	})
}

// subtractTags returns the tags of tags that are not in common, counting
// duplicates, so that common + the result is tags again.
func subtractTags(tags, common []string) []string {
	left := make(map[string]int, len(common))
	for _, t := range common {
		left[t]++
	}
	var extra []string
	for _, t := range tags {
		if left[t] > 0 {
			left[t]--
			continue
		}
		extra = append(extra, t)
	}
	return extra
}

// matchesJoin reports whether a join can match series with these labels.
func matchesJoin(labels map[string]string, config *joinsConfig) bool {
	for _, l := range config.labelsToMatch {
		if _, found := labels[l]; !found {
			return false
		}
	}
	return true
}

// seriesRun holds what a run with precomputed series tags reuses across series.
type seriesRun struct {
	namespaceTags map[string][]string
	fullJoiner    *labelJoiner // every join, built on first need
	planVersion   int          // version of the plan the run's joins were read at
}

// precomputedHostnameAndTags returns the hostname and tags of a series from its
// precomputed tags, the cross-object joins and the namespace tags. It reports
// false when the series must use the per-run path.
func (k *KSMCheck) precomputedHostnameAndTags(family *ksmstore.DDMetricsFam, m ksmstore.DDMetric, runJoiner *labelJoiner, lMapperOverride map[string]string) (string, []string, bool) {
	object := family.Object
	if object == nil || k.seriesTags == nil {
		return "", nil, false
	}
	cross, legacy := k.seriesTags.crossJoins(kindOfStore(family.Type), k.run.planVersion)
	if legacy {
		return "", nil, false
	}

	namespaceTags, cached := k.run.namespaceTags[object.Namespace]
	if !cached {
		namespaceTags = k.namespaceTags(object.Namespace)
		k.run.namespaceTags[object.Namespace] = namespaceTags
	}

	var labelsToAdd []label
	if len(cross) > 0 {
		labelsToAdd = runJoiner.getLabelsToAddFrom(m.Labels, cross)
	}

	tags := make([]string, 0, len(object.Tags)+len(m.ExtraTags)+len(labelsToAdd)+len(namespaceTags))
	tags = append(tags, object.Tags...)
	tags = append(tags, m.ExtraTags...)
	for _, l := range labelsToAdd {
		tag, _ := k.buildTag(l.key, l.value, lMapperOverride)
		tags = append(tags, tag)
	}
	tags = append(tags, namespaceTags...)
	return object.Hostname, tags, true
}

// seriesHostnameAndTags returns the hostname and tags of a series, from
// precomputed tags when possible.
func (k *KSMCheck) seriesHostnameAndTags(family *ksmstore.DDMetricsFam, m ksmstore.DDMetric, labelJoiner *labelJoiner, lMapperOverride map[string]string) (string, []string) {
	if k.run == nil {
		return k.hostnameAndTags(m.Labels, labelJoiner, lMapperOverride)
	}
	if hostname, tags, ok := k.precomputedHostnameAndTags(family, m, labelJoiner, lMapperOverride); ok {
		return hostname, tags
	}
	return k.hostnameAndTags(m.Labels, k.fullJoiner(), lMapperOverride)
}

// fullJoiner returns a joiner with every label join, built once per run, for
// the series that keep the per-run path.
func (k *KSMCheck) fullJoiner() *labelJoiner {
	if k.run.fullJoiner == nil {
		k.run.fullJoiner = k.buildJoiner(k.instance.labelJoins)
	}
	return k.run.fullJoiner
}

// buildJoiner builds a label joiner for the given joins from every store.
func (k *KSMCheck) buildJoiner(joins map[string]*joinsConfig) *labelJoiner {
	joiner := newLabelJoiner(joins)
	if len(joins) == 0 {
		return joiner
	}
	familyFilter := func(f ksmstore.DDMetricsFam) bool {
		_, found := joins[f.Name]
		return found
	}
	for _, stores := range k.allStores {
		for _, store := range stores {
			if ms, ok := store.(*ksmstore.MetricsStore); ok {
				joiner.insertFamilies(ms.Push(familyFilter, k.metricFilter))
			}
		}
	}
	return joiner
}
