// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package parser

// NoopConvertor implements Convertor.
type NoopConvertor struct {
	Convertor
}

// NewNoopConvertor create a new instance of NoopConvertor.
func NewNoopConvertor() *NoopConvertor {
	return &NoopConvertor{}
}

// Convert take the input content and defaultPrefix to construct a Line instance.
func (n *NoopConvertor) Convert(content []byte, defaultPrefix Prefix) *Line {
	if len(content) > 0 {
		return &Line{
			Content: content,
			Size:    len(content),
			Prefix:  defaultPrefix,
		}
	}
	return nil
}

// Convertor should replace current Parser.
// Convertor contains one method which convert content from byte array to struct.
type Convertor interface {
	// Convert converts log content from byte array to struct Line, if the content
	// is partial, meaning content has no prefix, a default prefix should be used.
	Convert(content []byte, defaultPrefix Prefix) *Line
}

// Line struct defines the fields from a typical log. Prefix can be found from some
// log sources like kubernetes or docker... Content holds the actual log message.
// Size is the actual length of this Line. Size might be different than len(Content)
// due to the extra truncation information added by LineTruncator.
type Line struct {
	Prefix
	Content []byte
	Size    int
}

// Prefix defines the information comes with the actual log message. It is normally
// been added by some containers who run the application, like kubernetes or docker.
// Some field in Prefix maybe absent depending on the source.
type Prefix struct {
	Timestamp string
	Status    string
	Flag      string
}
