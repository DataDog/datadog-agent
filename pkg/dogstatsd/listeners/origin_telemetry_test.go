package listeners

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCountMetricsAndTags(t *testing.T) {
	require := require.New(t)

	metrics, tags := countMetricsAndTags([]byte("♬†øU†øU¥ºuT0♪:61|g|#intitulé:T0µ,another:tag|TT1657100540|@0.21"))
	require.Equal(uint(1), metrics)
	require.Equal(uint(2), tags)

	metrics, tags = countMetricsAndTags([]byte("♬†øU†øU¥ºuT0♪:61|g|#intitulé:T0µ,another:tag,yetanother|TT1657100540|@0.21"))
	require.Equal(uint(1), metrics)
	require.Equal(uint(3), tags)

	metrics, tags = countMetricsAndTags([]byte("♬†øU†øU¥ºuT0♪:61|g|#intitulé:T0µ,another:tag,yetanother|TT1657100540|@0.21\n"))
	require.Equal(uint(1), metrics)
	require.Equal(uint(3), tags)

	metrics, tags = countMetricsAndTags([]byte("♬†øU†øU¥ºuT0♪:61|g|#intitulé:T0µ,another:tag,yetanother|TT1657100540|@0.21\nmetric_name:1:g"))
	require.Equal(uint(2), metrics)
	require.Equal(uint(3), tags)
	metrics, tags = countMetricsAndTags([]byte("♬†øU†øU¥ºuT0♪:61|g|#intitulé:T0µ,another:tag,yetanother|TT1657100540|@0.21\nmetric_name:1:g|#hello:world"))
	require.Equal(uint(2), metrics)
	require.Equal(uint(4), tags)
}
