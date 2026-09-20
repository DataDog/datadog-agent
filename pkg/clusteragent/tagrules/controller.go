// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagrules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	informers "k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	workerCount = 2

	// leaderCheckRetryInterval is how long a queued item is deferred while the
	// process is not the elected leader.
	leaderCheckRetryInterval = time.Second

	taskRulePrefix = "rule/"
	taskPodPrefix  = "pod/"
	taskNodePrefix = "node/"

	defaultStatusInterval = 15 * time.Second
	defaultResyncInterval = 15 * time.Second
)

var (
	tagRuleGVR = schema.GroupVersionResource{Group: GroupName, Version: GroupVersion, Resource: TagRuleResource}
	podGVR     = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	nodeGVR    = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
)

// Controller reconciles TagRule CRs onto pod and node annotations. It runs on
// the leader-elected Cluster Agent; the binary wiring handles leader election.
type Controller struct {
	logger    *slog.Logger
	dynClient dynamic.Interface
	now       func() time.Time

	statusInterval time.Duration
	resyncInterval time.Duration

	// leaderCheck gates all reconciles: while it returns false, queued items
	// are deferred and periodic work is skipped.
	leaderCheck func() bool

	queue workqueue.TypedRateLimitingInterface[string]

	evaluator *Evaluator
	flip      *flipHold

	informersMu sync.Mutex
	stopCh      <-chan struct{}
	informers   map[schema.GroupVersionResource]informers.GenericInformer

	rulesMu sync.RWMutex
	rules   map[string]*TagRule // by CR name, last synced spec

	ruleErrsMu sync.Mutex
	ruleErrs   map[string]ruleErr // validation/compile errors by rule name

	droppedMu sync.Mutex
	dropped   map[string]int // evaluation drops since last status report
}

type ruleErr struct {
	generation int64
	err        error
}

// Option configures a Controller.
type Option func(*Controller)

// WithLogger sets the controller logger.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Controller) { c.logger = logger }
}

// WithNow overrides the clock, for tests.
func WithNow(now func() time.Time) Option {
	return func(c *Controller) { c.now = now }
}

// WithIntervals sets the status and resync intervals, for tests.
func WithIntervals(status, resync time.Duration) Option {
	return func(c *Controller) { c.statusInterval, c.resyncInterval = status, resync }
}

// WithLeaderCheck gates the controller on leadership: while the check returns
// false, no reconcile runs; queued items are retried when it returns true.
func WithLeaderCheck(check func() bool) Option {
	return func(c *Controller) { c.leaderCheck = check }
}

// NewController builds the controller. Call Run to start it.
func NewController(dynClient dynamic.Interface, opts ...Option) (*Controller, error) {
	evaluator, err := NewEvaluator()
	if err != nil {
		return nil, err
	}
	c := &Controller{
		logger:         slog.Default(),
		dynClient:      dynClient,
		now:            time.Now,
		statusInterval: defaultStatusInterval,
		resyncInterval: defaultResyncInterval,
		queue: workqueue.NewTypedRateLimitingQueue(
			workqueue.DefaultTypedControllerRateLimiter[string](),
		),
		evaluator: evaluator,
		flip:      newFlipHold(),
		informers: map[schema.GroupVersionResource]informers.GenericInformer{},
		rules:     map[string]*TagRule{},
		ruleErrs:  map[string]ruleErr{},
		dropped:   map[string]int{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Run starts the informers, workers and periodic loops until the context is
// cancelled.
func (c *Controller) Run(ctx context.Context) error {
	c.stopCh = ctx.Done()

	tagRuleInformer := c.ensureInformer(tagRuleGVR, c.onTagRuleChange)
	podInformer := c.ensureInformer(podGVR, func(_ context.Context, obj *unstructured.Unstructured) {
		c.queue.Add(taskPodPrefix + obj.GetNamespace() + "/" + obj.GetName())
	})
	nodeInformer := c.ensureInformer(nodeGVR, func(_ context.Context, obj *unstructured.Unstructured) {
		c.queue.Add(taskNodePrefix + obj.GetName())
	})

	if !cache.WaitForCacheSync(ctx.Done(),
		tagRuleInformer.Informer().HasSynced,
		podInformer.Informer().HasSynced,
		nodeInformer.Informer().HasSynced,
	) {
		return fmt.Errorf("tag-rule controller: informer cache sync interrupted")
	}

	// Initial rule sync: enqueue every existing rule.
	for _, rule := range c.listTagRules() {
		c.queue.Add(taskRulePrefix + rule.GetName())
	}

	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c.processNext(ctx) {
			}
		}()
	}

	go c.periodic(ctx, c.resyncInterval, c.resyncAll)
	go c.periodic(ctx, c.statusInterval, c.syncAllStatus)

	<-ctx.Done()
	c.queue.ShutDown()
	wg.Wait()
	return nil
}

func (c *Controller) periodic(ctx context.Context, interval time.Duration, fn func(context.Context)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fn(ctx)
		}
	}
}

func (c *Controller) isLeader() bool {
	return c.leaderCheck == nil || c.leaderCheck()
}

// processNext handles one queue item; false when the queue is shut down.
func (c *Controller) processNext(ctx context.Context) bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(item)

	if !c.isLeader() {
		// Defer the item until leadership is held; workers keep looping.
		c.queue.AddAfter(item, leaderCheckRetryInterval)
		return true
	}

	if err := c.handle(ctx, item); err != nil {
		c.logger.Warn("tag-rule controller: retrying task", "task", item, "error", err)
		c.queue.AddRateLimited(item)
		return true
	}
	c.queue.Forget(item)
	return true
}

func (c *Controller) handle(ctx context.Context, item string) error {
	switch {
	case strings.HasPrefix(item, taskRulePrefix):
		return c.reconcileRule(ctx, item[len(taskRulePrefix):])
	case strings.HasPrefix(item, taskPodPrefix):
		rest := item[len(taskPodPrefix):]
		namespace, name, _ := strings.Cut(rest, "/")
		return c.reconcileEntity(ctx, EntityPod, namespace, name)
	case strings.HasPrefix(item, taskNodePrefix):
		return c.reconcileEntity(ctx, EntityNode, "", item[len(taskNodePrefix):])
	default:
		return fmt.Errorf("unknown task %q", item)
	}
}

// onTagRuleChange enqueues a rule sync on any add, update or delete.
func (c *Controller) onTagRuleChange(_ context.Context, obj *unstructured.Unstructured) {
	c.queue.Add(taskRulePrefix + obj.GetName())
}

// ensureInformer creates (once) and starts a dynamic informer for the
// resource. The handler is invoked on add, update and delete events.
func (c *Controller) ensureInformer(gvr schema.GroupVersionResource, handler func(context.Context, *unstructured.Unstructured)) informers.GenericInformer {
	c.informersMu.Lock()
	defer c.informersMu.Unlock()
	if informer, ok := c.informers[gvr]; ok {
		return informer
	}
	informer := dynamicinformer.NewFilteredDynamicInformer(c.dynClient, gvr, metav1.NamespaceAll, 0, cache.Indexers{}, nil)
	c.informers[gvr] = informer
	go informer.Informer().Run(c.stopCh)
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-c.stopCh
			cancel()
		}()
		cache.WaitForNamedCacheSync(gvr.String(), ctx.Done(), informer.Informer().HasSynced)
	}()
	if handler != nil {
		_, _ = informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				if u, ok := obj.(*unstructured.Unstructured); ok {
					handler(context.Background(), u)
				}
			},
			UpdateFunc: func(_, newObj any) {
				if u, ok := newObj.(*unstructured.Unstructured); ok {
					handler(context.Background(), u)
				}
			},
			DeleteFunc: func(obj any) {
				if u, ok := obj.(*unstructured.Unstructured); ok {
					handler(context.Background(), u)
				}
			},
		})
	}
	return informer
}

func (c *Controller) lister(gvr schema.GroupVersionResource) cache.GenericLister {
	return c.ensureInformer(gvr, nil).Lister()
}

func (c *Controller) listTagRules() []*unstructured.Unstructured {
	objects, err := c.lister(tagRuleGVR).List(labels.Everything())
	if err != nil {
		c.logger.Error("tag-rule controller: listing TagRules", "error", err)
		return nil
	}
	rules := make([]*unstructured.Unstructured, 0, len(objects))
	for _, obj := range objects {
		if u, ok := obj.(*unstructured.Unstructured); ok {
			rules = append(rules, u)
		}
	}
	return rules
}

// reconcileRule syncs one TagRule: cache state, ensure finalizer and source
// informers, handle deletion cleanup.
func (c *Controller) reconcileRule(ctx context.Context, name string) error {
	raw, err := c.lister(tagRuleGVR).Get(name)
	if err != nil {
		// The CR is gone from the store: any cleanup already ran before the
		// finalizer was removed. Drop the cache entry.
		c.removeCachedRule(name)
		return nil
	}
	unstructuredRule, ok := raw.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("tag-rule %s: unexpected object type %T", name, raw)
	}
	rule, err := tagRuleFromUnstructured(unstructuredRule)
	if err != nil {
		return err
	}

	if rule.DeletionTimestamp != nil {
		return c.deleteRule(ctx, rule)
	}

	if err := c.validateAndCacheRule(ctx, rule); err != nil {
		c.setRuleErr(rule, err)
		c.logger.Warn("tag-rule controller: invalid rule", "rule", name, "error", err)
		return nil // Configuration error: permanent until the CR is edited.
	}
	c.clearRuleErr(rule.Name)

	if err := c.ensureFinalizer(ctx, rule); err != nil {
		return err
	}

	if err := c.ensureSourceInformers(rule); err != nil {
		c.setRuleErr(rule, err)
		return nil
	}

	c.enqueueRuleEntities(rule)
	return nil
}

// validateAndCacheRule validates the rule (webhook invariants) and enforces
// tag-key uniqueness and immutability across cached rules, then caches it.
// Tag-key conflicts resolve deterministically: the lexicographically-smallest
// CR name owns the key; any other claimant is evicted and flagged.
func (c *Controller) validateAndCacheRule(ctx context.Context, rule *TagRule) error {
	if err := rule.validateRule(); err != nil {
		return err
	}

	c.rulesMu.Lock()
	defer c.rulesMu.Unlock()

	if cached, ok := c.rules[rule.Name]; ok && cached.Spec.Tag != rule.Spec.Tag {
		return ruleValidationErrorf("spec.tag is immutable (was %q)", cached.Spec.Tag)
	}

	var evict []*TagRule
	for _, name := range slices.Sorted(maps.Keys(c.rules)) {
		other := c.rules[name]
		if other.Name == rule.Name || other.Spec.Tag != rule.Spec.Tag {
			continue
		}
		if rule.Name > other.Name {
			return ruleValidationErrorf("tag key %q is already owned by rule %s", rule.Spec.Tag, other.Name)
		}
		evict = append(evict, other)
	}
	for _, other := range evict {
		c.setRuleErrEvicted(other, ruleValidationErrorf("tag key %q is also claimed by rule %s", other.Spec.Tag, rule.Name))
		delete(c.rules, other.Name)
		// Flag the loser's status (it is no longer cached, so the status loop
		// will not pick it up) and reconcile its entities to strip its key.
		if err := c.syncRuleStatus(ctx, other.Name); err != nil {
			c.logger.Warn("tag-rule controller: loser status sync failed", "rule", other.Name, "error", err)
		}
		c.enqueueRuleEntities(other)
	}

	c.rules[rule.Name] = rule
	return nil
}

// setRuleErrEvicted records a rule error while already holding rulesMu.
func (c *Controller) setRuleErrEvicted(rule *TagRule, err error) {
	c.ruleErrsMu.Lock()
	defer c.ruleErrsMu.Unlock()
	c.ruleErrs[rule.Name] = ruleErr{generation: rule.Generation, err: err}
}

func (c *Controller) removeCachedRule(name string) *TagRule {
	c.rulesMu.Lock()
	defer c.rulesMu.Unlock()
	rule := c.rules[name]
	delete(c.rules, name)
	return rule
}

// deleteRule sweeps the rule's entities with the rule removed from the cache
// (stripping owned keys and updating ledgers), then releases the finalizer.
func (c *Controller) deleteRule(ctx context.Context, rule *TagRule) error {
	if c.removeCachedRule(rule.Name) != nil {
		c.enqueueRuleEntities(rule)
	}
	c.flip.dropTagKey(rule.Spec.Tag)
	c.clearRuleErr(rule.Name)

	if slices.Contains(rule.Finalizers, CleanupFinalizer) {
		finalizers := slices.DeleteFunc(slices.Clone(rule.Finalizers), func(f string) bool { return f == CleanupFinalizer })
		patch, err := json.Marshal(map[string]any{"metadata": map[string]any{"finalizers": finalizers}})
		if err != nil {
			return err
		}
		_, err = c.dynClient.Resource(tagRuleGVR).Patch(ctx, rule.Name, types.MergePatchType, patch, metav1.PatchOptions{})
		if err != nil {
			return fmt.Errorf("removing finalizer from rule %s: %w", rule.Name, err)
		}
	}
	return nil
}

// ensureFinalizer adds the cleanup finalizer before any write happens.
func (c *Controller) ensureFinalizer(ctx context.Context, rule *TagRule) error {
	if slices.Contains(rule.Finalizers, CleanupFinalizer) {
		return nil
	}
	finalizers := append(slices.Clone(rule.Finalizers), CleanupFinalizer)
	patch, err := json.Marshal(map[string]any{"metadata": map[string]any{"finalizers": finalizers}})
	if err != nil {
		return err
	}
	_, err = c.dynClient.Resource(tagRuleGVR).Patch(ctx, rule.Name, types.MergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("adding finalizer to rule %s: %w", rule.Name, err)
	}
	return nil
}

// ensureSourceInformers starts the informer for the rule's source resource, if
// the source is a distinct object.
func (c *Controller) ensureSourceInformers(rule *TagRule) error {
	if sourceIsSelf(rule) {
		return nil
	}
	gvr, err := resolveSourceGVR(rule.Spec.Source)
	if err != nil {
		return err
	}
	c.ensureInformer(gvr, c.sourceChangeHandler(gvr))
	return nil
}

// sourceChangeHandler re-enqueues the rules referencing the source resource.
func (c *Controller) sourceChangeHandler(gvr schema.GroupVersionResource) func(context.Context, *unstructured.Unstructured) {
	return func(_ context.Context, _ *unstructured.Unstructured) {
		c.rulesMu.RLock()
		defer c.rulesMu.RUnlock()
		for name, rule := range c.rules {
			if rule.Spec.Source == nil || sourceIsSelf(rule) {
				continue
			}
			if sourceGVR, err := resolveSourceGVR(rule.Spec.Source); err == nil && sourceGVR == gvr {
				c.queue.Add(taskRulePrefix + name)
			}
		}
	}
}

// enqueueRuleEntities enqueues every entity matching the rule's selector.
func (c *Controller) enqueueRuleEntities(rule *TagRule) {
	for _, entity := range c.listEntities(rule.Spec.Entity) {
		if matched, err := rule.Spec.Selector.matches(entity, rule.Spec.Entity); err != nil {
			c.logger.Warn("tag-rule controller: selector match failed", "rule", rule.Name, "error", err)
			return
		} else if !matched {
			continue
		}
		c.enqueueEntity(entity, rule.Spec.Entity)
	}
}

func (c *Controller) enqueueEntity(entity *unstructured.Unstructured, kind EntityKind) {
	switch kind {
	case EntityPod:
		c.queue.Add(taskPodPrefix + entity.GetNamespace() + "/" + entity.GetName())
	case EntityNode:
		c.queue.Add(taskNodePrefix + entity.GetName())
	}
}

func (c *Controller) listEntities(kind EntityKind) []*unstructured.Unstructured {
	var gvr schema.GroupVersionResource
	switch kind {
	case EntityPod:
		gvr = podGVR
	case EntityNode:
		gvr = nodeGVR
	default:
		return nil
	}
	objects, err := c.lister(gvr).List(labels.Everything())
	if err != nil {
		c.logger.Error("tag-rule controller: listing entities", "kind", kind, "error", err)
		return nil
	}
	entities := make([]*unstructured.Unstructured, 0, len(objects))
	for _, obj := range objects {
		if u, ok := obj.(*unstructured.Unstructured); ok {
			entities = append(entities, u)
		}
	}
	return entities
}

// resyncAll re-enqueues every rule from the informer store (not the cache:
// a process that started as follower has an empty cache), handling drift,
// flip-holds whose interval has elapsed, and leadership transitions.
func (c *Controller) resyncAll(ctx context.Context) {
	if !c.isLeader() {
		return
	}
	for _, rule := range c.listTagRules() {
		if rule.GetDeletionTimestamp() == nil {
			c.queue.Add(taskRulePrefix + rule.GetName())
		}
	}
}

// reconcileEntity recomputes all owned tag keys for one entity from the
// current rules and cluster state, and writes at most one annotation patch.
func (c *Controller) reconcileEntity(ctx context.Context, kind EntityKind, namespace, name string) error {
	entity, err := c.getEntity(kind, namespace, name)
	if err != nil || entity == nil {
		return err
	}

	entityID := entityID(kind, namespace, name)
	owned := map[string]string{}

	for _, rule := range c.matchingRules(entity, kind) {
		value, dropped, err := c.evaluateRule(rule, entity)
		if errors.Is(err, errSourceNotSynced) {
			// Keep whatever the entity currently carries until the source
			// informer syncs; the resync re-evaluates.
			if current, ok, currentErr := ownedTagValue(entity, kind, rule.Spec.Tag); currentErr == nil && ok {
				owned[rule.Spec.Tag] = current
			}
			continue
		}
		if err != nil {
			// Evaluation errors are per entity write, not rule status: they are
			// usually missing source objects.
			c.logger.Debug("tag-rule controller: evaluation failed", "rule", rule.Name, "entity", entityID, "error", err)
		}
		if dropped {
			c.countDrop(rule.Name)
		}

		// The flip hold governs value changes and disappearances alike, so a
		// tag never flaps to absent faster than the hold allows.
		written := ""
		if !dropped {
			written = value
		}
		held, _ := c.flip.resolve(entityID, rule.Spec.Tag, written, time.Duration(rule.Spec.flipHold())*time.Second, c.now())
		if held == "" {
			continue
		}
		owned[rule.Spec.Tag] = held
	}

	var patch annotationPatch
	switch kind {
	case EntityPod:
		patch, err = computePodAnnotations(entity, owned)
	case EntityNode:
		patch, err = computeNodeAnnotations(entity, owned)
	}
	if err != nil {
		return err
	}
	if len(patch) == 0 {
		return nil
	}
	return c.patchAnnotations(ctx, kind, namespace, name, patch)
}

type genericGetter interface {
	Get(name string) (runtime.Object, error)
}

func (c *Controller) getEntity(kind EntityKind, namespace, name string) (*unstructured.Unstructured, error) {
	var gvr schema.GroupVersionResource
	switch kind {
	case EntityPod:
		gvr = podGVR
	case EntityNode:
		gvr = nodeGVR
	default:
		return nil, fmt.Errorf("unsupported entity kind %q", kind)
	}
	var getter genericGetter = c.lister(gvr)
	if namespace != "" {
		getter = c.lister(gvr).ByNamespace(namespace)
	}
	obj, err := getter.Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil // Entity deleted: nothing to reconcile.
		}
		return nil, fmt.Errorf("getting %s %s/%s: %w", kind, namespace, name, err)
	}
	entity, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("getting %s %s/%s: unexpected object type %T", kind, namespace, name, obj)
	}
	return entity, nil
}

func (c *Controller) matchingRules(entity *unstructured.Unstructured, kind EntityKind) []*TagRule {
	c.rulesMu.RLock()
	defer c.rulesMu.RUnlock()
	matched := make([]*TagRule, 0, len(c.rules))
	for _, ruleName := range slices.Sorted(maps.Keys(c.rules)) {
		rule := c.rules[ruleName]
		if m, err := ruleMatchesEntity(rule, entity, kind); err == nil && m {
			matched = append(matched, rule)
		}
	}
	return matched
}

// evaluateRule computes the tag value for one entity. dropped=true means the
// tag must not be written (the onError drop policy, or a bool evaluation
// failure). For string-set rules with onError=default, failures return the
// declared fallback value with dropped=false. The returned error is
// informational: the caller logs it; the value/dropped pair is authoritative.
func (c *Controller) evaluateRule(rule *TagRule, entity *unstructured.Unstructured) (value string, dropped bool, err error) {
	sourceMap, err := c.sourceObject(rule, entity)
	if errors.Is(err, errSourceNotSynced) {
		// No decision this pass: neither a value nor a drop.
		return "", false, err
	}
	if err != nil {
		if rule.Spec.Value.Type == ValueStringSet {
			fallback, drop := c.stringSetErrorValue(rule.Spec.Value.StringSet)
			return fallback, drop, err
		}
		return "", true, err
	}

	switch rule.Spec.Value.Type {
	case ValueBool:
		result, err := c.evaluator.EvalBool(rule.Spec.Value.Bool.Expression, normalizeForEval(entity.Object), normalizeForEval(sourceMap))
		if err != nil {
			return "", true, err
		}
		return entityValueString(result), false, nil

	case ValueStringSet:
		ss := rule.Spec.Value.StringSet
		result, err := c.evaluator.EvalString(ss.Expression, normalizeForEval(entity.Object), normalizeForEval(sourceMap))
		if err != nil {
			fallback, drop := c.stringSetErrorValue(ss)
			return fallback, drop, fmt.Errorf("evaluating string-set expression: %w", err)
		}
		if !containsString(ss.Values, result) {
			fallback, drop := c.stringSetErrorValue(ss)
			return fallback, drop, fmt.Errorf("value %q is outside the declared set", result)
		}
		return result, false, nil

	default:
		return "", true, ruleValidationErrorf("unsupported value type %q", rule.Spec.Value.Type)
	}
}

// stringSetErrorValue applies the onError policy: "" with dropped=true drops
// the tag, a default value writes the fallback.
func (c *Controller) stringSetErrorValue(ss *StringSetValue) (string, bool) {
	if ss.OnError == OnErrorDefault {
		return ss.Default, false
	}
	return "", true
}

// errSourceNotSynced marks a source lookup skipped because the informer has
// not synced yet: the rule is neither evaluated nor treated as failed, and
// whatever the entity currently carries is preserved until the informer
// syncs and the resync re-evaluates.
var errSourceNotSynced = errors.New("source informer not synced")

// informerSynced reports whether the informer for the resource exists and has
// synced.
func (c *Controller) informerSynced(gvr schema.GroupVersionResource) bool {
	c.informersMu.Lock()
	informer, ok := c.informers[gvr]
	c.informersMu.Unlock()
	return ok && informer.Informer().HasSynced()
}

// sourceObject resolves and fetches the rule's source object for an entity,
// returning its unstructured map. The entity itself is returned for self
// sources. A missing source object is an evaluation error (the onError path
// decides the outcome).
func (c *Controller) sourceObject(rule *TagRule, entity *unstructured.Unstructured) (map[string]any, error) {
	coords, distinct, err := resolveSource(rule, entity, c.evaluator)
	if err != nil {
		return nil, err
	}
	if !distinct {
		return entity.Object, nil
	}
	if !c.informerSynced(coords.gvr) {
		return nil, fmt.Errorf("%w: %s", errSourceNotSynced, coords.gvr)
	}

	lister := c.lister(coords.gvr)
	var getter genericGetter = lister
	if coords.namespace != "" {
		getter = lister.ByNamespace(coords.namespace)
	}
	sourceObj, err := getter.Get(coords.name)
	if err != nil {
		return nil, fmt.Errorf("getting source %s %s/%s: %w", coords.gvr, coords.namespace, coords.name, err)
	}
	source, ok := sourceObj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("getting source %s %s/%s: unexpected object type %T", coords.gvr, coords.namespace, coords.name, sourceObj)
	}
	return source.Object, nil
}

func (c *Controller) patchAnnotations(ctx context.Context, kind EntityKind, namespace, name string, patch annotationPatch) error {
	annotations := make(map[string]any, len(patch))
	for key, value := range patch {
		if value == nil {
			annotations[key] = nil
		} else {
			annotations[key] = *value
		}
	}
	data, err := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": annotations}})
	if err != nil {
		return err
	}
	client := c.dynClient
	switch kind {
	case EntityPod:
		_, err = client.Resource(podGVR).Namespace(namespace).Patch(ctx, name, types.MergePatchType, data, metav1.PatchOptions{})
	case EntityNode:
		_, err = client.Resource(nodeGVR).Patch(ctx, name, types.MergePatchType, data, metav1.PatchOptions{})
	default:
		return fmt.Errorf("unsupported entity kind %q", kind)
	}
	if err != nil {
		return fmt.Errorf("patching %s %s/%s: %w", kind, namespace, name, err)
	}
	return nil
}

func (c *Controller) setRuleErr(rule *TagRule, err error) {
	c.ruleErrsMu.Lock()
	defer c.ruleErrsMu.Unlock()
	c.ruleErrs[rule.Name] = ruleErr{generation: rule.Generation, err: err}
}

func (c *Controller) clearRuleErr(name string) {
	c.ruleErrsMu.Lock()
	defer c.ruleErrsMu.Unlock()
	delete(c.ruleErrs, name)
}

func (c *Controller) ruleError(name string, generation int64) error {
	c.ruleErrsMu.Lock()
	defer c.ruleErrsMu.Unlock()
	if e, ok := c.ruleErrs[name]; ok && e.generation == generation {
		return e.err
	}
	return nil
}

func (c *Controller) countDrop(ruleName string) {
	c.droppedMu.Lock()
	defer c.droppedMu.Unlock()
	c.dropped[ruleName]++
}

func (c *Controller) takeDrops(ruleName string) int {
	c.droppedMu.Lock()
	defer c.droppedMu.Unlock()
	count := c.dropped[ruleName]
	delete(c.dropped, ruleName)
	return count
}

// syncAllStatus recomputes and patches the status of every cached rule.
func (c *Controller) syncAllStatus(ctx context.Context) {
	if !c.isLeader() {
		return
	}
	// Walk the informer store, not the cache: invalid and evicted rules are
	// not cached, but their status must still report why they are inactive.
	for _, rule := range c.listTagRules() {
		if rule.GetDeletionTimestamp() != nil {
			continue
		}
		if err := c.syncRuleStatus(ctx, rule.GetName()); err != nil {
			c.logger.Warn("tag-rule controller: status sync failed", "rule", rule.GetName(), "error", err)
		}
	}
}

func (c *Controller) syncRuleStatus(ctx context.Context, name string) error {
	raw, err := c.lister(tagRuleGVR).Get(name)
	if err != nil {
		return nil // Deleted while syncing.
	}
	unstructuredRule, ok := raw.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("tag-rule %s: unexpected object type %T", name, raw)
	}
	rule, err := tagRuleFromUnstructured(unstructuredRule)
	if err != nil {
		return err
	}

	status := TagRuleStatus{
		State:              RuleStateActive,
		ObservedGeneration: rule.Generation,
		LastEvaluated:      &metav1.Time{Time: c.now()},
		Entities:           EntitiesStatus{},
		Values:             ValuesStatus{Dropped: c.takeDrops(rule.Name)},
		Conditions: []metav1.Condition{{
			Type:               "Synced",
			Status:             metav1.ConditionTrue,
			Reason:             "Reconciled",
			LastTransitionTime: metav1.NewTime(c.now()),
		}},
	}
	if err := c.ruleError(rule.Name, rule.Generation); err != nil {
		status.State = RuleStateError
		status.Conditions[0].Status = metav1.ConditionFalse
		status.Conditions[0].Reason = "InvalidRule"
		status.Conditions[0].Message = err.Error()
	}

	selector := rule.Spec.Selector
	status.ObservedSelector = &selector

	for _, entity := range c.listEntities(rule.Spec.Entity) {
		matched, err := selector.matches(entity, rule.Spec.Entity)
		if err != nil {
			return err
		}
		if !matched {
			continue
		}
		status.Entities.Matched++
		if value, tagged, err := ownedTagValue(entity, rule.Spec.Entity, rule.Spec.Tag); err != nil {
			return err
		} else if tagged {
			status.Entities.Tagged++
			if rule.Spec.Value.Type == ValueStringSet {
				if status.Values.Observed == nil {
					status.Values.Observed = map[string]int{}
				}
				status.Values.Observed[value]++
			}
		}
	}

	patch, err := json.Marshal(map[string]any{"status": status})
	if err != nil {
		return err
	}
	_, err = c.dynClient.Resource(tagRuleGVR).Patch(ctx, rule.Name, types.MergePatchType, patch, metav1.PatchOptions{}, "status")
	if err != nil {
		return fmt.Errorf("patching status of rule %s: %w", rule.Name, err)
	}
	return nil
}

func entityID(kind EntityKind, namespace, name string) string {
	if kind == EntityNode {
		return "node/" + name
	}
	return "pod/" + namespace + "/" + name
}

func containsString(values []string, target string) bool {
	return slices.Contains(values, target)
}

func apiIsNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}
