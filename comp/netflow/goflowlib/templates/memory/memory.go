// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package memory provides an in-memory NetFlow/IPFIX template store with a
// bounded number of entries. It is derived from the goflow2 in-memory template
// driver, adding least-recently-used eviction once the limit is reached.
package memory

import (
	"container/list"
	"context"
	"sync"

	"github.com/netsampler/goflow2/decoders/netflow/templates"
)

type entry struct {
	key  templates.TemplateKey
	data interface{}
}

// Driver stores templates in memory, evicting the least recently used
// template when the number of stored templates would exceed the maximum.
type Driver struct {
	lock      sync.Mutex
	maxSize   int
	templates map[templates.TemplateKey]*list.Element
	order     *list.List // front is most recently used
	evictions uint64
}

var _ templates.TemplateInterface = (*Driver)(nil)

// New returns a Driver holding at most maxSize templates. A maxSize <= 0
// means the number of templates is not limited.
func New(maxSize int) *Driver {
	return &Driver{
		maxSize:   maxSize,
		templates: make(map[templates.TemplateKey]*list.Element),
		order:     list.New(),
	}
}

// Close releases the driver's resources. It is a no-op.
func (d *Driver) Close(context.Context) error {
	return nil
}

// Len returns the number of templates currently stored.
func (d *Driver) Len() int {
	d.lock.Lock()
	defer d.lock.Unlock()
	return len(d.templates)
}

// Evictions returns the total number of templates evicted to make room.
func (d *Driver) Evictions() uint64 {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.evictions
}

// ListTemplates sends the key of every stored template to ch, followed by nil.
func (d *Driver) ListTemplates(ctx context.Context, ch chan *templates.TemplateKey) error {
	d.lock.Lock()
	keys := make([]*templates.TemplateKey, 0, len(d.templates))
	for el := d.order.Front(); el != nil; el = el.Next() {
		k := el.Value.(*entry).key
		keys = append(keys, &k)
	}
	d.lock.Unlock()

	for _, k := range keys {
		select {
		case ch <- k:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	select {
	case ch <- nil:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// AddTemplate stores a template, replacing any existing one with the same key.
// If the store is full, the least recently used template is evicted.
func (d *Driver) AddTemplate(_ context.Context, key *templates.TemplateKey, template interface{}) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	if el, ok := d.templates[*key]; ok {
		el.Value.(*entry).data = template
		d.order.MoveToFront(el)
		return nil
	}

	for d.maxSize > 0 && len(d.templates) >= d.maxSize {
		oldest := d.order.Back()
		d.order.Remove(oldest)
		delete(d.templates, oldest.Value.(*entry).key)
		d.evictions++
	}

	d.templates[*key] = d.order.PushFront(&entry{key: *key, data: template})
	return nil
}

// GetTemplate returns the template stored for key, or nil if there is none.
func (d *Driver) GetTemplate(_ context.Context, key *templates.TemplateKey) (interface{}, error) {
	d.lock.Lock()
	defer d.lock.Unlock()

	el, ok := d.templates[*key]
	if !ok {
		return nil, nil
	}
	d.order.MoveToFront(el)
	return el.Value.(*entry).data, nil
}
