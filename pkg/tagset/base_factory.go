// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

// baseFactory provides some utility functions that are useful in all factory
// implementations.
type baseFactory struct {
	// builders is a cache of unused Builder instances for reuse. Because
	// factories are not thread-safe, there is no need to synchronize access
	// to this list.
	builders []*Builder

	// sliceBuilders is a cache of unused SliceBuilder instances for reuse.
	// Because factories are not thread-safe, there is no need to synchronize
	// access to this list.
	sliceBuilders []*SliceBuilder
}

// newBuilder implements NewBuilder for a factory
func (f *baseFactory) newBuilder(ff Factory, capacity int) *Builder {
	var bldr *Builder
	if len(f.builders) > 0 {
		l := len(f.builders)
		bldr, f.builders = f.builders[l-1], f.builders[:l-1]
	} else {
		bldr = newBuilder(ff)
	}
	bldr.reset(capacity)
	return bldr
}

// newSliceBuilder implements NewSliceBuilder for a factory
func (f *baseFactory) newSliceBuilder(ff Factory, levels, capacity int) *SliceBuilder {
	var bldr *SliceBuilder
	if len(f.sliceBuilders) > 0 {
		l := len(f.sliceBuilders)
		bldr, f.sliceBuilders = f.sliceBuilders[l-1], f.sliceBuilders[:l-1]
	} else {
		bldr = newSliceBuilder(ff)
	}
	bldr.reset(levels, capacity)
	return bldr
}

func (f *baseFactory) builderClosed(builder *Builder) {
	f.builders = append(f.builders, builder)
}

func (f *baseFactory) sliceBuilderClosed(builder *SliceBuilder) {
	f.sliceBuilders = append(f.sliceBuilders, builder)
}
