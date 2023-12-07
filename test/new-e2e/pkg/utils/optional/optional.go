// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package optional

func MakeParams[Param any, Option ~func(*Param) error](options ...Option) (*Param, error) {
	var p Param
	if err := ApplyOptions(&p, options); err != nil {
		return nil, err
	}

	return &p, nil
}

func ApplyOptions[Param any, Option ~func(*Param) error](instance *Param, options []Option) error {
	for _, o := range options {
		if err := o(instance); err != nil {
			return err
		}
	}

	return nil
}
