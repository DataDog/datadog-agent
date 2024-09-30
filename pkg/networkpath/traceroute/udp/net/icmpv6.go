/* SPDX-License-Identifier: BSD-2-Clause */

package net

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// ICMPv6 is an ICMPv6 packet
type ICMPv6 struct {
	Type     ICMPv6Type
	Code     ICMPv6Code
	Checksum uint16
	// See RFC792, RFC4884, RFC4950.
	Unused uint32
	next   Layer
}

// ICMPv6HeaderLen is the ICMPv6 header length
var ICMPv6HeaderLen = 8

// ICMPv6Type defines ICMP types
type ICMPv6Type uint8

// ICMP types
var (
	ICMPv6TypeDestUnreachable                              ICMPv6Type = 1
	ICMPv6TypePacketTooBig                                 ICMPv6Type = 2
	ICMPv6TypeTimeExceeded                                 ICMPv6Type = 3
	ICMPv6TypeParameterProblem                             ICMPv6Type = 4
	ICMPv6TypeEchoRequest                                  ICMPv6Type = 128
	ICMPv6TypeEchoReply                                    ICMPv6Type = 129
	ICMPv6TypeGroupMembershipQuery                         ICMPv6Type = 130
	ICMPv6TypeGroupMembershipReport                        ICMPv6Type = 131
	ICMPv6TypeGroupMembershipReduction                     ICMPv6Type = 132
	ICMPv6TypeRouterSolicitation                           ICMPv6Type = 133
	ICMPv6TypeRouterAdvertisement                          ICMPv6Type = 134
	ICMPv6TypeNeighborAdvertisement                        ICMPv6Type = 135
	ICMPv6TypeNeighborSolicitation                         ICMPv6Type = 136
	ICMPv6TypeRedirect                                     ICMPv6Type = 137
	ICMPv6TypeRouterRenumbering                            ICMPv6Type = 138
	ICMPv6TypeICMPNodeInformationQuery                     ICMPv6Type = 139
	ICMPv6TypeICMPNodeInformationResponse                  ICMPv6Type = 140
	ICMPv6TypeInverseNeighborDiscoverySolicitationMessage  ICMPv6Type = 141
	ICMPv6TypeInverseNeighborDiscoveryAdvertisementMessage ICMPv6Type = 142
	ICMPv6TypeMLDv2MulticastListenerReport                 ICMPv6Type = 143
	ICMPv6TypeHomeAgentAddressDiscoveryRequestMessage      ICMPv6Type = 144
	ICMPv6TypeHomeAgentAddressDiscoveryReplyMessage        ICMPv6Type = 145
	ICMPv6TypeMobilePrefixSolicitation                     ICMPv6Type = 146
	ICMPv6TypeMobilePrefixAdvertisement                    ICMPv6Type = 147
	ICMPv6TypeCertificationPathSolicitation                ICMPv6Type = 148
	ICMPv6TypeCertificationPathAdvertisement               ICMPv6Type = 149
	ICMPv6TypeExperimentalMobilityProtocols                ICMPv6Type = 150
	ICMPv6TypeMulticastRouterAdvertisement                 ICMPv6Type = 151
	ICMPv6TypeMulticastRouterSolicitation                  ICMPv6Type = 152
	ICMPv6TypeMulticastRouterTermination                   ICMPv6Type = 153
	ICMPv6TypeFMIPv6Messages                               ICMPv6Type = 154
)

// ICMPv6Code defines ICMP types
type ICMPv6Code uint8

// TODO map ICMP codes, see https://www.iana.org/assignments/icmp-parameters/icmp-parameters.xhtml#icmp-parameters-codes
var (
	// Destination unreachable
	ICMPv6CodeNoRouteToDestination       ICMPv6Code
	ICMPv6CodeAdministrativelyProhibited ICMPv6Code = 1
	ICMPv6CodeAddressUnreachable         ICMPv6Code = 2
	ICMPv6CodePortUnreachable            ICMPv6Code = 4
	// Time exceeded
	ICMPv6CodeHopLimitExceeded          ICMPv6Code
	ICMPv6CodeFragmentReassemblyTimeout ICMPv6Code = 1
)

// NewICMPv6 constructs a new ICMPv6 header from a sequence of bytes
func NewICMPv6(b []byte) (*ICMPv6, error) {
	var i ICMPv6
	if err := i.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return &i, nil
}

// Next returns the next layer
func (i ICMPv6) Next() Layer {
	return i.next
}

// SetNext sets the next layer
func (i *ICMPv6) SetNext(l Layer) {
	i.next = l
}

// MarshalBinary serializes the layer
func (i ICMPv6) MarshalBinary() ([]byte, error) {
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, i.Type)
	binary.Write(&b, binary.BigEndian, i.Code)
	var (
		payload []byte
		err     error
	)
	if i.next != nil {
		payload, err = i.next.MarshalBinary()
		if err != nil {
			return nil, err
		}
	}
	// compute checksum
	i.Checksum = 0
	var bc bytes.Buffer
	binary.Write(&bc, binary.BigEndian, i.Type)
	binary.Write(&bc, binary.BigEndian, i.Code)
	binary.Write(&bc, binary.BigEndian, payload)
	i.Checksum = Checksum(bc.Bytes())
	binary.Write(&b, binary.BigEndian, i.Checksum)
	binary.Write(&b, binary.BigEndian, i.Unused)
	return b.Bytes(), nil
}

// UnmarshalBinary deserializes the layer
func (i *ICMPv6) UnmarshalBinary(b []byte) error {
	if len(b) < ICMPv6HeaderLen {
		return errors.New("short icmpv6 header")
	}
	i.Type = ICMPv6Type(b[0])
	i.Code = ICMPv6Code(b[1])
	i.Checksum = binary.BigEndian.Uint16(b[2:4])
	// TODO parse ICMP extensions
	payload := b[ICMPv6HeaderLen:]
	if len(payload) > 0 {
		i.next = &Raw{Data: payload}
	}
	return nil
}
