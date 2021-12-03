package tagset

import (
	"encoding/json"
	"errors"

	"github.com/twmb/murmur3"
)

// UnmarshalBuilder implements json.Unmarshaler and yaml.Unmarshaler for tags,
// acting as a mutable wrapper for the immutable Tags type. It is safe to use
// the zero value of this struct in a call to json.Unmarshal, and it is safe to
// reuse the struct for multiple, sequential Unmarshal operations.
type UnmarshalBuilder struct {
	// Factory is the factory to use to construct the resulting Tags. If this
	// is nil, the default factory is used.
	Factory Factory

	// Tags is the resulting tagset, if unmarshaling was successful.
	Tags *Tags

	// buffer is a temporary slice that can be reused from invocation to
	// invocation
	buffer []string
}

// UnmarshalJSON implements json.Unmarshaler
func (bldr *UnmarshalBuilder) UnmarshalJSON(data []byte) error {
	factory := bldr.Factory
	if factory == nil {
		factory = DefaultFactory
	}

	tags, err := factory.getCachedTagsErr(byJSONCache, murmur3.Sum64(data), func() (*Tags, error) {
		// unmarshal the underlying array of strings
		err := json.Unmarshal(data, &bldr.buffer)
		if err != nil {
			return nil, err
		} else if bldr.buffer == nil {
			// json.Unmarshal([]byte("null"), &[]T) will nil out the slice,
			// but we want to consider that an error.
			return nil, errors.New("Cannot unmarshal Tags from null")
		}

		return factory.NewTags(bldr.buffer), nil
	})

	if err != nil {
		return err
	}
	if tags == nil {
		panic("nil")
	}
	bldr.Tags = tags

	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler.
//
// Note that unmarshalling a null value will not call this method, and the
// `Tags` field will retain whatever value it had before the `yaml.Unmarshal`
// call, possibly leaking data. This is a limitation of the `gopkg.in/yaml.v2`
// package.
func (bldr *UnmarshalBuilder) UnmarshalYAML(unmarshal func(interface{}) error) error {
	factory := bldr.Factory
	if factory == nil {
		factory = DefaultFactory
	}

	// Unlike UnmarshalJSON, we have nothing convenient to hash here in order
	// to recognize a YAML value we have seen before. That's OK, as YAML
	// unmarshaling is known to be slow in many respects. The use of
	// `NewTags`, below, will still deduplicate tagsets.

	// unmarshal the underlying array of strings
	err := unmarshal(&bldr.buffer)
	if err != nil {
		return err
	}

	bldr.Tags = factory.NewTags(bldr.buffer)

	return nil
}
