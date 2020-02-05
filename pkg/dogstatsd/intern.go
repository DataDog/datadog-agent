package dogstatsd

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

func (i *stringInterner) LoadOrStore(key []byte) string {
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
