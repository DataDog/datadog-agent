// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package checkmetadata

/*
The payload is a sequence of name-value pairs.
*/

// Payload handles the JSON unmarshalling of the check metadata payload
type Payload [][2]string
