// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build !linux

package network

import "net"

func netInterfaces() ([]net.Interface, error) {
	return net.Interfaces()
}
