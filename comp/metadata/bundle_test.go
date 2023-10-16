// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// This test is disabled for now. This is because TestBundle expect all types returned by comp to be instantiable by FX.
// But types returned in `group` are not (ex comp/core/flare/types:Provider).
//func TestBundleDependencies(t *testing.T) {
//	fxutil.TestBundle(t, Bundle, core.MockBundle,
//		fx.Supply(util.NewNoneOptional[runner.MetadataProvider]()),
//		fx.Provide(func() serializer.MetricSerializer { return nil }),
//	)
//}

func TestMockBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, MockBundle)
}
