// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package snmpscanmanagerimpl implements the snmpscanmanager component interface
package snmpscanmanagerimpl

import (
	"fmt"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	snmpscanmanager "github.com/DataDog/datadog-agent/comp/snmpscanmanager/def"
)

type Requires struct {
	compdef.In
}

type Provides struct {
	Comp snmpscanmanager.Component
}

func NewComponent(reqs Requires) (Provides, error) {
	fmt.Println("==================================")
	fmt.Println("==================================")
	fmt.Println("==================================")
	fmt.Println("==================================")
	fmt.Println("==================================")
	fmt.Println("==================================")
	fmt.Println("==================================")
	fmt.Println("SNMPSCAN MANAGER COMPONENT")
	fmt.Println("==================================")
	fmt.Println("==================================")
	fmt.Println("==================================")
	fmt.Println("==================================")
	fmt.Println("==================================")
	fmt.Println("==================================")
	fmt.Println("==================================")

	scanManager := snmpScanManagerImpl{}

	return Provides{
		Comp: scanManager,
	}, nil
}

type snmpScanManagerImpl struct {
}

func (s snmpScanManagerImpl) TestPrint() {
	fmt.Println("HELLO")
}
