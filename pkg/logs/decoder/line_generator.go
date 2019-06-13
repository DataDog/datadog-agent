// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import "github.com/DataDog/datadog-agent/pkg/logs/parser"

type LineGenerator struct {
	maxLen           int // max decode length
	inputChan        chan *Input
	endLineMatcher   EndLineMatcher
	convertor        parser.Convertor
	handlerScheduler LineHandlerScheduler
}

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

type RichLine struct {
	parser.Line
	// flag to know if it's necessary to add leading '...TRUNCATED...'
	needLeading bool
	// flag to know if it's necessary to add tailing '...TRUNCATED...'
	needTailing bool
}
