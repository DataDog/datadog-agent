// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package py

//PythonStatsEntry are entries for specific object type memory usage
type PythonStatsEntry struct {
	Reference string
	NObjects  int
	Size      int
}

//PythonStats contains python memory statistics
type PythonStats struct {
	Type     string
	NObjects int
	Size     int
	Entries  []*PythonStatsEntry
}
