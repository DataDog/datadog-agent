// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package verifier

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	insnRegex           = regexp.MustCompile(`^([0-9]+): \([0-9a-f]+\) ([^;]*)\s*(; R[0-9]+.*)?`)
	regStateRegex       = regexp.MustCompile(`^([0-9]+): (R[0-9]+.*)`)
	singleRegStateRegex = regexp.MustCompile(`R([0-9]+)(_[^=]+)?=([^ ]+)`)
	regInfoRegex        = regexp.MustCompile(`^([a-z_]+)?(P)?(-?[0-9]+|\((.*)\))`)
)

// verifierLogParser is a struct that maintains the state necessary to parse the verifier log
// and extract the complexity information.
type verifierLogParser struct {
	complexity    ComplexityInfo      // Resulting complexity information
	progSourceMap map[int]*SourceLine // Mapping of assembly instruction to source line
}

func newVerifierLogParser(progSourceMap map[int]*SourceLine) *verifierLogParser {
	return &verifierLogParser{
		progSourceMap: progSourceMap,
		complexity: ComplexityInfo{
			InsnMap:   make(map[int]*InstructionInfo),
			SourceMap: make(map[string]*SourceLineStats),
		},
	}
}

// parseVerifierLog parses the verifier log and returns the complexity information, which is also stored
// in the verifierLogParser struct.
func (vlp *verifierLogParser) parseVerifierLog(log string) (*ComplexityInfo, error) {
	// Read all the verifier log, parse the assembly instructions and count how many times we've seen them
	for _, line := range strings.Split(log, "\n") {
		if err := vlp.parseLine(line); err != nil {
			return nil, err
		}
	}

	// Now build the source map for the source lines
	for _, insn := range vlp.complexity.InsnMap {
		if insn.Source == nil {
			continue
		}
		if _, ok := vlp.complexity.SourceMap[insn.Source.LineInfo]; !ok {
			vlp.complexity.SourceMap[insn.Source.LineInfo] = &SourceLineStats{
				NumInstructions:            0,
				MaxPasses:                  0,
				TotalInstructionsProcessed: 0,
				MinPasses:                  math.MaxInt32,
				AssemblyInsns:              []int{},
			}
		}
		stats := vlp.complexity.SourceMap[insn.Source.LineInfo]
		stats.NumInstructions++
		stats.MaxPasses = max(stats.MaxPasses, insn.TimesProcessed)
		stats.MinPasses = min(stats.MinPasses, insn.TimesProcessed)
		stats.TotalInstructionsProcessed += insn.TimesProcessed
		stats.AssemblyInsns = append(stats.AssemblyInsns, insn.Index)
	}

	return &vlp.complexity, nil
}

func (vlp *verifierLogParser) tryAttachRegisterState(regData string, insIdx int) error {
	insinfo, ok := vlp.complexity.InsnMap[insIdx]
	if !ok {
		// Some times we will not have the previous instruction available depending on how
		// the verifier outputs register state. For example, in some versions it will generate
		// the state *before* each instruction, but we want the state *after* the instruction, and
		// as such the first state we find (state before instruction 1) will have no match, for example.
		return nil
	}

	// For ease of parsing, replace certain patterns that introduce spaces and make parsing harder
	regData = strings.ReplaceAll(regData, "; ", ";")

	regMatches := singleRegStateRegex.FindAllStringSubmatch(regData, -1)
	regState := make(map[int]*RegisterState)
	for _, regMatch := range regMatches {
		data, err := parseRegisterState(regMatch)
		if err != nil {
			return fmt.Errorf("failed to parse register state ('%s'): %w", regData, err)
		}
		regState[data.Register] = data
	}

	insinfo.RegisterState = regState
	insinfo.RegisterStateRaw = regData

	return nil
}

func (vlp *verifierLogParser) parseLine(line string) error {
	match := regStateRegex.FindStringSubmatch(line)
	if len(match) > 0 {
		regInsnIdx, err := strconv.Atoi(match[1])
		if err != nil {
			return fmt.Errorf("failed to parse instruction index (line is '%s'): %w", line, err)
		}

		// Try attach the register state to the previous instruction
		err = vlp.tryAttachRegisterState(match[2], regInsnIdx-1)
		if err != nil {
			return fmt.Errorf("failed to attach register state (line is '%s'): %w", line, err)
		}
		return nil
	}

	match = insnRegex.FindStringSubmatch(line)
	if len(match) == 0 {
		return nil // Only interested in lines that contain assembly instructions
	}
	insIdx, err := strconv.Atoi(match[1])
	if err != nil {
		return fmt.Errorf("failed to parse instruction index (line is '%s'): %w", line, err)
	}
	if _, ok := vlp.complexity.InsnMap[insIdx]; !ok {
		vlp.complexity.InsnMap[insIdx] = &InstructionInfo{Index: insIdx}
	}
	insinfo := vlp.complexity.InsnMap[insIdx]
	insinfo.TimesProcessed++
	insinfo.Code = strings.TrimSpace(match[2])
	if vlp.progSourceMap != nil {
		insinfo.Source = vlp.progSourceMap[insIdx]
	}

	// In some kernel versions (6.5 at least), the register state for the next instruction might be printed after this instruction
	if len(match) >= 4 && match[3] != "" {
		regData := match[3][2:] // Remove the leading "; "
		err = vlp.tryAttachRegisterState(regData, insIdx)
		if err != nil {
			return fmt.Errorf("Cannot attach register state to instruction %d: %w", insIdx, err)
		}
	}

	return nil
}

func tryPowerOfTwoRepresentation(value int64) string {
	if value == 0 {
		return "0"
	} else if value == math.MaxInt64 {
		// Compute here to avoid overflow
		return "2^63 - 1"
	} else if value == math.MinInt64 {
		return "-2^63"
	}

	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}

	if (value & (value - 1)) == 0 { // Exact power of two, return a nice representation
		return fmt.Sprintf("%s2^%d", sign, int(math.Log2(float64(value))))
	} else if ((value + 1) & value) == 0 { // Value is a power of two minus one
		return fmt.Sprintf("%s2^%d - 1", sign, int(math.Log2(float64(value+1))))
	}

	return fmt.Sprintf("%s%d (%s0x%X)", sign, value, sign, value)
}

// parseRegisterState parses the state of a single register and returns a RegisterState struct. Receives
// the match from the singleRegStateRegex.
func parseRegisterState(regMatch []string) (*RegisterState, error) {
	if len(regMatch) != 4 {
		return nil, fmt.Errorf("failed to parse register state: %v, should have a full match and 3 groups", regMatch)
	}

	regNum, err := strconv.Atoi(regMatch[1])
	if err != nil {
		return nil, fmt.Errorf("cannot parse register number %v: %w", regMatch[1], err)
	}

	livenessCode := regMatch[2]
	var liveness string

	switch livenessCode {
	case "_w":
		liveness = "written"
	case "_r":
		liveness = "read"
	case "_D":
		liveness = "done"
	default:
		liveness = ""
	}

	regValue := regMatch[3]
	regInfoGroups := regInfoRegex.FindStringSubmatch(regValue)
	if len(regInfoGroups) == 0 {
		return nil, fmt.Errorf("Cannot parse register value %v", regValue)
	}

	regType := regInfoGroups[1]
	if regType == "inv" || regType == "" {
		// Depending on the kernel version, we might see scalars represented either
		// as "scalar" type, as "inv" type or as a raw number with no type
		regType = "scalar"
	}

	if regType == "scalar" {
		// Parse scalar values a bit better
		regValue = parseRegisterScalarValue(regInfoGroups)
	} else {
		regValue = strings.Replace(regValue, regType, "", 1) // Remove the type from the value
		regValue = strings.Trim(regValue, "()")              // Remove the parentheses
	}

	return &RegisterState{
		Register: regNum,
		Live:     liveness,
		Type:     regType,
		Value:    regValue,
		Precise:  regInfoGroups[2] == "P",
	}, nil
}

// parseRegisterScalarValue parses the scalar value from the register state match and returns a
// human-readable value.
func parseRegisterScalarValue(regInfoGroups []string) string {
	// Scalar values are either a raw numeric value, or a list of key-value pairs within parenthesis
	regRawValue := regInfoGroups[3]
	regAttributes := regInfoGroups[4]

	if regAttributes == "" {
		if regRawValue == "()" {
			return "?" // Handle the case where the register is just "scalar()"
		}
		return regRawValue
	}

	// Parse the attributes, we're mainly interested in the interval that's defined by those attributes
	minValue := int64(0)
	maxValue := int64(0)
	hasRange := false

	for _, kv := range strings.Split(regInfoGroups[4], ",") {
		kvParts := strings.Split(kv, "=")
		if strings.Contains(kvParts[0], "min") {
			// Ignore errors here, mostly due to sizes (can't parse UINT_MAX in INT64) and for now we don't care
			// about
			v, _ := strconv.ParseInt(kvParts[1], 10, 64)
			minValue = min(v, minValue)
			hasRange = true
		} else if strings.Contains(kvParts[0], "max") {
			v, _ := strconv.ParseInt(kvParts[1], 10, 64)
			maxValue = max(v, maxValue)
			hasRange = true
		}
	}

	if hasRange {
		return fmt.Sprintf("[%s, %s]", tryPowerOfTwoRepresentation(minValue), tryPowerOfTwoRepresentation(maxValue))
	}

	return "?"
}
