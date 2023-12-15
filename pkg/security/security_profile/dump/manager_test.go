// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dump holds dump related files
package dump

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
)

func compareListOfDumps(t *testing.T, out, expectedOut []*ActivityDump) {
	for _, elem := range out {
		var found bool
		for _, expected := range expectedOut {
			if elem.Name == expected.Name {
				found = true
			}
		}

		assert.Truef(t, found, "output didn't match: dump %s should not be in the output", elem.Name)
	}
	for _, elem := range expectedOut {
		var found bool
		for _, got := range out {
			if elem.Name == got.Name {
				found = true
			}
		}

		assert.Truef(t, found, "output didn't match: dump %s is missing from the output", elem.Name)
	}
}

func TestActivityDumpManager_getExpiredDumps(t *testing.T) {
	type fields struct {
		activeDumps []*ActivityDump
	}
	tests := []struct {
		name         string
		fields       fields
		expiredDumps []*ActivityDump
		activeDumps  []*ActivityDump
	}{
		{
			"no_dump",
			fields{},
			[]*ActivityDump{},
			[]*ActivityDump{},
		},
		{
			"one_dump/one_expired_dump",
			fields{
				activeDumps: []*ActivityDump{
					{Metadata: mtdt.Metadata{Name: "1", End: time.Now().Add(-time.Minute)}},
				},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "1", End: time.Now().Add(-time.Minute)}},
			},
			[]*ActivityDump{},
		},
		{
			"one_dump/no_expired_dump",
			fields{
				activeDumps: []*ActivityDump{
					{Metadata: mtdt.Metadata{Name: "1", End: time.Now().Add(time.Minute)}},
				},
			},
			[]*ActivityDump{},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "1", End: time.Now().Add(time.Minute)}},
			},
		},
		{
			"5_dumps/no_expired_dump",
			fields{
				activeDumps: []*ActivityDump{
					{Metadata: mtdt.Metadata{Name: "1", End: time.Now().Add(time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "2", End: time.Now().Add(time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "3", End: time.Now().Add(time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "4", End: time.Now().Add(time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "5", End: time.Now().Add(time.Minute)}},
				},
			},
			[]*ActivityDump{},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "1", End: time.Now().Add(time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "2", End: time.Now().Add(time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "3", End: time.Now().Add(time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "4", End: time.Now().Add(time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "5", End: time.Now().Add(time.Minute)}},
			},
		},
		{
			"5_dumps/5_expired_dumps",
			fields{
				activeDumps: []*ActivityDump{
					{Metadata: mtdt.Metadata{Name: "1", End: time.Now().Add(-time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "2", End: time.Now().Add(-time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "3", End: time.Now().Add(-time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "4", End: time.Now().Add(-time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "5", End: time.Now().Add(-time.Minute)}},
				},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "1", End: time.Now().Add(-time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "2", End: time.Now().Add(-time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "3", End: time.Now().Add(-time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "4", End: time.Now().Add(-time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "5", End: time.Now().Add(-time.Minute)}},
			},
			[]*ActivityDump{},
		},
		{
			"5_dumps/2_expired_dumps",
			fields{
				activeDumps: []*ActivityDump{
					{Metadata: mtdt.Metadata{Name: "1", End: time.Now().Add(time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "2", End: time.Now().Add(-time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "3", End: time.Now().Add(time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "4", End: time.Now().Add(-time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "5", End: time.Now().Add(time.Minute)}},
				},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "2", End: time.Now().Add(-time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "4", End: time.Now().Add(-time.Minute)}},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "1", End: time.Now().Add(time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "3", End: time.Now().Add(time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "5", End: time.Now().Add(time.Minute)}},
			},
		},
		{
			"5_dumps/2_expired_dumps_at_the_start",
			fields{
				activeDumps: []*ActivityDump{
					{Metadata: mtdt.Metadata{Name: "1", End: time.Now().Add(-time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "2", End: time.Now().Add(-time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "3", End: time.Now().Add(time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "4", End: time.Now().Add(time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "5", End: time.Now().Add(time.Minute)}},
				},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "1", End: time.Now().Add(-time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "2", End: time.Now().Add(-time.Minute)}},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "3", End: time.Now().Add(time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "4", End: time.Now().Add(time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "5", End: time.Now().Add(time.Minute)}},
			},
		},
		{
			"5_dumps/2_expired_dumps_at_the_end",
			fields{
				activeDumps: []*ActivityDump{
					{Metadata: mtdt.Metadata{Name: "1", End: time.Now().Add(time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "2", End: time.Now().Add(time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "3", End: time.Now().Add(time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "4", End: time.Now().Add(-time.Minute)}},
					{Metadata: mtdt.Metadata{Name: "5", End: time.Now().Add(-time.Minute)}},
				},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "4", End: time.Now().Add(-time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "5", End: time.Now().Add(-time.Minute)}},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "1", End: time.Now().Add(time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "2", End: time.Now().Add(time.Minute)}},
				{Metadata: mtdt.Metadata{Name: "3", End: time.Now().Add(time.Minute)}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adm := &ActivityDumpManager{
				activeDumps:        tt.fields.activeDumps,
				ignoreFromSnapshot: make(map[string]bool),
			}

			compareListOfDumps(t, adm.getExpiredDumps(), tt.expiredDumps)
			compareListOfDumps(t, adm.activeDumps, tt.activeDumps)
		})
	}
}

func TestActivityDumpManager_getOverweightDumps(t *testing.T) {
	type fields struct {
		activeDumps []*ActivityDump
	}
	tests := []struct {
		name            string
		fields          fields
		overweightDumps []*ActivityDump
		activeDumps     []*ActivityDump
	}{
		{
			"no_dump",
			fields{},
			[]*ActivityDump{},
			[]*ActivityDump{},
		},
		{
			"one_dump/one_overweight_dump",
			fields{
				activeDumps: []*ActivityDump{
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "1"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{ProcessNodes: 2},
					}},
				},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "1"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{ProcessNodes: 2},
				}},
			},
			[]*ActivityDump{},
		},
		{
			"one_dump/no_overweight_dump",
			fields{
				activeDumps: []*ActivityDump{
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "1"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					}},
				},
			},
			[]*ActivityDump{},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "1"}},
			},
		},
		{
			"5_dumps/no_overweight_dump",
			fields{
				activeDumps: []*ActivityDump{
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "1"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "2"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "3"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "4"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "5"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					}},
				},
			},
			[]*ActivityDump{},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "1"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{},
				}},
				{Metadata: mtdt.Metadata{Name: "2"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{},
				}},
				{Metadata: mtdt.Metadata{Name: "3"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{},
				}},
				{Metadata: mtdt.Metadata{Name: "4"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{},
				}},
				{Metadata: mtdt.Metadata{Name: "5"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{},
				}},
			},
		},
		{
			"5_dumps/5_overweight_dumps",
			fields{
				activeDumps: []*ActivityDump{
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "1"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{ProcessNodes: 2},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "2"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{ProcessNodes: 3},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "3"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{ProcessNodes: 2},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "4"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{ProcessNodes: 3},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "5"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{ProcessNodes: 2},
					}},
				},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "1"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{ProcessNodes: 2},
				}},
				{Metadata: mtdt.Metadata{Name: "2"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{ProcessNodes: 3},
				}},
				{Metadata: mtdt.Metadata{Name: "3"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{ProcessNodes: 2},
				}},
				{Metadata: mtdt.Metadata{Name: "4"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{ProcessNodes: 3},
				}},
				{Metadata: mtdt.Metadata{Name: "5"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{ProcessNodes: 2},
				}},
			},
			[]*ActivityDump{},
		},
		{
			"5_dumps/2_expired_dumps",
			fields{
				activeDumps: []*ActivityDump{
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "1"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "2"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{ProcessNodes: 3},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "3"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "4"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{ProcessNodes: 2},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "5"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					}},
				},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "2"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{ProcessNodes: 3},
				}},
				{Metadata: mtdt.Metadata{Name: "4"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{ProcessNodes: 2},
				}},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "1"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{},
				}},
				{Metadata: mtdt.Metadata{Name: "3"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{},
				}},
				{Metadata: mtdt.Metadata{Name: "5"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{},
				}},
			},
		},
		{
			"5_dumps/2_expired_dumps_at_the_start",
			fields{
				activeDumps: []*ActivityDump{
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "1"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{ProcessNodes: 3},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "2"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{ProcessNodes: 2},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "3"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "4"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "5"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					}},
				},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "1"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{ProcessNodes: 3},
				}},
				{Metadata: mtdt.Metadata{Name: "2"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{ProcessNodes: 2},
				}},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "3", End: time.Now().Add(time.Minute)}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{},
				}},
				{Metadata: mtdt.Metadata{Name: "4", End: time.Now().Add(time.Minute)}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{},
				}},
				{Metadata: mtdt.Metadata{Name: "5", End: time.Now().Add(time.Minute)}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{},
				}},
			},
		},
		{
			"5_dumps/2_expired_dumps_at_the_end",
			fields{
				activeDumps: []*ActivityDump{
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "1"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "2"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "3"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "4"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{ProcessNodes: 3},
					}},
					{Mutex: &sync.Mutex{}, Metadata: mtdt.Metadata{Name: "5"}, ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{ProcessNodes: 2},
					}},
				},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "4"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{ProcessNodes: 3},
				}},
				{Metadata: mtdt.Metadata{Name: "5"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{ProcessNodes: 2},
				}},
			},
			[]*ActivityDump{
				{Metadata: mtdt.Metadata{Name: "1"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{},
				}},
				{Metadata: mtdt.Metadata{Name: "2"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{},
				}},
				{Metadata: mtdt.Metadata{Name: "3"}, ActivityTree: &activity_tree.ActivityTree{
					Stats: &activity_tree.Stats{},
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adm := &ActivityDumpManager{
				activeDumps: tt.fields.activeDumps,
				config: &config.Config{
					RuntimeSecurity: &config.RuntimeSecurityConfig{
						ActivityDumpMaxDumpSize: func() int {
							return 2048
						},
					},
				},
				statsdClient:       &statsd.NoOpClient{},
				ignoreFromSnapshot: make(map[string]bool),
			}

			compareListOfDumps(t, adm.getOverweightDumps(), tt.overweightDumps)
			compareListOfDumps(t, adm.activeDumps, tt.activeDumps)
		})
	}
}
