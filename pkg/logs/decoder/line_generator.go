// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import "github.com/DataDog/datadog-agent/pkg/logs/parser"

// LineGenerator encapsulates the details of log reading and parsing. In general,
// line generator decide whether to cut the cached bytes as a line when:
// * a EndLine matched according to the specific matcher configured.
// * caching bytes reaches capacity, which is defined by 'maxLen'
//
// To encapsulate the information of the content sent to downstream, struct Line
// is introduced. To form a Line, generator takes the cached bytes, pass to a
// Convertor to get the final Line instance.
//
// The operation is completed so far for the first case above. For the 2nd case,
// extra information is required in order to help truncation logic. For this,
// RichLine struct is introduced, it contains 2 more fields to know if a line is
// cut by generator (whether it is a part of a long log with length exceeds maxLen).
type LineGenerator struct {
	maxLen           int // max decode length
	inputChan        chan *Input
	endLineMatcher   EndLineMatcher
	convertor        parser.Convertor
	handlerScheduler LineHandlerScheduler
}

// Start prepares the process for reading logs.
func (l *LineGenerator) Start() {
	l.handlerScheduler.Start()
	go func() {
		for chunk := range l.inputChan {
			l.read(chunk)
		}
		l.handlerScheduler.Stop()
	}()
}

// read reads the input chunks check if match the endline criteria,
// form a line if matches, it also forms a line if the length reaches
// maxLen limit.
func (l *LineGenerator) read(chunk *Input) {
	//TODO
}

// RichLine takes extra fields to give necessary information to generate a Output message.
type RichLine struct {
	parser.Line
	// needLeading indicates if leading truncation information is required, typically
	// it sets to true for the non-first part of a log (cut by line generator).
	needLeading bool
	// needTailing indicates if tailing truncation information is required. When
	// the line is not the last part of a log (cut by line generator), this flag needs
	// to be true.
	needTailing bool
}
