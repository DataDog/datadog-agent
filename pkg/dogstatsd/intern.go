package dogstatsd

// stringInterner is a string cache providing a longer life for strings,
// helping to avoid GC runs because they're re-used many times instead of
// created every time.
type stringInterner struct {
	strings map[string]string
	maxSize int
}

func newStringInterner(maxSize int) *stringInterner {
	return &stringInterner{
		strings: make(map[string]string),
		maxSize: maxSize,
	}
}

// LoadOrStore always returns the string from the cache, adding it into the
// cache if needed.
// If we need to store a new entry and the cache is at its maximum capacity,
// it is reset.
func (i *stringInterner) LoadOrStore(key []byte) string {
	// here is the string interner trick: the map lookup using
	// string(key) doesn't actually allocate a string, but is
	// returning the string value -> no new heap allocation
	// for this string.
	// See https://github.com/golang/go/commit/f5f5a8b6209f84961687d993b93ea0d397f5d5bf
	if s, found := i.strings[string(key)]; found {
		return s
	}
	if len(i.strings) >= i.maxSize {
		i.strings = make(map[string]string)
	}
	s := string(key)
	i.strings[s] = s
	return s
}
