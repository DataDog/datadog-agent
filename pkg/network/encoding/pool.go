package encoding

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"
)

type pool struct {
	New func() interface{}
	TTL time.Duration

	mux    sync.Mutex
	now    func() int64
	ttlSec int64
	list   *list.List
	// divider is a sentinel value that partitions the list in two
	// 1. we do this so we can pool not only the client code objects but also the underlying list.Element objects
	// 2. we can't have two lists instead because standard library doesn't let you move elements between lists
	// list structure:
	// FRONT [empty list.Element objects | divider | filled list.Element objects] BACK
	divider *list.Element
}

type entry struct {
	obj      interface{}
	deadline int64
}

func (p *pool) Get() interface{} {
	p.mux.Lock()
	defer p.mux.Unlock()

	if p.list == nil {
		p.init()
		p.start()
	}

	if el := p.list.Back(); el != nil && el != p.divider {
		entry := el.Value.(*entry)
		obj := entry.obj

		// here we recycle the *list.Element object itself and move it to the first partition of the list
		entry.obj = nil
		entry.deadline = p.now() + p.ttlSec
		p.list.MoveBefore(el, p.divider)

		return obj
	}

	return p.New()
}

func (p *pool) Put(obj interface{}) {
	p.mux.Lock()
	defer p.mux.Unlock()

	if el := p.divider.Prev(); el != nil {
		// Recycle *list.Element if available
		entry := el.Value.(*entry)
		entry.obj = obj
		entry.deadline = p.now() + p.ttlSec

		p.list.MoveToBack(el)
		return
	}

	entry := &entry{obj: obj, deadline: p.now() + p.ttlSec}
	p.list.PushBack(entry)
}

// we lazily initiate stuff to match the sync.Pool API
func (p *pool) init() {
	if p.TTL == 0 {
		// 40 seconds ensures that objects will be pooled until the next agent check
		p.TTL = 40 * time.Second
	}

	p.ttlSec = int64(p.TTL.Seconds())
	p.now = clock.now
	p.list = list.New()
	p.divider = p.list.PushBack(nil)
}

func (p *pool) start() {
	clearInterval := time.Duration(p.TTL.Nanoseconds() / 2)
	ticker := time.NewTicker(clearInterval)
	go func() {
		for range ticker.C {
			p.clear()
		}
	}()
}

func (p *pool) clear() {
	now := p.now()
	p.mux.Lock()
	defer p.mux.Unlock()

	// Clear old empty list.Element objects
	p.clearSegment(p.list.Front(), now)
	// Clear old list.Element objects carrying pooled objects
	p.clearSegment(p.divider.Next(), now)
}

func (p *pool) clearSegment(el *list.Element, now int64) {
	for el != nil && el != p.divider {
		if entry := el.Value.(*entry); now < entry.deadline {
			break
		}

		next := el.Next()
		p.list.Remove(el)
		el = next
	}
}

var clock *sampledClock

type sampledClock struct {
	nowTS int64
}

func newSampledClock(resolution time.Duration) *sampledClock {
	c := &sampledClock{nowTS: time.Now().Unix()}
	go func() {
		ticker := time.NewTicker(resolution)
		for t := range ticker.C {
			atomic.StoreInt64(&c.nowTS, t.Unix())
		}
	}()
	return c
}

func (c *sampledClock) now() int64 {
	return atomic.LoadInt64(&c.nowTS)
}

func init() {
	clock = newSampledClock(time.Second)
}
