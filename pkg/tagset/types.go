package tagset

// A TagAccumulator accumulates tags.  The underlying type will provide a means of getting
// the resulting tag set.
type TagAccumulator interface {
	// Append the given tags to the tag set
	Append(...string)
	// Append the tags contained in the given HashedTags instance to the tag set.
	AppendHashed(HashedTags)
}
