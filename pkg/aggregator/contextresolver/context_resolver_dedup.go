package contextresolver

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/contextresolver/dedup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util"
	"strings"
)

// Dedup allows tracking and expiring contexts
type Dedup struct {
	contextResolverBase
	stringSet *dedup.StringSet
	contextsByKey map[ckey.ContextKey]*ContextDedup
}

func NewDedup() *Dedup {
	return &Dedup{
		contextResolverBase: contextResolverBase{
			keyGenerator: ckey.NewKeyGenerator(),
			tagsBuffer:   util.NewHashingTagsBuilder(),
		},
		stringSet: dedup.NewStringSet(),
		contextsByKey: make(map[ckey.ContextKey]*ContextDedup),
	}
}

// TrackContext returns the contextKey associated with the context of the metricSample and tracks that context
func (cr *Dedup) TrackContext(metricSampleContext metrics.MetricSampleContext) ckey.ContextKey {
	metricSampleContext.GetTags(cr.tagsBuffer)               // tags here are not sorted and can contain duplicates
	contextKey := cr.generateContextKey(metricSampleContext) // the generator will remove duplicates from cr.tagsBuffer (and doesn't mind the order)

	if _, ok := cr.Get(contextKey); !ok {
		// making a copy of tags for the context since tagsBuffer
		// will be reused later. This allows us to allocate one slice
		// per context instead of one per sample.
		context := &Context{
			Name: metricSampleContext.GetName(),
			Tags: cr.tagsBuffer.Copy(),
			Host: metricSampleContext.GetHost(),
		}
		cr.Add(contextKey, context)
	}

	cr.tagsBuffer.Reset()
	return contextKey
}

func (cr *Dedup) Add(key ckey.ContextKey, context *Context) {
	cr.contextsByKey[key] = NewContextDedup(context.Host, context.Name, context.Tags, cr.stringSet)
}

// Get gets a context matching a key
func (cr *Dedup) Get(key ckey.ContextKey) (*Context, bool) {
	ctx, found := cr.contextsByKey[key]
	if !found {
		return nil, found
	}
	return ctx.Context(), found
}

// Size returns the number of objects in the cache
func (cr *Dedup) Size() int {
	return len(cr.contextsByKey)
}

func (cr *Dedup) removeKey(key ckey.ContextKey) {
	ctx, found := cr.contextsByKey[key]
	if found {
		ctx.Drop()
		delete(cr.contextsByKey, key)
	}
}

func (cr *Dedup) removeKeys(expiredContextKeys []ckey.ContextKey) {
	for _, key := range expiredContextKeys {
		cr.removeKey(key)
	}
}

// Clear drops all contexts
func (cr *Dedup) Clear() {
	// We need to iterate on all the keys because we need to dec the references
	for key, _ := range cr.contextsByKey {
		cr.removeKey(key)
	}
}

// Close frees up resources
func (cr *Dedup) Close() {
	cr.Clear()
}

type ContextDedup struct {
	ss *dedup.StringSet
	// All the pointers are pointers to strings from the StringSet
	Host *string
	Name   *string
	Keys   []*string
	Values []*string
}

func NewContextDedup(host, name string, tags []string, ss *dedup.StringSet) *ContextDedup {
	// We split Keys and Values from the tags to get optimal deduplication since the
	// Keys are way more likely to be duplicated than the Values.

	keys := make([]*string, len(tags))
	values := make([]*string, len(tags))

	for i, tag := range tags {
		n := strings.IndexRune(tag, ':')
		if n == -1 {
			keys[i] = ss.Get(tag)
			values[i] = nil
		} else {
			keys[i] = ss.Get(tag[0:n+1])
			values[i] = ss.Get(tag[n+1:])
		}
	}

	return &ContextDedup{
		ss:     ss,
		Host:   ss.Get(host),
		Name:   ss.Get(name),
		Keys:   keys,
		Values: values,
	}
}

func (c *ContextDedup) Drop() {
	c.ss.Dec(c.Name)
	c.ss.Dec(c.Host)
	for _, k := range c.Keys {
		c.ss.Dec(k)
	}
	for _, v := range c.Values {
		c.ss.Dec(v)
	}
}

func (c *ContextDedup) Context() *Context {
	tags := make([]string, len(c.Keys))

	// re-building the tags
	for i, k := range c.Keys {
		var b strings.Builder

		b.WriteString(*k)
		if c.Values[i] != nil {
			b.WriteString(*c.Values[i])
		}
		tags[i] = b.String()
	}

	return &Context {
		Host: *c.Host,
		Name: *c.Name,
		Tags: tags,
	}
}
