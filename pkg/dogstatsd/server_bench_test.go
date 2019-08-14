// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package dogstatsd

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners"
)

func BenchmarkParsePacket(b *testing.B) {
	s, _ := NewServer(nil, nil, nil)
	defer s.Stop()

	packet := listeners.Packet{
		Contents: []byte("daemon:666|h|@0.5|#sometag1:somevalue1,sometag2:somevalue2"),
		Origin:   listeners.NoOrigin,
	}
	for n := 0; n < b.N; n++ {
		s.parsePacket(&packet, nil, nil, nil)
	}
}
