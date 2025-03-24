// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package nvml

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

func TestEnsureinit(t *testing.T) {
	t.Run("library present", func(t *testing.T) {
		var cache nvmlCache
		opts := nvmlCacheInitOpts{
			nvmlNewFunc: func(_ ...nvml.LibraryOption) nvml.Interface {
				return &nvmlmock.Interface{
					InitFunc: func() nvml.Return { return nvml.SUCCESS },
				}
			},
		}

		require.NoError(t, cache.ensureInitWithOpts(opts))
	})
	t.Run("library absent", func(t *testing.T) {
		var cache nvmlCache
		opts := nvmlCacheInitOpts{
			nvmlNewFunc: func(_ ...nvml.LibraryOption) nvml.Interface {
				return &nvmlmock.Interface{
					InitFunc: func() nvml.Return { return nvml.ERROR_LIBRARY_NOT_FOUND },
				}
			},
		}
		require.Error(t, cache.ensureInitWithOpts(opts))
	})

	t.Run("library absent, second call fails too", func(t *testing.T) {
		var cache nvmlCache
		opts := nvmlCacheInitOpts{
			nvmlNewFunc: func(_ ...nvml.LibraryOption) nvml.Interface {
				return &nvmlmock.Interface{
					InitFunc: func() nvml.Return { return nvml.ERROR_LIBRARY_NOT_FOUND },
				}
			},
		}
		require.Error(t, cache.ensureInitWithOpts(opts))
		require.Error(t, cache.ensureInitWithOpts(opts))
	})

	t.Run("library absent, second call succeeds", func(t *testing.T) {
		var cache nvmlCache
		alreadyCalled := false
		opts := nvmlCacheInitOpts{
			nvmlNewFunc: func(_ ...nvml.LibraryOption) nvml.Interface {
				return &nvmlmock.Interface{
					InitFunc: func() nvml.Return {
						if alreadyCalled {
							return nvml.SUCCESS
						}
						alreadyCalled = true
						return nvml.ERROR_LIBRARY_NOT_FOUND
					},
				}
			},
		}
		require.Error(t, cache.ensureInitWithOpts(opts))
		require.NoError(t, cache.ensureInitWithOpts(opts))
	})
}
