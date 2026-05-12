// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
)

type fakeConditionalWalkSession struct {
	responses []*gosnmp.SnmpPacket
	requests  [][]string
	logs      []string
	maxReps   uint32
}

func (s *fakeConditionalWalkSession) getBulk(oids []string, _ uint8, _ uint32) (*gosnmp.SnmpPacket, error) {
	s.requests = append(s.requests, append([]string(nil), oids...))
	return s.nextResponse(), nil
}

func (s *fakeConditionalWalkSession) getNext(oids []string) (*gosnmp.SnmpPacket, error) {
	s.requests = append(s.requests, append([]string(nil), oids...))
	return s.nextResponse(), nil
}

func (s *fakeConditionalWalkSession) nextResponse() *gosnmp.SnmpPacket {
	if len(s.responses) == 0 {
		return &gosnmp.SnmpPacket{}
	}
	response := s.responses[0]
	s.responses = s.responses[1:]
	return response
}

func (s *fakeConditionalWalkSession) maxRepetitions() uint32 {
	return s.maxReps
}

func (s *fakeConditionalWalkSession) logf(format string, v ...interface{}) {
	s.logs = append(s.logs, fmt.Sprintf(format, v...))
}

func TestConditionalWalkNonIncreasingOid(t *testing.T) {
	pdu := gosnmp.SnmpPDU{
		Name:  ".1.3.6.1.2.1.2.2.1.2.1",
		Type:  gosnmp.OctetString,
		Value: []byte("desc"),
	}

	t.Run("errors when flag is disabled", func(t *testing.T) {
		session := &fakeConditionalWalkSession{
			responses: []*gosnmp.SnmpPacket{
				{Variables: []gosnmp.SnmpPDU{pdu}},
				{Variables: []gosnmp.SnmpPDU{pdu}},
			},
		}
		var visited []string

		err := conditionalWalk(context.Background(), session, "", false, 0, 0, false, func(dataUnit gosnmp.SnmpPDU) (string, error) {
			visited = append(visited, dataUnit.Name)
			return dataUnit.Name, nil
		})

		assert.EqualError(t, err, "detected infinite cycle: next OID '.1.3.6.1.2.1.2.2.1.2.1' is not after last OID '.1.3.6.1.2.1.2.2.1.2.1'")
		assert.Equal(t, []string{pdu.Name, pdu.Name}, visited)
		assert.Equal(t, [][]string{{".0.0"}, {pdu.Name}}, session.requests)
	})

	t.Run("skips when flag is enabled", func(t *testing.T) {
		session := &fakeConditionalWalkSession{
			responses: []*gosnmp.SnmpPacket{
				{Variables: []gosnmp.SnmpPDU{pdu}},
				{Variables: []gosnmp.SnmpPDU{pdu}},
				{Variables: []gosnmp.SnmpPDU{{Name: ".1.3.6.1.2.1.2.2.1.3.1", Type: gosnmp.EndOfMibView}}},
			},
		}
		var visited []string

		err := conditionalWalk(context.Background(), session, "", false, 0, 0, true, func(dataUnit gosnmp.SnmpPDU) (string, error) {
			visited = append(visited, dataUnit.Name)
			return dataUnit.Name, nil
		})

		assert.NoError(t, err)
		assert.Equal(t, []string{pdu.Name, pdu.Name}, visited)
		assert.Equal(t, [][]string{{".0.0"}, {pdu.Name}, {SkipOIDRowsNaive(pdu.Name)}}, session.requests)
		assert.Contains(t, strings.Join(session.logs, "\n"), "detected non-increasing OID while walking")
	})
}

func TestSkipOIDRowsNaive(t *testing.T) {
	for _, tc := range []struct{ oid, expected string }{
		{"1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.1.0"},
		{".1.3.6.1.2.1.1.1.0.", "1.3.6.1.2.1.1.1.0"},
		{"1.3.6.1.2.1.1.9.1.2.1", "1.3.6.1.2.1.1.9.1.3"},
		// breakdown example: column ID 1, key 127.0.0.1 is interpreted as column ID 127
		{"1.3.6.1.2.1.4.20.1.1.127.0.0.1", "1.3.6.1.2.1.4.20.1.1.128"},
		// breakdown example: column ID 1, key ending in 0 is interpreted as scalar
		{"1.3.6.1.2.1.4.24.4.1.1.195.200.251.0.0.255.255.255.0.0.0.0.0", "1.3.6.1.2.1.4.24.4.1.1.195.200.251.0.0.255.255.255.0.0.0.0.0"},
		// breakdown example: key containing '.1.':
		{"1.3.6.1.2.1.4.22.1.1.2.192.168.1.1", "1.3.6.1.2.1.4.22.1.1.2.192.168.1.2"},
	} {
		assert.Equal(t, tc.expected, SkipOIDRowsNaive(tc.oid))
	}
}
