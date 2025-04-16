// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fixtures contains example responses from Versa API for testing
package fixtures

// GetSLAMetrics /versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SDWAN
const GetSLAMetrics = `
{
    "qTime": 1,
    "sEcho": 0,
    "iTotalDisplayRecords": 1,
    "iTotalRecords": 1,
    "aaData": [
        [
            "test-branch-2B,Controller-2,INET-1,INET-1,fc_nc",
            "test-branch-2B",
            "Controller-2",
            "INET-1",
            "INET-1",
            "fc_nc",
            101.0,
            0.0,
            0.0,
            0.0,
            0.0,
            0.0
        ]
    ]
}
`
