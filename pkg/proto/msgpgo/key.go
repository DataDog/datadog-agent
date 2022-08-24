// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package msgpgo

// RemoteConfigKey is a RemoteConfigKey
type RemoteConfigKey struct {
	AppKey     string `msgpack:"key"`
	OrgID      int64  `msgpack:"org"`
	Datacenter string `msgpack:"dc"`
}
