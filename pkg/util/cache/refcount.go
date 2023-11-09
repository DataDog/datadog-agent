package cache

import (
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"strings"
	"sync"
)

// Refcounted tracks references.  The interface doesn't provide a reference construction
// mechanism, use the per-type implementation functions for that.
type Refcounted interface {
	// Retain n additional references
	Retain(n int32)
	// Release n references.
	Release(n int32)
	// Name of the object.
	Name() string
}

// InternRetainer counts references to internal types, that we can then release later.  Here, it's
// used for stringInterner instances.
type InternRetainer interface {
	// Reference the obj once
	Reference(obj Refcounted)
	// ReferenceN References obj n times.
	ReferenceN(obj Refcounted, n int32)
	// CopyTo duplicates all references in this InternRetainer into dest.
	CopyTo(dest InternRetainer)
	// Import takes all the references from source.  Source will have no references after this operation.
	Import(source InternRetainer)
	// ReleaseAll calls Release once on every object it's ever Referenced for each time it was referenced.
	ReleaseAll()
	// ReleaseAllWith lets the callback do the releasing, or take ownership of the retentions.  This
	// InternRetainer will forget retentions passed into ReleaseAllWith.
	ReleaseAllWith(func(obj Refcounted, count int32))
}

// SmallRetainer is a tiny (2-slice) retainer structure that doesn't need explicit initialization
type SmallRetainer struct {
	origins []Refcounted
	counts  []int32
}

// Len has the number of origins.
func (s *SmallRetainer) Len() int {
	return len(s.origins)
}

// Reference an object once.
func (s *SmallRetainer) Reference(obj Refcounted) {
	s.ReferenceN(obj, 1)
}

// ReferenceN an object n times
func (s *SmallRetainer) ReferenceN(obj Refcounted, n int32) {
	if s.origins == nil {
		s.origins = make([]Refcounted, 0, 2)
		s.counts = make([]int32, 0, 2)
	} else {
		for i := 0; i < len(s.origins); i++ {
			if s.origins[i] == obj {
				s.counts[i] += n
				return
			}
		}
	}

	s.origins = append(s.origins, obj)
	s.counts = append(s.counts, n)
}

// ReleaseAllWith uses callback to release everything it holds, then forgets what
// it has.
func (s *SmallRetainer) ReleaseAllWith(callback func(obj Refcounted, count int32)) {
	if s.origins == nil {
		return
	}
	for i := 0; i < len(s.origins); i++ {
		callback(s.origins[i], s.counts[i])
	}
	s.origins = s.origins[:0]
	s.counts = s.counts[:0]
}

// ReleaseAll releases every object.
func (s *SmallRetainer) ReleaseAll() {
	s.ReleaseAllWith(func(obj Refcounted, count int32) {
		obj.Release(count)
	})
}

// CopyTo copies every reference this RetainerBlock has to other, creating
// additional references to each Refcounted in the process.
func (s *SmallRetainer) CopyTo(dest InternRetainer) {
	for n, o := range s.origins {
		o.Retain(s.counts[n])
		dest.ReferenceN(o, s.counts[n])
	}
}

// Import another retainer's retentions.  The other retainer loses
// its retentions after this.
func (s *SmallRetainer) Import(other InternRetainer) {
	if other != nil {
		other.ReleaseAllWith(s.ReferenceN)
	}
}

// RetainerBlock is a synchronized Retainer.
type RetainerBlock struct {
	retentions map[Refcounted]int32
	lock       sync.Mutex
}

// NewRetainerBlock allocates a RetainerBlock.
func NewRetainerBlock() *RetainerBlock {
	return &RetainerBlock{
		retentions: make(map[Refcounted]int32),
		lock:       sync.Mutex{},
	}
}

// Reference an object once.
func (r *RetainerBlock) Reference(obj Refcounted) {
	r.lock.Lock()
	r.retentions[obj]++
	r.lock.Unlock()
}

// ReferenceN an object n times.
func (r *RetainerBlock) ReferenceN(obj Refcounted, n int32) {
	r.lock.Lock()
	r.retentions[obj] += n
	r.lock.Unlock()
}

// ReleaseAllWith uses callback to release everything it holds, then forgets what
// it has.
func (r *RetainerBlock) ReleaseAllWith(callback func(obj Refcounted, count int32)) {
	r.lock.Lock()
	for k, v := range r.retentions {
		callback(k, v)
		delete(r.retentions, k)
	}
	r.lock.Unlock()
}

// ReleaseAll releases every object.
func (r *RetainerBlock) ReleaseAll() {
	// Don't lock as we're calling a locking method.
	r.ReleaseAllWith(func(obj Refcounted, count int32) {
		obj.Release(count)
	})
}

// Import another retainer's retentions.  The other retainer loses
// its retentions after this.
func (r *RetainerBlock) Import(other InternRetainer) {
	r.lock.Lock()
	other.ReleaseAllWith(func(obj Refcounted, count int32) {
		r.retentions[obj] += count
	})
	r.lock.Unlock()
}

// CopyTo copies every reference this RetainerBlock has to other, creating
// additional references to each Refcounted in the process.
func (r *RetainerBlock) CopyTo(other InternRetainer) {
	r.lock.Lock()
	for k, v := range r.retentions {
		k.Retain(v)
		other.ReferenceN(k, v)
	}
	r.lock.Unlock()
}

// Summarize generates a string summary of what's summarized (via Name) and how many references
// each Refcounted gets.
func (r *RetainerBlock) Summarize() string {
	r.lock.Lock()
	p := message.NewPrinter(language.English)
	s := strings.Builder{}
	var total int32
	s.WriteString(p.Sprintf("{%d keys. ", len(r.retentions)))
	for k, v := range r.retentions {
		s.WriteString(p.Sprintf("%s: %d, ", k.Name(), v))
		total += v
	}
	s.WriteString(p.Sprintf("; %d total}", total))
	r.lock.Unlock()
	return s.String()
}
