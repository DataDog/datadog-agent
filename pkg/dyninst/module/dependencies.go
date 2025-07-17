// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"io"

	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcscrape"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
)

// Scraper is an interface that enables the Controller to get updates from the
// scraper and to set the probe status to emitting.
type Scraper interface {
	// GetUpdates returns the current set of updates.
	GetUpdates() []rcscrape.ProcessUpdate
}

// DecoderFactory is a factory for creating decoders.
type DecoderFactory interface {
	NewDecoder(*ir.Program) (Decoder, error)
}

// Decoder is a decoder for a program.
type Decoder interface {
	// Decode writes the decoded event to the output writer and returns the
	// relevant probe definition.
	Decode(
		event decode.Event,
		symbolicator symbol.Symbolicator,
		out io.Writer,
	) (ir.ProbeDefinition, error)
}

// DefaultDecoderFactory is the default decoder factory.
type DefaultDecoderFactory struct{}

// NewDecoder creates a new decoder using decode.NewDecoder.
func (DefaultDecoderFactory) NewDecoder(program *ir.Program) (Decoder, error) {
	decoder, err := decode.NewDecoder(program)
	if err != nil {
		return nil, err
	}
	return decoder, nil
}
