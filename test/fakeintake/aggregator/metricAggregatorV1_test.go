// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"testing"
)

/*
//go:embed fixtures/metric_bytes
var metricsData []byte
*/
func TestV1MetricPayloads(t *testing.T) {
	t.Run("ParseV1MetricSeries ", func(t *testing.T) {
		data := []byte("eJy8ktFqMjEQhd9lrmNIdvdfNa/xX8qyhDiuKd1JSCaCiO9e1lbQamFbSm9PTma+4ZwTZEweM5jNCUbk5B0Y2Fq22zBIOyCxzGwT4xYExOCJJ+9Gt+u2qatGKaG7TgDbYdLhgCn7QGYpm6XU0AnYh8xgwC+U1m27a1y9quqq+edqEMDHiGDAhUIMAjwxpoN9BaPVWdwA5WNmHKWLRVIZexcS5uc8a1Hd8MzcP9gy4N1+JSCHkhz2k6UnO06+/xcMuEMjjjLsdhn5K6CFkkopVderVv8t232O8cj7QPIjoqe0+lOc71/6a6oT1mXWVelH+xKSWT7qnkIyzeNDtOz2ZnYzfu/6VIg8DbPO/lmLv8vand8CAAD//+MjG/o=")
		enflated, _ := enflate(data, "deflate")
		println(enflated)
	})
}
