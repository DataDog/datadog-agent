// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"errors"
	"fmt"
	"math/big"
	"net"
)

// NotOfValue returns the NOT of a value
func NotOfValue(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case int:
		return ^v, nil
	case string:
		// ensure the not value is different
		if v == "" {
			return "<NOT>", nil
		}
		return "", nil
	case bool:
		return !v, nil
	}

	return nil, errors.New("value type unknown")
}

// IPToInt transforms an IP to a big Int
func IPToInt(ip net.IP) (*big.Int, int, error) {
	val := &big.Int{}
	val.SetBytes(ip)
	if len(ip) == net.IPv4len {
		return val, 32, nil
	} else if len(ip) == net.IPv6len {
		return val, 128, nil
	} else {
		return nil, 0, fmt.Errorf("unsupported address length %d", len(ip))
	}
}

// IntToIP transforms a big Int to an IP
func IntToIP(ipInt *big.Int, bits int) net.IP {
	ipBytes := ipInt.Bytes()
	ret := make([]byte, bits/8)
	// Pack our IP bytes into the end of the return array,
	// since big.Int.Bytes() removes front zero padding.
	for i := 1; i <= len(ipBytes); i++ {
		ret[len(ret)-i] = ipBytes[len(ipBytes)-i]
	}
	return ret
}

func KeysOfMap[M ~map[K]V, K comparable, V any](m M) []K {
	r := make([]K, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	return r
}
