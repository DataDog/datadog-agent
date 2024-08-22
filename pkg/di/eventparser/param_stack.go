// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package eventparser

import (
	"github.com/DataDog/datadog-agent/pkg/di/ditypes"
)

type paramStack struct {
	arr []*ditypes.Param
}

func newParamStack() *paramStack {
	s := paramStack{arr: []*ditypes.Param{}}
	return &s
}

func (s *paramStack) isEmpty() bool {
	return len(s.arr) == 0
}

func (s *paramStack) pop() *ditypes.Param {
	if s.isEmpty() {
		return nil
	}
	top := s.peek()
	s.arr = s.arr[0 : len(s.arr)-1]
	return top
}

func (s *paramStack) peek() *ditypes.Param {
	if s.isEmpty() {
		return nil
	}
	return s.arr[len(s.arr)-1]
}

func (s *paramStack) push(p *ditypes.Param) {
	s.arr = append(s.arr, p)
}
