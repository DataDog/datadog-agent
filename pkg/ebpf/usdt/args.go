// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package usdt

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// Register arguments are read from struct pt_regs, the context of uprobe
// programs.

// x86Registers maps the x86-64 general purpose registers to their offset in
// struct pt_regs (arch/x86/include/asm/ptrace.h). All the aliases of a
// register (rax, eax, ax, al) share its slot: the argument size truncates the
// value. The high byte registers (ah, bh, ch, dh) are not supported.
var x86Registers = func() map[string]int16 {
	regs := map[string]int16{}
	for _, r := range []struct {
		names  []string
		offset int16
	}{
		{[]string{"r15", "r15d", "r15w", "r15b"}, 0},
		{[]string{"r14", "r14d", "r14w", "r14b"}, 8},
		{[]string{"r13", "r13d", "r13w", "r13b"}, 16},
		{[]string{"r12", "r12d", "r12w", "r12b"}, 24},
		{[]string{"rbp", "ebp", "bp", "bpl"}, 32},
		{[]string{"rbx", "ebx", "bx", "bl"}, 40},
		{[]string{"r11", "r11d", "r11w", "r11b"}, 48},
		{[]string{"r10", "r10d", "r10w", "r10b"}, 56},
		{[]string{"r9", "r9d", "r9w", "r9b"}, 64},
		{[]string{"r8", "r8d", "r8w", "r8b"}, 72},
		{[]string{"rax", "eax", "ax", "al"}, 80},
		{[]string{"rcx", "ecx", "cx", "cl"}, 88},
		{[]string{"rdx", "edx", "dx", "dl"}, 96},
		{[]string{"rsi", "esi", "si", "sil"}, 104},
		{[]string{"rdi", "edi", "di", "dil"}, 112},
		{[]string{"rsp", "esp", "sp", "spl"}, 152},
	} {
		for _, name := range r.names {
			regs[name] = r.offset
		}
	}
	return regs
}()

// On arm64, struct pt_regs starts with struct user_pt_regs
// (arch/arm64/include/uapi/asm/ptrace.h): u64 regs[31], sp, pc, pstate.
const arm64SPOffset = 31 * 8

// ParseArgs parses the argument specification of a USDT probe site of a file
// targeting machine. Only x86-64 and arm64 are supported.
//
// The specification is a space-separated list of "size@operand", where size
// is the size of the argument in bytes (1, 2, 4 or 8), negative if signed,
// and operand is an assembler operand:
//   - x86-64: "%reg", "$imm", "off(%reg)", "(%reg)";
//   - arm64: "xN", "wN", "sp", "imm", "[xN]", "[xN, off]", "[sp, off]".
//
// Other operands (SIB or symbolic addressing, ...) are rejected.
func ParseArgs(machine safeelf.Machine, args string) (Spec, error) {
	var parseOperand func(string, *ArgSpec) error
	switch machine {
	case safeelf.EM_X86_64:
		parseOperand = parseX86Operand
	case safeelf.EM_AARCH64:
		parseOperand = parseARM64Operand
	default:
		return Spec{}, fmt.Errorf("unsupported machine %s", machine)
	}

	var spec Spec
	rest := args
	for {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			break
		}
		if spec.Cnt == MaxArgs {
			return Spec{}, fmt.Errorf("more than %d arguments in %q", MaxArgs, args)
		}

		sizeStr, operandStart, ok := strings.Cut(rest, "@")
		if !ok {
			return Spec{}, fmt.Errorf("argument %q: missing size", rest)
		}
		// arm64 memory operands contain a space ("[sp, 112]"): an operand
		// ends at the first space outside brackets
		end := len(operandStart)
		depth := 0
		for i, c := range operandStart {
			if c == '[' {
				depth++
			} else if c == ']' {
				depth--
			} else if (c == ' ' || c == '\t') && depth <= 0 {
				end = i
				break
			}
		}
		operand := operandStart[:end]
		rest = operandStart[end:]

		arg := &spec.Args[spec.Cnt]
		if err := parseSize(sizeStr, arg); err != nil {
			return Spec{}, fmt.Errorf("argument %s@%s: %w", sizeStr, operand, err)
		}
		if err := parseOperand(operand, arg); err != nil {
			return Spec{}, fmt.Errorf("argument %s@%s: %w", sizeStr, operand, err)
		}
		spec.Cnt++
	}
	return spec, nil
}

func parseSize(s string, arg *ArgSpec) error {
	size, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("invalid size %q", s)
	}
	if size < 0 {
		arg.Arg_signed = 1
		size = -size
	}
	switch size {
	case 1, 2, 4, 8:
		arg.Arg_bitshift = uint8(64 - 8*size)
		return nil
	default:
		return fmt.Errorf("invalid size %q", s)
	}
}

func parseX86Operand(s string, arg *ArgSpec) error {
	switch {
	case strings.HasPrefix(s, "$"):
		v, err := parseNumber(s[1:])
		if err != nil {
			return err
		}
		arg.Arg_type = ArgConst
		arg.Val_off = v
		return nil

	case strings.HasPrefix(s, "%"):
		off, err := x86Register(s[1:])
		if err != nil {
			return err
		}
		arg.Arg_type = ArgReg
		arg.Reg_off = off
		return nil

	case strings.HasSuffix(s, ")"):
		disp, inner, ok := strings.Cut(strings.TrimSuffix(s, ")"), "(")
		if !ok {
			return errors.New("unsupported operand")
		}
		if strings.Contains(inner, ",") {
			return errors.New("SIB addressing is not supported")
		}
		if !strings.HasPrefix(inner, "%") {
			return errors.New("unsupported operand")
		}
		off, err := x86Register(inner[1:])
		if err != nil {
			return err
		}
		var v uint64
		if disp != "" {
			if v, err = parseNumber(disp); err != nil {
				return err
			}
		}
		arg.Arg_type = ArgRegDeref
		arg.Reg_off = off
		arg.Val_off = v
		return nil

	default:
		return errors.New("unsupported operand")
	}
}

func x86Register(name string) (int16, error) {
	off, ok := x86Registers[name]
	if !ok {
		return 0, fmt.Errorf("unsupported register %q", name)
	}
	return off, nil
}

func parseARM64Operand(s string, arg *ArgSpec) error {
	if inner, ok := strings.CutPrefix(s, "["); ok {
		inner, ok = strings.CutSuffix(inner, "]")
		if !ok {
			return errors.New("unsupported operand")
		}
		base, disp, hasDisp := strings.Cut(inner, ",")
		off, err := arm64Register(strings.TrimSpace(base), false)
		if err != nil {
			return err
		}
		var v uint64
		if hasDisp {
			disp = strings.TrimSpace(disp)
			if _, err := arm64Register(disp, true); err == nil {
				return errors.New("register offset addressing is not supported")
			}
			if v, err = parseNumber(strings.TrimPrefix(disp, "#")); err != nil {
				return err
			}
		}
		arg.Arg_type = ArgRegDeref
		arg.Reg_off = off
		arg.Val_off = v
		return nil
	}

	if v, err := parseNumber(strings.TrimPrefix(s, "#")); err == nil {
		arg.Arg_type = ArgConst
		arg.Val_off = v
		return nil
	}

	off, err := arm64Register(s, true)
	if err != nil {
		return err
	}
	arg.Arg_type = ArgReg
	arg.Reg_off = off
	return nil
}

// arm64Register returns the offset in struct pt_regs of the general purpose
// register name: sp, xN or, if allowW, its 32-bit view wN.
func arm64Register(name string, allowW bool) (int16, error) {
	if name == "sp" {
		return arm64SPOffset, nil
	}
	if len(name) >= 2 && (name[0] == 'x' || (allowW && name[0] == 'w')) {
		if n, err := strconv.ParseUint(name[1:], 10, 8); err == nil && n <= 30 && (name[1] != '0' || len(name) == 2) {
			return int16(n * 8), nil
		}
	}
	return 0, fmt.Errorf("unsupported register %q", name)
}

// parseNumber parses a decimal or hexadecimal ("0x") integer with an
// optional sign, into its 64-bit two's complement representation.
func parseNumber(s string) (uint64, error) {
	digits, neg := strings.CutPrefix(s, "-")
	if !neg {
		digits = strings.TrimPrefix(digits, "+")
	}
	base := 10
	if hex, ok := strings.CutPrefix(strings.ToLower(digits), "0x"); ok {
		digits, base = hex, 16
	}
	// with an explicit base, ParseUint rejects signs and '_' separators
	v, err := strconv.ParseUint(digits, base, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", s)
	}
	if neg {
		v = -v
	}
	return v, nil
}
