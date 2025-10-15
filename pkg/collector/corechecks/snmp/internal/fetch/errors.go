// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fetch

import (
	"fmt"
)

type fetchError struct {
	oidType       oidType
	oidList       []string
	snmpOperation snmpOperation
	err           error
}

func newFetchError(oidType oidType, oidList []string, snmpOperation snmpOperation, err error) *fetchError {
	return &fetchError{
		oidType:       oidType,
		oidList:       oidList,
		snmpOperation: snmpOperation,
		err:           err,
	}
}

func (e *fetchError) Error() string {
	return fmt.Sprintf("fetch %s: failed getting oids `%v` using %s: %v",
		e.oidType, e.oidList, e.snmpOperation, e.err)
}

func (e *fetchError) Unwrap() error {
	return e.err
}
