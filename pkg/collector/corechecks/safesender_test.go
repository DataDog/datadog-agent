// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package corechecks

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloneTags(t *testing.T) {
	t.Run("tags", func(t *testing.T) {
		tags := []string{"tag1", "tag2"}
		cloned := cloneTags(tags)
		tags[0] = "uh-oh"
		tags[1] = "oh no"
		require.Equal(t, []string{"tag1", "tag2"}, cloned)
	})

	t.Run("nil", func(t *testing.T) {
		cloned := cloneTags(nil)
		require.Nil(t, cloned)
	})
}
