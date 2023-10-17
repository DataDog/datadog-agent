package cache

// Refcounted tracks references.  The interface doesn't provide a reference construction
// mechanism, use the per-type implementation functions for that.
type Refcounted interface {
	Release(n int32)
}

// InternRetainer counts references to internal types, that we can then release later.  Here, it's
// used for stringInterner instances.
type InternRetainer interface {
	Reference(obj Refcounted)
	ReferenceN(obj Refcounted, n int32)
	// CopyTo duplicates all references in this InternRetainer into dest.
	CopyTo(dest InternRetainer)
	// Import takes all the references from source.  Source will hae no references after this operation.
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

func (s *SmallRetainer) Reference(obj Refcounted) {
	s.ReferenceN(obj, 1)
}
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

func (s *SmallRetainer) ReleaseAll() {
	s.ReleaseAllWith(func(obj Refcounted, count int32) {
		obj.Release(count)
	})
}

func (s *SmallRetainer) CopyTo(dest InternRetainer) {
	for n, o := range s.origins {
		dest.ReferenceN(o, s.counts[n])
	}
}

func (s *SmallRetainer) Import(other InternRetainer) {
	other.ReleaseAllWith(s.ReferenceN)
}

type RetainerBlock struct {
	retentions map[Refcounted]int32
}

func NewRetainerBlock() *RetainerBlock {
	return &RetainerBlock{
		retentions: make(map[Refcounted]int32),
	}
}

func (r *RetainerBlock) Reference(obj Refcounted) {
	r.retentions[obj] += 1
}

func (r *RetainerBlock) ReferenceN(obj Refcounted, n int32) {
	r.retentions[obj] += n
}

func (r *RetainerBlock) ReleaseAllWith(callback func(obj Refcounted, count int32)) {
	for k, v := range r.retentions {
		callback(k, v)
		delete(r.retentions, k)
	}
}

func (r *RetainerBlock) ReleaseAll() {
	r.ReleaseAllWith(func(obj Refcounted, count int32) {
		obj.Release(count)
	})
}

func (r *RetainerBlock) Import(other InternRetainer) {
	other.ReleaseAllWith(func(obj Refcounted, count int32) {
		r.retentions[obj] += count
	})
}

func (r *RetainerBlock) CopyTo(other InternRetainer) {
	for k, v := range r.retentions {
		other.ReferenceN(k, v)
	}
}
