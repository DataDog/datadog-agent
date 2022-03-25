// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"fmt"
	"net"
)

// CIDRValues describes a set of CIDRs, either simple IPs or CIDR
type CIDRValues struct {
	matchers []IPMatcher

	// caches
	fieldValues []FieldValue

	exists map[interface{}]bool
}

// NewCIDRValues returns a new CIDRValues
func NewCIDRValues(ips []net.IP, cidrs []*net.IPNet) *CIDRValues {
	var values CIDRValues
	for _, ip := range ips {
		values.AppendIP(ip)
	}
	for _, cidr := range cidrs {
		values.AppendCIDR(cidr)
	}
	return &values
}

// AppendFieldValue append a FieldValue
func (s *CIDRValues) AppendFieldValue(value FieldValue) error {
	if s.exists == nil {
		s.exists = make(map[interface{}]bool)
	}
	if s.exists[value.Value] {
		return nil
	}
	s.exists[value.Value] = true

	switch value.Type {
	case IPValueType, CIDRValueType:
		if err := value.Compile(); err != nil {
			return err
		}
		s.matchers = append(s.matchers, value.IPMatcher)
	default:
		return fmt.Errorf("unknown value type `%v`", value.Type)
	}
	s.fieldValues = append(s.fieldValues, value)

	return nil
}

// AppendIP appends an IP
func (s *CIDRValues) AppendIP(ip net.IP) {
	fieldValue := FieldValue{
		Type: IPValueType,
		IPMatcher: &SingleIPMatcher{
			ip: ip,
		},
	}
	s.matchers = append(s.matchers, fieldValue.IPMatcher)
	s.fieldValues = append(s.fieldValues, fieldValue)
}

// AppendCIDR appends a CIDR
func (s *CIDRValues) AppendCIDR(cidr *net.IPNet) {
	fieldValue := FieldValue{
		Type: CIDRValueType,
		IPMatcher: &CIDRMatcher{
			net: cidr,
		},
	}
	s.matchers = append(s.matchers, fieldValue.IPMatcher)
	s.fieldValues = append(s.fieldValues, fieldValue)
}

// GetMatchers return the matchers
func (s *CIDRValues) GetMatchers() []IPMatcher {
	return s.matchers
}

// GetFieldValues returns the list of FieldValues stored in the current CIDRValues
func (s *CIDRValues) GetFieldValues() []FieldValue {
	return s.fieldValues
}

// SetFieldValues apply field values
func (s *CIDRValues) SetFieldValues(values ...FieldValue) error {
	// reset internal caches
	s.matchers = s.matchers[:0]
	s.exists = nil

	for _, value := range values {
		if err := s.AppendFieldValue(value); err != nil {
			return err
		}
	}

	return nil
}

// Matches returns whether the value matches the provided IPMatcher
func (s *CIDRValues) Matches(value IPMatcher) bool {
	for _, pm := range s.matchers {
		if pm.Matches(value) {
			return true
		}
	}

	return false
}

// MatchesAll returns true if value matches all CIDRValues entries
func (s *CIDRValues) MatchesAll(value IPMatcher) bool {
	for _, pm := range s.matchers {
		if !pm.Matches(value) {
			return false
		}
	}

	return true
}

// IPMatcher defines an IP matcher
type IPMatcher interface {
	Compile(pattern string) error
	IPMatches(value net.IP) bool
	Matches(value IPMatcher) bool
	GetType() FieldValueType
	String() string
}

// SingleIPMatcher defines an IPv4/IPv6 matcher
type SingleIPMatcher struct {
	ip net.IP
}

func (sipm *SingleIPMatcher) String() string {
	return sipm.ip.String()
}

// Compile the string representation of an IP to a net.IP
func (sipm *SingleIPMatcher) Compile(pattern string) error {
	if sipm.ip != nil {
		return nil
	}

	ip := net.ParseIP(pattern)
	if ip == nil {
		return fmt.Errorf("invalid textual representation of an IP address")
	}
	sipm.ip = ip

	return nil
}

// GetType returns the FieldValueType of the IPMatcher
func (sipm *SingleIPMatcher) GetType() FieldValueType {
	return IPValueType
}

// IPMatches returns true the value matches
func (sipm *SingleIPMatcher) IPMatches(value net.IP) bool {
	return sipm.ip.Equal(value)
}

// Matches matches with an IPMatcher
func (sipm *SingleIPMatcher) Matches(value IPMatcher) bool {
	switch value.GetType() {
	case IPValueType:
		ipMatcher, ok := value.(*SingleIPMatcher)
		if ok {
			return sipm.IPMatches(ipMatcher.ip)
		}
		return false
	case CIDRValueType:
		return value.IPMatches(sipm.ip)
	default:
		return false
	}
}

// CIDRMatcher defines an IPv4/IPv6 CIDR
type CIDRMatcher struct {
	net    *net.IPNet
	lastIP net.IP
}

func (cm *CIDRMatcher) String() string {
	return cm.net.String()
}

// Compile the string representation of a CIDR to a net.IPNet
func (cm *CIDRMatcher) Compile(pattern string) error {
	if cm.net != nil {
		return nil
	}

	_, ipNet, err := net.ParseCIDR(pattern)
	if err != nil {
		return fmt.Errorf("invalid textual representation of a CIDR: %v", err)
	}
	cm.net = ipNet

	return nil
}

// GetType returns the FieldValueType of the IPMatcher
func (cm *CIDRMatcher) GetType() FieldValueType {
	return CIDRValueType
}

// IPMatches returns wether the value matches
func (cm *CIDRMatcher) IPMatches(value net.IP) bool {
	return cm.net.Contains(value)
}

// Matches matches with an IPMatcher
func (cm *CIDRMatcher) Matches(value IPMatcher) bool {
	switch value.GetType() {
	case IPValueType:
		sipm, ok := value.(*SingleIPMatcher)
		if ok {
			return cm.IPMatches(sipm.ip)
		}
		return false
	case CIDRValueType:
		cidrMatcher, ok := value.(*CIDRMatcher)
		if ok {
			return cidrMatcher.IPMatches(cm.net.IP) || cm.IPMatches(cidrMatcher.net.IP)
		}
		return false
	default:
		return false
	}
}

// NewIPMatcher returns a new IP matcher
func NewIPMatcher(kind FieldValueType, pattern string) (IPMatcher, error) {
	switch kind {
	case IPValueType:
		var matcher SingleIPMatcher
		if err := matcher.Compile(pattern); err != nil {
			return nil, fmt.Errorf("invalid IP `%s`: %s", pattern, err)
		}
		return &matcher, nil
	case CIDRValueType:
		var matcher CIDRMatcher
		if err := matcher.Compile(pattern); err != nil {
			return nil, fmt.Errorf("invalid CIDR `%s`: %s", pattern, err)
		}
		return &matcher, nil
	}

	return nil, fmt.Errorf("unknown type")
}
