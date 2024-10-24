// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package metrics

// func TestCompMonitor(t *testing.T) {

// 	i := &IngressMonitor{name: "", instance: "", ticker: time.NewTicker(5 * time.Second)}

// 	i.AddIngress(20)
// 	assert.Equal(t, float64(20), i.avg)

// 	i.AddEgress(10)
// 	assert.Equal(t, float64(15), i.avg)

// 	i.samples = 0
// 	i.avg = 0

// 	i.AddIngress(10)
// 	i.AddIngress(10)
// 	i.AddIngress(10)
// 	i.AddIngress(10)
// 	assert.Equal(t, float64(10), i.avg)

// }
