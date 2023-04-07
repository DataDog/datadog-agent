// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package processes

import "flag"

var options struct {
	limit int
}

type Processes struct{}

const name = "processes"

func init() {
	flag.IntVar(&options.limit, name+"-limit", 20, "Number of process groups to return")
}

func (self *Processes) Name() string {
	return name
}

func (self *Processes) Collect() (result interface{}, err error) {
	// even if getProcesses returns nil, simply assigning to result
	// will have a non-nil return, because it has a valid inner
	// type (more info here: https://golang.org/doc/faq#nil_error )
	// so, jump through the hoop of temporarily storing the return,
	// and explicitly return nil if it fails.
	gpresult, err := getProcesses(options.limit)
	if gpresult == nil {
		return nil, err
	}
	return gpresult, err
}
