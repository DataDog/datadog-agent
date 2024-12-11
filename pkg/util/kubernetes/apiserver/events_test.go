// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDiffEvents(t *testing.T) {
	// This method is called when the RV used to watch is too old (server response)
	// We List all events and only send the ones with a RV superior to the one we have (e.g. newer events).

	for n, tc := range []struct {
		caseName   string
		listEvents []*v1.Event
		resver     int
		expected   []*v1.Event
	}{

		{
			caseName: "2 new events",
			listEvents: []*v1.Event{{
				Reason: "OOM",
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "43",
				},
			},
				{
					Reason: "Create",
					ObjectMeta: metav1.ObjectMeta{
						ResourceVersion: "52",
					},
				},
				{
					Reason: "Scheduled",
					ObjectMeta: metav1.ObjectMeta{
						ResourceVersion: "21",
					},
				},
			},
			resver: 22,
			expected: []*v1.Event{{
				Reason: "OOM",
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "43",
				},
			},
				{
					Reason: "Create",
					ObjectMeta: metav1.ObjectMeta{
						ResourceVersion: "52",
					},
				},
			},
		},
		{
			caseName: "all new events",
			listEvents: []*v1.Event{{
				Reason: "OOM",
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "43",
				},
			},
				{
					Reason: "Create",
					ObjectMeta: metav1.ObjectMeta{
						ResourceVersion: "52",
					},
				},
				{
					Reason: "Scheduled",
					ObjectMeta: metav1.ObjectMeta{
						ResourceVersion: "21",
					},
				},
			},
			resver: 15,
			expected: []*v1.Event{{
				Reason: "OOM",
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "43",
				},
			},
				{
					Reason: "Create",
					ObjectMeta: metav1.ObjectMeta{
						ResourceVersion: "52",
					},
				},
				{
					Reason: "Scheduled",
					ObjectMeta: metav1.ObjectMeta{
						ResourceVersion: "21",
					},
				},
			},
		},
		{
			caseName: "no new events",
			listEvents: []*v1.Event{
				{
					Reason: "OOM",
					ObjectMeta: metav1.ObjectMeta{
						ResourceVersion: "43",
					},
				},
				{
					Reason: "Create",
					ObjectMeta: metav1.ObjectMeta{
						ResourceVersion: "52",
					},
				},
				{
					Reason: "Scheduled",
					ObjectMeta: metav1.ObjectMeta{
						ResourceVersion: "21",
					},
				},
			},
			resver:   52,
			expected: []*v1.Event{},
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", n, tc.caseName), func(t *testing.T) {
			diffList := diffEvents(tc.resver, tc.listEvents)
			assert.Equal(t, len(tc.expected), len(diffList))
		})
	}
}
