// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
)

type EC2Metadata struct {
	vm    *VM
	token string
}

const metadataEndPoint = "http://169.254.169.254"

func NewEC2Metadata(vm *VM) *EC2Metadata {
	cmd := fmt.Sprintf(`curl -s -X PUT "%v/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600"`, metadataEndPoint)
	output := vm.Execute(cmd)
	return &EC2Metadata{vm: vm, token: output}
}

func (m *EC2Metadata) Get(name string) string {
	cmd := fmt.Sprintf(`curl -s -H "X-aws-ec2-metadata-token: %v" "%v/latest/meta-data/%v"`, m.token, metadataEndPoint, name)
	return m.vm.Execute(cmd)
}
