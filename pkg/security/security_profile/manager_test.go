// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"errors"
	"math/rand"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
)

// Old activity dump manager unit tests

func compareListOfDumps(t *testing.T, out, expectedOut []*dump.ActivityDump) {
	for _, elem := range out {
		var found bool
		for _, expected := range expectedOut {
			if elem.Profile.Metadata.Name == expected.Profile.Metadata.Name {
				found = true
			}
		}

		assert.Truef(t, found, "output didn't match: dump %s should not be in the output", elem.Profile.Metadata.Name)
	}
	for _, elem := range expectedOut {
		var found bool
		for _, got := range out {
			if elem.Profile.Metadata.Name == got.Profile.Metadata.Name {
				found = true
			}
		}

		assert.Truef(t, found, "output didn't match: dump %s is missing from the output", elem.Profile.Metadata.Name)
	}
}

func TestActivityDumpManager_getExpiredDumps(t *testing.T) {
	type fields struct {
		activeDumps []*dump.ActivityDump
	}
	tests := []struct {
		name         string
		fields       fields
		expiredDumps []*dump.ActivityDump
		activeDumps  []*dump.ActivityDump
	}{
		{
			"no_dump",
			fields{},
			[]*dump.ActivityDump{},
			[]*dump.ActivityDump{},
		},
		{
			"one_dump/one_expired_dump",
			fields{
				activeDumps: []*dump.ActivityDump{
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "1",
							End:  time.Now().Add(-time.Minute),
						},
					}},
				},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "1",
						End:  time.Now().Add(-time.Minute),
					},
				}},
			},
			[]*dump.ActivityDump{},
		},
		{
			"one_dump/no_expired_dump",
			fields{
				activeDumps: []*dump.ActivityDump{
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "1",
							End:  time.Now().Add(time.Minute),
						},
					}},
				},
			},
			[]*dump.ActivityDump{},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "1",
						End:  time.Now().Add(time.Minute),
					},
				}},
			},
		},
		{
			"5_dumps/no_expired_dump",
			fields{
				activeDumps: []*dump.ActivityDump{
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "1",
							End:  time.Now().Add(time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "2",
							End:  time.Now().Add(time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "3",
							End:  time.Now().Add(time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "4",
							End:  time.Now().Add(time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "5",
							End:  time.Now().Add(time.Minute),
						},
					}},
				},
			},
			[]*dump.ActivityDump{},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "1",
						End:  time.Now().Add(time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "2",
						End:  time.Now().Add(time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "3",
						End:  time.Now().Add(time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "4",
						End:  time.Now().Add(time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "5",
						End:  time.Now().Add(time.Minute),
					},
				}},
			},
		},
		{
			"5_dumps/5_expired_dumps",
			fields{
				activeDumps: []*dump.ActivityDump{
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "1",
							End:  time.Now().Add(-time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "2",
							End:  time.Now().Add(-time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "3",
							End:  time.Now().Add(-time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "4",
							End:  time.Now().Add(-time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "5",
							End:  time.Now().Add(-time.Minute),
						},
					}},
				},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "1",
						End:  time.Now().Add(-time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "2",
						End:  time.Now().Add(-time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "3",
						End:  time.Now().Add(-time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "4",
						End:  time.Now().Add(-time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "5",
						End:  time.Now().Add(-time.Minute),
					},
				}},
			},
			[]*dump.ActivityDump{},
		},
		{
			"5_dumps/2_expired_dumps",
			fields{
				activeDumps: []*dump.ActivityDump{
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "1",
							End:  time.Now().Add(time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "2",
							End:  time.Now().Add(-time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "3",
							End:  time.Now().Add(time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "4",
							End:  time.Now().Add(-time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "5",
							End:  time.Now().Add(time.Minute),
						},
					}},
				},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "2",
						End:  time.Now().Add(-time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "4",
						End:  time.Now().Add(-time.Minute),
					},
				}},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "1",
						End:  time.Now().Add(time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "3",
						End:  time.Now().Add(time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "5",
						End:  time.Now().Add(time.Minute),
					},
				}},
			},
		},
		{
			"5_dumps/2_expired_dumps_at_the_start",
			fields{
				activeDumps: []*dump.ActivityDump{
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "1",
							End:  time.Now().Add(-time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "2",
							End:  time.Now().Add(-time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "3",
							End:  time.Now().Add(time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "4",
							End:  time.Now().Add(time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "5",
							End:  time.Now().Add(time.Minute),
						},
					}},
				},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "1",
						End:  time.Now().Add(-time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "2",
						End:  time.Now().Add(-time.Minute),
					},
				}},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "3",
						End:  time.Now().Add(time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "4",
						End:  time.Now().Add(time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "5",
						End:  time.Now().Add(time.Minute),
					},
				}},
			},
		},
		{
			"5_dumps/2_expired_dumps_at_the_end",
			fields{
				activeDumps: []*dump.ActivityDump{
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "1",
							End:  time.Now().Add(time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "2",
							End:  time.Now().Add(time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "3",
							End:  time.Now().Add(time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "4",
							End:  time.Now().Add(-time.Minute),
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{
							Name: "5",
							End:  time.Now().Add(-time.Minute),
						},
					}},
				},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "4",
						End:  time.Now().Add(-time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "5",
						End:  time.Now().Add(-time.Minute),
					},
				}},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "1",
						End:  time.Now().Add(time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "2",
						End:  time.Now().Add(time.Minute),
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{
						Name: "3",
						End:  time.Now().Add(time.Minute),
					},
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, ad := range tt.fields.activeDumps {
				ad.SetState(dump.Running)
			}

			adm := &Manager{
				activeDumps:        tt.fields.activeDumps,
				ignoreFromSnapshot: make(map[uint64]bool),
			}

			expiredDumps := adm.getExpiredDumps()
			for _, ad := range expiredDumps {
				ad.SetState(dump.Stopped)
			}

			compareListOfDumps(t, expiredDumps, tt.expiredDumps)
			compareListOfDumps(t, adm.activeDumps, tt.activeDumps)
		})
	}
}

func TestActivityDumpManager_getOverweightDumps(t *testing.T) {
	type fields struct {
		activeDumps []*dump.ActivityDump
	}
	tests := []struct {
		name            string
		fields          fields
		overweightDumps []*dump.ActivityDump
		activeDumps     []*dump.ActivityDump
	}{
		{
			"no_dump",
			fields{},
			[]*dump.ActivityDump{},
			[]*dump.ActivityDump{},
		},
		{
			"one_dump/one_overweight_dump",
			fields{
				activeDumps: []*dump.ActivityDump{
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "1"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{ProcessNodes: 2},
						},
					}},
				},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "1"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{ProcessNodes: 2},
					},
				}},
			},
			[]*dump.ActivityDump{},
		},
		{
			"one_dump/no_overweight_dump",
			fields{
				activeDumps: []*dump.ActivityDump{
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "1"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{},
						},
					}},
				},
			},
			[]*dump.ActivityDump{},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "1"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					},
				}},
			},
		},
		{
			"5_dumps/no_overweight_dump",
			fields{
				activeDumps: []*dump.ActivityDump{
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "1"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "2"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "3"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "4"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "5"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{},
						},
					}},
				},
			},
			[]*dump.ActivityDump{},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "1"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "2"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "3"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "4"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "5"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					},
				}},
			},
		},
		{
			"5_dumps/5_overweight_dumps",
			fields{
				activeDumps: []*dump.ActivityDump{
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "1"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{
								ProcessNodes: 2,
							},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "2"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{
								ProcessNodes: 3,
							},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "3"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{
								ProcessNodes: 2,
							},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "4"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{
								ProcessNodes: 3,
							},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "5"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{
								ProcessNodes: 2,
							},
						},
					}},
				},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "1"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{
							ProcessNodes: 2,
						},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "2"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{
							ProcessNodes: 3,
						},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "3"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{
							ProcessNodes: 2,
						},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "4"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{
							ProcessNodes: 3,
						},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "5"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{
							ProcessNodes: 2,
						},
					},
				}},
			},
			[]*dump.ActivityDump{},
		},
		{
			"5_dumps/2_expired_dumps",
			fields{
				activeDumps: []*dump.ActivityDump{
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "1"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "2"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{
								ProcessNodes: 3,
							},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "3"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "4"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{
								ProcessNodes: 2,
							},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "5"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{},
						},
					}},
				},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "2"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{
							ProcessNodes: 3,
						},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "4"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{
							ProcessNodes: 2,
						},
					},
				}},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "1"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "3"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "5"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					},
				}},
			},
		},
		{
			"5_dumps/2_expired_dumps_at_the_start",
			fields{
				activeDumps: []*dump.ActivityDump{
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "1"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{
								ProcessNodes: 3,
							},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "2"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{
								ProcessNodes: 2,
							},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "3"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "4"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "5"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{},
						},
					}},
				},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "1"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{
							ProcessNodes: 3,
						},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "2"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{
							ProcessNodes: 2,
						},
					},
				}},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "3"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "4"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "5"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					},
				}},
			},
		},
		{
			"5_dumps/2_expired_dumps_at_the_end",
			fields{
				activeDumps: []*dump.ActivityDump{
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "1"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "2"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "3"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "4"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{
								ProcessNodes: 3,
							},
						},
					}},
					{Profile: &profile.Profile{
						Metadata: mtdt.Metadata{Name: "5"},
						ActivityTree: &activity_tree.ActivityTree{
							Stats: &activity_tree.Stats{
								ProcessNodes: 2,
							},
						},
					}},
				},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "4"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{
							ProcessNodes: 3,
						},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "5"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{
							ProcessNodes: 2,
						},
					},
				}},
			},
			[]*dump.ActivityDump{
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "1"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "2"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					},
				}},
				{Profile: &profile.Profile{
					Metadata: mtdt.Metadata{Name: "3"},
					ActivityTree: &activity_tree.ActivityTree{
						Stats: &activity_tree.Stats{},
					},
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adm := &Manager{
				activeDumps: tt.fields.activeDumps,
				config: &config.Config{
					RuntimeSecurity: &config.RuntimeSecurityConfig{
						ActivityDumpMaxDumpSize: func() int {
							return 2048
						},
					},
				},
				statsdClient:       &statsd.NoOpClient{},
				ignoreFromSnapshot: make(map[uint64]bool),
			}

			compareListOfDumps(t, adm.getOverweightDumps(), tt.overweightDumps)
			compareListOfDumps(t, adm.activeDumps, tt.activeDumps)
		})
	}
}

// Old security profile manager unit tests

type testIteration struct {
	name                string                           // test name
	result              model.EventFilteringProfileState // expected result
	newProfile          bool                             // true if a new profile have to be generated for this test
	containerCreatedAt  time.Duration                    // time diff from t0
	addFakeProcessNodes int64                            // number of fake process nodes to add (adds 1024 to approx size)
	eventTimestampRaw   time.Duration                    // time diff from t0
	eventType           model.EventType                  // only exec for now, TODO: add dns
	eventProcessPath    string                           // exec path
	eventDNSReq         string                           // dns request name (only for eventType == DNSEventType)
	loopUntil           time.Duration                    // if not 0, will loop until the given duration is reached
	loopIncrement       time.Duration                    // if loopUntil is not 0, will increment this duration at each loop
}

func craftFakeEvent(t0 time.Time, ti *testIteration, defaultContainerID string) *model.Event {
	event := model.NewFakeEvent()
	event.Type = uint32(ti.eventType)
	event.TimestampRaw = uint64(t0.Add(ti.eventTimestampRaw).UnixNano())
	event.Timestamp = t0.Add(ti.eventTimestampRaw)

	// setting process
	event.ProcessCacheEntry = model.NewPlaceholderProcessCacheEntry(42, 42, false)
	event.ProcessCacheEntry.ContainerContext.ContainerID = containerutils.ContainerID(defaultContainerID)
	event.ProcessCacheEntry.FileEvent.PathnameStr = ti.eventProcessPath
	event.ProcessCacheEntry.FileEvent.Inode = 42
	event.ProcessCacheEntry.Args = "foo"
	event.ProcessContext = &event.ProcessCacheEntry.ProcessContext
	event.ProcessContext.Process.ContainerContext.CreatedAt = uint64(t0.Add(ti.containerCreatedAt).UnixNano())
	switch ti.eventType {
	case model.ExecEventType:
		event.Exec.Process = &event.ProcessCacheEntry.ProcessContext.Process
	case model.DNSEventType:
		event.DNS.Question.Name = ti.eventDNSReq
		event.DNS.Question.Type = 1  // A
		event.DNS.Question.Class = 1 // INET
		event.DNS.Question.Size = uint16(len(ti.eventDNSReq))
		event.DNS.Question.Count = 1
	}

	// setting process ancestor
	event.ProcessCacheEntry.Ancestor = model.NewPlaceholderProcessCacheEntry(1, 1, false)
	event.ProcessCacheEntry.Ancestor.FileEvent.PathnameStr = "systemd"
	event.ProcessCacheEntry.Ancestor.FileEvent.Inode = 41
	event.ProcessCacheEntry.Ancestor.Args = "foo"
	return event
}

func TestSecurityProfileManager_tryAutolearn(t *testing.T) {
	AnomalyDetectionMinimumStablePeriod := time.Hour
	AnomalyDetectionWorkloadWarmupPeriod := time.Minute
	AnomalyDetectionUnstableProfileTimeThreshold := time.Hour * 48
	MaxNbProcess := int64(1000)
	AnomalyDetectionUnstableProfileSizeThreshold := int64(unsafe.Sizeof(activity_tree.ProcessNode{})) * MaxNbProcess
	defaultContainerID := "424242424242424242424242424242424242424242424242424242424242424"

	tests := []testIteration{
		// checking warmup period for exec:
		{
			name:                "warmup-exec/not-warmup",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "warmup-exec/warmup",
			result:              model.WorkloadWarmup,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod + time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},

		// and for dns:
		{
			name:                "warmup-dns/insert-dns-process",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "warmup-dns/not-warmup",
			result:              model.AutoLearning,
			newProfile:          false,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo0",
			eventDNSReq:         "foo.bar",
		},
		{
			name:                "warmup-dns/insert-dns-process2",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "warmup-dns/warmup",
			result:              model.WorkloadWarmup,
			newProfile:          false,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod + time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo0",
			eventDNSReq:         "foo.baz",
		},

		// dont insert dns process if not already present:
		{
			name:                "dont-insert-dns-process/add-first-exec-event",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "dont-insert-dns-process/wait-stable-period",
			result:              model.StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second*2 + AnomalyDetectionMinimumStablePeriod,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},
		{
			name:                "dont-insert-dns-process/reject-dns-process",
			result:              model.AutoLearning,
			newProfile:          false,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/bar",
			eventDNSReq:         "foo.baz",
		},

		// checking stable period for exec:
		{
			name:                "stable-exec/add-first-event",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "stable-exec/add-second-event",
			result:              model.AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second * 2,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},
		{
			name:                "stable-exec/wait-stable-period",
			result:              model.StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second*2 + AnomalyDetectionMinimumStablePeriod,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo2",
		},
		{
			name:                "stable-exec/still-stable",
			result:              model.StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second*3 + AnomalyDetectionMinimumStablePeriod,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo3",
		},
		{
			name:                "stable-exec/dont-get-unstable",
			result:              model.StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Minute,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo4",
		},
		{
			name:                "stable-exec/meanwhile-dns-still-learning",
			result:              model.AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo0",
			eventDNSReq:         "foo0.bar",
		},
		// and for dns:
		{
			name:                "stable-dns/insert-dns-process",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo",
		},
		{
			name:                "stable-dns/add-first-event",
			result:              model.AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		{
			name:                "stable-dns/add-second-event",
			result:              model.AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second * 2,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo1.bar",
		},
		{
			name:                "stable-dns/wait-stable-period",
			result:              model.StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second*2 + AnomalyDetectionMinimumStablePeriod,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo2.bar",
		},
		{
			name:                "stable-dns/still-stable",
			result:              model.StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second*3 + AnomalyDetectionMinimumStablePeriod,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo3.bar",
		},
		{
			name:                "stable-dns/dont-get-unstable",
			result:              model.StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Minute,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo4.bar",
		},
		{
			name:                "stable-dns/meanwhile-exec-still-learning",
			result:              model.AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},

		// checking unstable period for exec:
		{
			name:                "unstable-exec/insert-dns-process",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "unstable-exec/wait-unstable-period",
			result:              model.AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo_",
			loopUntil:           AnomalyDetectionUnstableProfileTimeThreshold - time.Second,
			loopIncrement:       AnomalyDetectionMinimumStablePeriod - time.Second,
		},
		{
			name:                "unstable-exec/still-unstable",
			result:              model.UnstableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},
		{
			name:                "unstable-exec/still-unstable-after-stable-period",
			result:              model.UnstableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + AnomalyDetectionMinimumStablePeriod + time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo2",
		},
		{
			name:                "unstable-exec/meanwhile-dns-still-learning",
			result:              model.AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo0",
			eventDNSReq:         "foo0.bar",
		},
		// and for dns:
		{
			name:                "unstable-dns/insert-dns-process",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo",
		},
		{
			name:                "unstable-dns/wait-unstable-period",
			result:              model.AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         ".foo.bar",
			loopUntil:           AnomalyDetectionUnstableProfileTimeThreshold - time.Second,
			loopIncrement:       AnomalyDetectionMinimumStablePeriod - time.Second,
		},
		{
			name:                "unstable-dns/still-unstable",
			result:              model.UnstableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo1.bar",
		},
		{
			name:                "unstable-dns/still-unstable-after-stable-period",
			result:              model.UnstableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + AnomalyDetectionMinimumStablePeriod + time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo2.bar",
		},
		{
			name:                "unstable-dns/meanwhile-exec-still-learning",
			result:              model.AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},

		// checking max size threshold different cases for exec:
		{
			name:                "profile-at-max-size-exec/add-first-event",
			result:              model.WorkloadWarmup,
			newProfile:          true,
			containerCreatedAt:  0,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "profile-at-max-size-exec/warmup-stable",
			result:              model.WorkloadWarmup,
			newProfile:          false,
			containerCreatedAt:  0,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "profile-at-max-size-exec/warmup-unstable",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  0,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},
		{
			name:                "profile-at-max-size-exec/stable",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "profile-at-max-size-exec/unstable",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},
		{
			name:                "profile-at-max-size-exec/warmup-NOT-at-max-size",
			result:              model.WorkloadWarmup,
			newProfile:          true,
			containerCreatedAt:  0,
			addFakeProcessNodes: MaxNbProcess - 1,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo2",
		},
		{
			name:                "profile-at-max-size-exec/NOT-at-max-size",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess - 1,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo2",
		},
		// and for dns:
		{
			name:                "profile-at-max-size-dns/insert-dns-process",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo",
		},
		{
			name:                "profile-at-max-size-dns/add-first-event",
			result:              model.WorkloadWarmup,
			newProfile:          false,
			containerCreatedAt:  0,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		{
			name:                "profile-at-max-size-dns/warmup-stable",
			result:              model.WorkloadWarmup,
			newProfile:          false,
			containerCreatedAt:  0,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		{
			name:                "profile-at-max-size-dns/warmup-unstable",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  0,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo1.bar",
		},
		{
			name:                "profile-at-max-size-dns/stable",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		{
			name:                "profile-at-max-size-dns/unstable",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo1.bar",
		},
		{
			name:                "profile-at-max-size-dns/insert-dns-process2",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo",
		},
		{
			name:                "profile-at-max-size-dns/warmup-NOT-at-max-size",
			result:              model.WorkloadWarmup,
			newProfile:          false,
			containerCreatedAt:  0,
			addFakeProcessNodes: MaxNbProcess - 2,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo2.bar",
		},
		{
			name:                "profile-at-max-size-dns/insert-dns-process3",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo",
		},
		{
			name:                "profile-at-max-size-dns/NOT-at-max-size",
			result:              model.AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess - 2,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo2.bar",
		},

		// checking from max-size to stable, for exec:
		{
			name:                "profile-at-max-size-to-stable-exec/insert-dns-process",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo",
		},
		{
			name:                "profile-at-max-size-to-stable-exec/max-size",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "profile-at-max-size-to-stable-exec/stable",
			result:              model.StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},
		{
			name:                "profile-at-max-size-to-stable-exec/meanwhile-dns-still-at-max-size",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		// and for dns:
		{
			name:                "profile-at-max-size-to-stable-dns/insert-dns-process",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo",
		},
		{
			name:                "profile-at-max-size-to-stable-dns/max-size",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		{
			name:                "profile-at-max-size-to-stable-dns/stable",
			result:              model.StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo1.bar",
		},
		{
			name:                "profile-at-max-size-to-stable-dns/meanwhile-exec-still-at-max-size",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},

		// from checking max-size to unstable for exec:
		{
			name:                "profile-at-max-size-to-unstable-exec/insert-dns-process",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo",
		},
		{
			name:                "profile-at-max-size-to-unstable-exec/max-size",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "profile-at-max-size-to-unstable-exec/unstable",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo_",
			loopUntil:           AnomalyDetectionUnstableProfileTimeThreshold - time.Second,
			loopIncrement:       AnomalyDetectionMinimumStablePeriod - time.Second,
		},
		{
			name:                "profile-at-max-size-to-unstable-exec/unstable",
			result:              model.UnstableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},
		{
			name:                "profile-at-max-size-to-unstable-exec/meanwhile-dns-still-at-max-size",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		// and for dns:
		{
			name:                "profile-at-max-size-to-unstable-dns/insert-dns-process",
			result:              model.AutoLearning,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo",
		},
		{
			name:                "profile-at-max-size-to-unstable-dns/insert-dns-process2",
			result:              model.AutoLearning,
			newProfile:          false,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "profile-at-max-size-to-unstable-dns/max-size",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo0",
			eventDNSReq:         "foo0.bar",
		},
		{
			name:                "profile-at-max-size-to-unstable-dns/unstable",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         ".foo.bar",
			loopUntil:           AnomalyDetectionUnstableProfileTimeThreshold - time.Second,
			loopIncrement:       AnomalyDetectionMinimumStablePeriod - time.Second,
		},
		{
			name:                "profile-at-max-size-to-unstable-dns/unstable",
			result:              model.UnstableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo1.bar",
		},
		{
			name:                "profile-at-max-size-to-unstable-dns/meanwhile-exec-still-at-max-size",
			result:              model.ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},
	}

	// Initial time reference
	t0 := time.Now()

	// secprofile manager, only use for config and stats
	spm := &Manager{
		eventFiltering: make(map[eventFilteringEntry]*atomic.Uint64),
		config: &config.Config{
			RuntimeSecurity: &config.RuntimeSecurityConfig{
				AnomalyDetectionDefaultMinimumStablePeriod:   AnomalyDetectionMinimumStablePeriod,
				AnomalyDetectionWorkloadWarmupPeriod:         AnomalyDetectionWorkloadWarmupPeriod,
				AnomalyDetectionUnstableProfileTimeThreshold: AnomalyDetectionUnstableProfileTimeThreshold,
				AnomalyDetectionUnstableProfileSizeThreshold: AnomalyDetectionUnstableProfileSizeThreshold,
			},
		},
	}
	spm.initMetricsMap()

	var secprof *profile.Profile
	for _, ti := range tests {
		t.Run(ti.name, func(t *testing.T) {
			if ti.newProfile || secprof == nil {
				secprof = profile.New(
					profile.WithWorkloadSelector(cgroupModel.WorkloadSelector{Image: "image", Tag: "tag"}),
					profile.WithEventTypes([]model.EventType{model.ExecEventType, model.DNSEventType}),
				)
				secprof.ActivityTree = activity_tree.NewActivityTree(secprof, nil, "security_profile")
				cgce := cgroupModel.NewCacheEntry(model.ContainerContext{ContainerID: containerutils.ContainerID(defaultContainerID)}, model.CGroupContext{CGroupID: containerutils.CGroupID(defaultContainerID)}, 0)
				secprof.Instances = append(secprof.Instances, &tags.Workload{
					GCroupCacheEntry: cgce,
					Selector:         cgroupModel.WorkloadSelector{Image: "image", Tag: "tag"},
				})
				secprof.LoadedNano.Store(uint64(t0.UnixNano()))
			}
			secprof.ActivityTree.Stats.ProcessNodes += ti.addFakeProcessNodes
			ctx := secprof.GetVersionContextIndex(0)
			if ctx == nil {
				t.Fatal(errors.New("profile should have one ctx"))
			}
			ctx.FirstSeenNano = uint64(t0.Add(ti.containerCreatedAt).UnixNano())

			if ti.loopUntil != 0 {
				currentIncrement := time.Duration(0)
				basePath := ti.eventProcessPath
				baseDNSReq := ti.eventDNSReq
				for currentIncrement < ti.loopUntil {
					if ti.eventType == model.ExecEventType {
						ti.eventProcessPath = basePath + strconv.Itoa(rand.Int())
					} else if ti.eventType == model.DNSEventType {
						ti.eventDNSReq = strconv.Itoa(rand.Int()) + baseDNSReq
					}
					ti.eventTimestampRaw = currentIncrement
					event := craftFakeEvent(t0, &ti, defaultContainerID)
					assert.Equal(t, ti.result, spm.tryAutolearn(secprof, ctx, event, "tag"))
					currentIncrement += ti.loopIncrement
				}
			} else { // only run once
				event := craftFakeEvent(t0, &ti, defaultContainerID)
				assert.Equal(t, ti.result, spm.tryAutolearn(secprof, ctx, event, "tag"))
			}

			// TODO: also check profile stats and global metrics
		})
	}
}
