// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib

import (
	"context"
	"errors"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootOIDsEncoding(t *testing.T) {
	assert.Equal(t, []string{".0.0", ".1.0"}, RootOIDs)
	for _, tc := range []struct {
		version gosnmp.SnmpVersion
		pduType gosnmp.PDUType
	}{
		{version: gosnmp.Version1, pduType: gosnmp.GetNextRequest},
		{version: gosnmp.Version2c, pduType: gosnmp.GetBulkRequest},
	} {
		for _, rootOID := range RootOIDs {
			t.Run(tc.pduType.String()+"/"+rootOID, func(t *testing.T) {
				snmp := &gosnmp.GoSNMP{Version: tc.version, Community: "public"}
				packet, err := snmp.SnmpEncodePacket(tc.pduType, []gosnmp.SnmpPDU{
					{Name: rootOID, Type: gosnmp.Null},
				}, 0, 1)
				require.NoError(t, err)
				decoded, err := snmp.SnmpDecodePacket(packet)
				require.NoError(t, err)
				require.Len(t, decoded.Variables, 1)
				assert.Equal(t, rootOID, decoded.Variables[0].Name)
			})
		}
	}
}

func TestConditionalWalkStartingOID(t *testing.T) {
	timeoutErr := errors.New("request timeout")
	data := &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
		{Name: ".1.0.8802.1.1.2.1.3.3.0", Type: gosnmp.OctetString, Value: "device"},
	}}
	end := &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
		{Type: gosnmp.EndOfMibView},
	}}
	type response struct {
		packet *gosnmp.SnmpPacket
		err    error
	}
	for _, tc := range []struct {
		name          string
		rootOID       string
		responses     []response
		expectedCalls []string
		expectedPDUs  []gosnmp.SnmpPDU
		expectedError error
	}{
		{
			name:          "default root succeeds",
			responses:     []response{{packet: data}, {packet: end}},
			expectedCalls: []string{".0.0", data.Variables[0].Name},
			expectedPDUs:  data.Variables,
		},
		{
			name:          "transport error falls back",
			responses:     []response{{err: timeoutErr}, {packet: data}, {packet: end}},
			expectedCalls: []string{".0.0", ".1.0", data.Variables[0].Name},
			expectedPDUs:  data.Variables,
		},
		{
			name:          "SNMP error falls back",
			rootOID:       ".",
			responses:     []response{{packet: &gosnmp.SnmpPacket{Error: gosnmp.GenErr}}, {packet: data}, {packet: end}},
			expectedCalls: []string{".0.0", ".1.0", data.Variables[0].Name},
			expectedPDUs:  data.Variables,
		},
		{
			name:          "both roots fail",
			rootOID:       "0.0",
			responses:     []response{{err: timeoutErr}, {err: timeoutErr}},
			expectedCalls: []string{".0.0", ".1.0"},
			expectedError: timeoutErr,
		},
		{
			name:          "no fallback after progress",
			responses:     []response{{packet: data}, {err: timeoutErr}},
			expectedCalls: []string{".0.0", data.Variables[0].Name},
			expectedPDUs:  data.Variables,
			expectedError: timeoutErr,
		},
		{
			name:          "no fallback for explicit root",
			rootOID:       ".1.3.6.1.2.1",
			responses:     []response{{err: timeoutErr}},
			expectedCalls: []string{".1.3.6.1.2.1"},
			expectedError: timeoutErr,
		},
		{
			name:          "empty walk does not retry",
			responses:     []response{{packet: end}},
			expectedCalls: []string{".0.0"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			getNext := func(oids []string) (*gosnmp.SnmpPacket, error) {
				require.Len(t, oids, 1)
				calls = append(calls, oids[0])
				require.LessOrEqual(t, len(calls), len(tc.responses))
				resp := tc.responses[len(calls)-1]
				return resp.packet, resp.err
			}
			var collected []gosnmp.SnmpPDU
			walkFn := func(pdu gosnmp.SnmpPDU) (string, error) {
				collected = append(collected, pdu)
				return "", nil
			}

			err := conditionalWalk(context.Background(), getNext, gosnmp.Logger{}, tc.rootOID, 0, 0, walkFn)
			if tc.expectedError != nil {
				require.ErrorIs(t, err, tc.expectedError)
				assert.IsType(t, &ConnectionError{}, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.expectedCalls, calls)
			assert.Equal(t, tc.expectedPDUs, collected)
		})
	}
}

func TestConditionalWalkTriesRootsInOrder(t *testing.T) {
	originalRoots := RootOIDs
	RootOIDs = []string{".0.0", ".1.0", ".1.3.6.1.2.1"}
	t.Cleanup(func() { RootOIDs = originalRoots })

	for _, tc := range []struct {
		name    string
		success bool
	}{
		{name: "all roots fail"},
		{name: "third root succeeds", success: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			timeoutErr := errors.New("request timeout")
			var calls []string
			getNext := func(oids []string) (*gosnmp.SnmpPacket, error) {
				calls = append(calls, oids[0])
				require.LessOrEqual(t, len(calls), 4)
				if len(calls) < 3 || !tc.success {
					return nil, timeoutErr
				}
				if len(calls) == 3 {
					return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
						{Name: ".1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: "device"},
					}}, nil
				}
				return &gosnmp.SnmpPacket{}, nil
			}
			var collected []string
			walkFn := func(pdu gosnmp.SnmpPDU) (string, error) {
				collected = append(collected, pdu.Name)
				return "", nil
			}

			err := conditionalWalk(context.Background(), getNext, gosnmp.Logger{}, "", 0, 0, walkFn)
			if tc.success {
				require.NoError(t, err)
				assert.Equal(t, []string{".0.0", ".1.0", ".1.3.6.1.2.1", ".1.3.6.1.2.1.1.1.0"}, calls)
				assert.Equal(t, []string{".1.3.6.1.2.1.1.1.0"}, collected)
			} else {
				require.ErrorIs(t, err, timeoutErr)
				assert.Equal(t, []string{".0.0", ".1.0", ".1.3.6.1.2.1"}, calls)
				assert.Empty(t, collected)
			}
		})
	}
}

func TestConditionalWalkFallbackRespectsLimits(t *testing.T) {
	for _, tc := range []struct {
		name         string
		cancel       bool
		maxCallCount int
		expectedErr  string
	}{
		{name: "cancellation", cancel: true, expectedErr: "context canceled"},
		{name: "request limit", maxCallCount: 2, expectedErr: "exceeded the maximum request limit (2)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls []string
			getNext := func(oids []string) (*gosnmp.SnmpPacket, error) {
				calls = append(calls, oids[0])
				if tc.cancel {
					cancel()
				}
				return nil, errors.New("request timeout")
			}
			walkFn := func(gosnmp.SnmpPDU) (string, error) {
				t.Fatal("no PDU should be emitted")
				return "", nil
			}

			err := conditionalWalk(ctx, getNext, gosnmp.Logger{}, "", 0, tc.maxCallCount, walkFn)
			require.EqualError(t, err, tc.expectedErr)
			assert.Equal(t, []string{".0.0"}, calls)
		})
	}
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
