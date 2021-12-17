package tagset

// DefaultFactory is a global thread-safe factory, used by calls to
// package-level functions. This is suitable for non-performance-critical tags
// manipulation
var DefaultFactory Factory

// EmptyTags is a ready-to-use Tags instance that contains no tags. Use this
// instead of nil values for *Tags.
var EmptyTags *Tags

func init() {
	cf, _ := NewCachingFactory(1000, 5)
	DefaultFactory = NewThreadsafeFactory(cf)
	EmptyTags = NewTags([]string{})
}

// NewTags calls DefaultFactory.NewTags
func NewTags(tags []string) *Tags {
	return DefaultFactory.NewTags(tags)
}

// NewUniqueTags calls DefaultFactory.NewUniqueTags
func NewUniqueTags(tags ...string) *Tags {
	return DefaultFactory.NewUniqueTags(tags...)
}

// NewTagsFromMap calls DefaultFactory.NewTagsFromMap
func NewTagsFromMap(tags map[string]struct{}) *Tags {
	return DefaultFactory.NewTagsFromMap(tags)
}

// NewBuilder calls DefaultFactory.NewBuilder
func NewBuilder(capacity int) *Builder {
	return DefaultFactory.NewBuilder(capacity)
}

// NewSliceBuilder calls DefaultFactory.NewSliceBuilder
func NewSliceBuilder(levels, capacity int) *SliceBuilder {
	return DefaultFactory.NewSliceBuilder(levels, capacity)
}

// Union calls DefaultFactory.Union
func Union(a, b *Tags) *Tags {
	return DefaultFactory.Union(a, b)
}
