// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package filesystem

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetTimeout(t *testing.T) {
	prevTimeout := timeout
	timeout = time.Nanosecond
	defer func() {
		timeout = prevTimeout
	}()

	mountInfo, err := new(FileSystem).Get()
	fmt.Println(mountInfo, err)
	require.ErrorIs(t, err, ErrTimeoutExceeded)
}
