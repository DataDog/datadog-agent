// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package missedbytes

import (
	"embed"
	"testing"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// The golden files were rendered by BuildIssue before it moved onto the shared
// logsissue helpers, so they pin what customers read.
//
//go:embed testdata/golden/*.json
var golden embed.FS

func TestBuildIssueMatchesGolden(t *testing.T) {
	for name, ctx := range goldenCases {
		t.Run(name, func(t *testing.T) {
			raw, err := golden.ReadFile("testdata/golden/" + name + ".json")
			require.NoError(t, err)
			want := &healthplatform.Issue{}
			require.NoError(t, protojson.Unmarshal(raw, want))

			got, err := MissedBytesIssue{}.BuildIssue(ctx)
			require.NoError(t, err)
			if !proto.Equal(want, got) {
				gotJSON, _ := protojson.MarshalOptions{Multiline: true}.Marshal(got)
				t.Fatalf("rendered issue changed:\n--- want\n%s\n--- got\n%s", raw, gotJSON)
			}
		})
	}
}
