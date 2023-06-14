// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package platform

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcIsAthlon(t *testing.T) {
	athlonSource := `processor   : 0
vendor_id   : AuthenticAMD
cpu family  : 15
model       : 67
model name  : Dual-Core AMD Opteron(tm) Processor 1218 HE`
	reader := strings.NewReader(athlonSource)
	require.True(t, procIsAthlon(reader))

	notAthlonSource := `processor	: 0
vendor_id	: GenuineIntel
cpu family	: 6
model		: 79
model name	: Intel(R) Xeon(R) CPU E5-2686 v4 @ 2.30GHz`
	reader = strings.NewReader(notAthlonSource)
	require.False(t, procIsAthlon(reader))
}
