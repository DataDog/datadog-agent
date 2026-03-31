// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package optimizer

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
)

// foldOp performs constant folding on a binary ALU operation.
// Port of fold_op() from optimize.c.
func (os *OptState) foldOp(s *codegen.Stmt, v0, v1 uint32) {
	a := os.ConstVal(v0)
	b := os.ConstVal(v1)

	switch bpf.Op(uint16(s.Code)) {
	case bpf.BPF_ADD:
		a += b
	case bpf.BPF_SUB:
		a -= b
	case bpf.BPF_MUL:
		a *= b
	case bpf.BPF_DIV:
		if b == 0 {
			os.Err = fmt.Errorf("division by zero")
			return
		}
		a /= b
	case bpf.BPF_MOD:
		if b == 0 {
			os.Err = fmt.Errorf("modulus by zero")
			return
		}
		a %= b
	case bpf.BPF_AND:
		a &= b
	case bpf.BPF_OR:
		a |= b
	case bpf.BPF_XOR:
		a ^= b
	case bpf.BPF_LSH:
		if b < 32 {
			a <<= b
		} else {
			a = 0
		}
	case bpf.BPF_RSH:
		if b < 32 {
			a >>= b
		} else {
			a = 0
		}
	}
	s.K = a
	s.Code = int(bpf.BPF_LD | bpf.BPF_IMM)
	os.NonBranchMovementPerformed = true
	os.Done = false
}

// OptStmt performs symbolic evaluation and optimization of a single statement.
// val is the current value table, alter controls whether to perform transformations.
// Port of opt_stmt() from optimize.c.
func (os *OptState) OptStmt(s *codegen.Stmt, val []uint32, alter bool) {
	switch s.Code {
	case int(bpf.BPF_LD | bpf.BPF_ABS | bpf.BPF_W),
		int(bpf.BPF_LD | bpf.BPF_ABS | bpf.BPF_H),
		int(bpf.BPF_LD | bpf.BPF_ABS | bpf.BPF_B):
		v := os.F(s.Code, s.K, 0)
		os.vstore(s, &val[AAtom], v, alter)

	case int(bpf.BPF_LD | bpf.BPF_IND | bpf.BPF_W),
		int(bpf.BPF_LD | bpf.BPF_IND | bpf.BPF_H),
		int(bpf.BPF_LD | bpf.BPF_IND | bpf.BPF_B):
		v := val[XAtom]
		if alter && os.IsConst(v) {
			s.Code = int(bpf.BPF_LD | bpf.BPF_ABS | bpf.Size(uint16(s.Code)))
			s.K += os.ConstVal(v)
			v = os.F(s.Code, s.K, 0)
			os.NonBranchMovementPerformed = true
			os.Done = false
		} else {
			v = os.F(s.Code, s.K, v)
		}
		os.vstore(s, &val[AAtom], v, alter)

	case int(bpf.BPF_LD | bpf.BPF_LEN):
		v := os.F(s.Code, 0, 0)
		os.vstore(s, &val[AAtom], v, alter)

	case int(bpf.BPF_LD | bpf.BPF_IMM):
		v := os.K(s.K)
		os.vstore(s, &val[AAtom], v, alter)

	case int(bpf.BPF_LDX | bpf.BPF_IMM):
		v := os.K(s.K)
		os.vstore(s, &val[XAtom], v, alter)

	case int(bpf.BPF_LDX | bpf.BPF_MSH | bpf.BPF_B):
		v := os.F(s.Code, s.K, 0)
		os.vstore(s, &val[XAtom], v, alter)

	case int(bpf.BPF_ALU | bpf.BPF_NEG):
		if alter && os.IsConst(val[AAtom]) {
			s.Code = int(bpf.BPF_LD | bpf.BPF_IMM)
			s.K = 0 - os.ConstVal(val[AAtom])
			val[AAtom] = os.K(s.K)
		} else {
			val[AAtom] = os.F(s.Code, val[AAtom], 0)
		}

	case int(bpf.BPF_ALU | bpf.BPF_ADD | bpf.BPF_K),
		int(bpf.BPF_ALU | bpf.BPF_SUB | bpf.BPF_K),
		int(bpf.BPF_ALU | bpf.BPF_MUL | bpf.BPF_K),
		int(bpf.BPF_ALU | bpf.BPF_DIV | bpf.BPF_K),
		int(bpf.BPF_ALU | bpf.BPF_MOD | bpf.BPF_K),
		int(bpf.BPF_ALU | bpf.BPF_AND | bpf.BPF_K),
		int(bpf.BPF_ALU | bpf.BPF_OR | bpf.BPF_K),
		int(bpf.BPF_ALU | bpf.BPF_XOR | bpf.BPF_K),
		int(bpf.BPF_ALU | bpf.BPF_LSH | bpf.BPF_K),
		int(bpf.BPF_ALU | bpf.BPF_RSH | bpf.BPF_K):
		op := bpf.Op(uint16(s.Code))
		if alter {
			if s.K == 0 {
				if op == bpf.BPF_ADD || op == bpf.BPF_LSH || op == bpf.BPF_RSH ||
					op == bpf.BPF_OR || op == bpf.BPF_XOR {
					s.Code = codegen.NOP
					return
				}
				if op == bpf.BPF_MUL || op == bpf.BPF_AND {
					s.Code = int(bpf.BPF_LD | bpf.BPF_IMM)
					val[AAtom] = os.K(s.K)
					return
				}
				if op == bpf.BPF_DIV {
					os.Err = fmt.Errorf("division by zero")
					return
				}
				if op == bpf.BPF_MOD {
					os.Err = fmt.Errorf("modulus by zero")
					return
				}
			}
			if os.IsConst(val[AAtom]) {
				os.foldOp(s, val[AAtom], os.K(s.K))
				val[AAtom] = os.K(s.K)
				return
			}
		}
		val[AAtom] = os.F(s.Code, val[AAtom], os.K(s.K))

	case int(bpf.BPF_ALU | bpf.BPF_ADD | bpf.BPF_X),
		int(bpf.BPF_ALU | bpf.BPF_SUB | bpf.BPF_X),
		int(bpf.BPF_ALU | bpf.BPF_MUL | bpf.BPF_X),
		int(bpf.BPF_ALU | bpf.BPF_DIV | bpf.BPF_X),
		int(bpf.BPF_ALU | bpf.BPF_MOD | bpf.BPF_X),
		int(bpf.BPF_ALU | bpf.BPF_AND | bpf.BPF_X),
		int(bpf.BPF_ALU | bpf.BPF_OR | bpf.BPF_X),
		int(bpf.BPF_ALU | bpf.BPF_XOR | bpf.BPF_X),
		int(bpf.BPF_ALU | bpf.BPF_LSH | bpf.BPF_X),
		int(bpf.BPF_ALU | bpf.BPF_RSH | bpf.BPF_X):
		op := bpf.Op(uint16(s.Code))
		if alter && os.IsConst(val[XAtom]) {
			if os.IsConst(val[AAtom]) {
				os.foldOp(s, val[AAtom], val[XAtom])
				val[AAtom] = os.K(s.K)
			} else {
				s.Code = int(bpf.BPF_ALU | bpf.BPF_K | op)
				s.K = os.ConstVal(val[XAtom])
				if (op == bpf.BPF_LSH || op == bpf.BPF_RSH) && s.K > 31 {
					os.Err = fmt.Errorf("shift by more than 31 bits")
					return
				}
				os.NonBranchMovementPerformed = true
				os.Done = false
				val[AAtom] = os.F(s.Code, val[AAtom], os.K(s.K))
			}
			return
		}
		// Check for A==0 simplifications
		if alter && os.IsConst(val[AAtom]) && os.ConstVal(val[AAtom]) == 0 {
			if op == bpf.BPF_ADD || op == bpf.BPF_OR || op == bpf.BPF_XOR {
				s.Code = int(bpf.BPF_MISC | bpf.BPF_TXA)
				os.vstore(s, &val[AAtom], val[XAtom], alter)
				return
			} else if op == bpf.BPF_MUL || op == bpf.BPF_DIV || op == bpf.BPF_MOD ||
				op == bpf.BPF_AND || op == bpf.BPF_LSH || op == bpf.BPF_RSH {
				s.Code = int(bpf.BPF_LD | bpf.BPF_IMM)
				s.K = 0
				os.vstore(s, &val[AAtom], os.K(s.K), alter)
				return
			} else if op == bpf.BPF_NEG {
				s.Code = codegen.NOP
				return
			}
		}
		val[AAtom] = os.F(s.Code, val[AAtom], val[XAtom])

	case int(bpf.BPF_MISC | bpf.BPF_TXA):
		os.vstore(s, &val[AAtom], val[XAtom], alter)

	case int(bpf.BPF_LD | bpf.BPF_MEM):
		v := val[s.K]
		if alter && os.IsConst(v) {
			s.Code = int(bpf.BPF_LD | bpf.BPF_IMM)
			s.K = os.ConstVal(v)
			os.NonBranchMovementPerformed = true
			os.Done = false
		}
		os.vstore(s, &val[AAtom], v, alter)

	case int(bpf.BPF_MISC | bpf.BPF_TAX):
		os.vstore(s, &val[XAtom], val[AAtom], alter)

	case int(bpf.BPF_LDX | bpf.BPF_MEM):
		v := val[s.K]
		if alter && os.IsConst(v) {
			s.Code = int(bpf.BPF_LDX | bpf.BPF_IMM)
			s.K = os.ConstVal(v)
			os.NonBranchMovementPerformed = true
			os.Done = false
		}
		os.vstore(s, &val[XAtom], v, alter)

	case int(bpf.BPF_ST):
		os.vstore(s, &val[s.K], val[AAtom], alter)

	case int(bpf.BPF_STX):
		os.vstore(s, &val[s.K], val[XAtom], alter)
	}
}

// thisOp skips NOP statements and returns the next real instruction.
func thisOp(s *codegen.SList) *codegen.SList {
	for s != nil && s.S.Code == codegen.NOP {
		s = s.Next
	}
	return s
}

// optNot swaps the true and false branches of a block.
func optNot(b *codegen.Block) {
	tmp := codegen.JT(b)
	codegen.SetJT(b, codegen.JF(b))
	codegen.SetJF(b, tmp)
}

// OptPeep performs peephole optimization on a block.
// Port of opt_peep() from optimize.c.
func (os *OptState) OptPeep(b *codegen.Block) {
	s := b.Stmts
	if s == nil {
		return
	}

	var last *codegen.SList
	last = s

	for s = thisOp(s); s != nil; {
		next := thisOp(s.Next)
		if next == nil {
			break
		}
		last = next

		// st M[k]; ldx M[k] → st M[k]; tax
		if s.S.Code == int(bpf.BPF_ST) &&
			next.S.Code == int(bpf.BPF_LDX|bpf.BPF_MEM) &&
			s.S.K == next.S.K {
			os.NonBranchMovementPerformed = true
			os.Done = false
			next.S.Code = int(bpf.BPF_MISC | bpf.BPF_TAX)
		}

		// ld #k; tax → ldx #k; txa
		if s.S.Code == int(bpf.BPF_LD|bpf.BPF_IMM) &&
			next.S.Code == int(bpf.BPF_MISC|bpf.BPF_TAX) {
			s.S.Code = int(bpf.BPF_LDX | bpf.BPF_IMM)
			next.S.Code = int(bpf.BPF_MISC | bpf.BPF_TXA)
			os.NonBranchMovementPerformed = true
			os.Done = false
		}

		// ldi #k; [ldxms]; addx; tax; ild → nop; ldxms; nop; nop; ild [x+k]
		if s.S.Code == int(bpf.BPF_LD|bpf.BPF_IMM) {
			if !AtomElem(b.OutUse, XAtom) {
				var add *codegen.SList
				if next.S.Code != int(bpf.BPF_LDX|bpf.BPF_MSH|bpf.BPF_B) {
					add = next
				} else {
					add = thisOp(next.Next)
				}
				if add != nil && add.S.Code == int(bpf.BPF_ALU|bpf.BPF_ADD|bpf.BPF_X) {
					tax := thisOp(add.Next)
					if tax != nil && tax.S.Code == int(bpf.BPF_MISC|bpf.BPF_TAX) {
						ild := thisOp(tax.Next)
						if ild != nil && bpf.Class(uint16(ild.S.Code)) == bpf.BPF_LD &&
							bpf.Mode(uint16(ild.S.Code)) == bpf.BPF_IND {
							ild.S.K += s.S.K
							s.S.Code = codegen.NOP
							add.S.Code = codegen.NOP
							tax.S.Code = codegen.NOP
							os.NonBranchMovementPerformed = true
							os.Done = false
						}
					}
				}
			}
		}

		s = next
	}

	// JEQ #k with preceding ALU operations
	if b.S.Code == int(bpf.BPF_JMP|bpf.BPF_JEQ|bpf.BPF_K) && !AtomElem(b.OutUse, AAtom) {
		if last != nil && last.S.Code == int(bpf.BPF_ALU|bpf.BPF_SUB|bpf.BPF_X) {
			val := b.Val[XAtom]
			if os.IsConst(val) {
				// sub x; jeq #y → nop; jeq #(x+y)
				b.S.K += os.ConstVal(val)
				last.S.Code = codegen.NOP
				os.NonBranchMovementPerformed = true
				os.Done = false
			} else if b.S.K == 0 {
				// sub x; jeq #0 → nop; jeq x
				last.S.Code = codegen.NOP
				b.S.Code = int(bpf.BPF_JMP | bpf.BPF_JEQ | bpf.BPF_X)
				os.NonBranchMovementPerformed = true
				os.Done = false
			}
		} else if last != nil && last.S.Code == int(bpf.BPF_ALU|bpf.BPF_SUB|bpf.BPF_K) {
			// sub #x; jeq #y → nop; jeq #(x+y)
			b.S.K += last.S.K
			last.S.Code = codegen.NOP
			os.NonBranchMovementPerformed = true
			os.Done = false
		} else if last != nil && last.S.Code == int(bpf.BPF_ALU|bpf.BPF_AND|bpf.BPF_K) && b.S.K == 0 {
			// and #k; jeq #0 → nop; jset #k (negated)
			b.S.K = last.S.K
			b.S.Code = int(bpf.BPF_JMP | bpf.BPF_K | bpf.BPF_JSET)
			last.S.Code = codegen.NOP
			os.NonBranchMovementPerformed = true
			os.Done = false
			optNot(b)
		}
	}

	// jset #0 → always false; jset #ffffffff → always true
	if b.S.Code == int(bpf.BPF_JMP|bpf.BPF_K|bpf.BPF_JSET) {
		if b.S.K == 0 {
			codegen.SetJT(b, codegen.JF(b))
		}
		if b.S.K == 0xFFFFFFFF {
			codegen.SetJF(b, codegen.JT(b))
		}
	}

	// If X is a known constant and branch uses X, convert to K
	val := b.Val[XAtom]
	if os.IsConst(val) && bpf.Src(uint16(b.S.Code)) == bpf.BPF_X {
		v := os.ConstVal(val)
		b.S.Code &= ^int(bpf.BPF_X) // clear X bit
		b.S.K = v
	}

	// If A is a known constant, evaluate branch at compile time
	val = b.Val[AAtom]
	if os.IsConst(val) && bpf.Src(uint16(b.S.Code)) == bpf.BPF_K {
		v := os.ConstVal(val)
		switch bpf.Op(uint16(b.S.Code)) {
		case bpf.BPF_JEQ:
			if v == b.S.K {
				v = 1
			} else {
				v = 0
			}
		case bpf.BPF_JGT:
			if v > b.S.K {
				v = 1
			} else {
				v = 0
			}
		case bpf.BPF_JGE:
			if v >= b.S.K {
				v = 1
			} else {
				v = 0
			}
		case bpf.BPF_JSET:
			v &= b.S.K
		default:
			return
		}
		if codegen.JF(b) != codegen.JT(b) {
			os.NonBranchMovementPerformed = true
			os.Done = false
		}
		if v != 0 {
			codegen.SetJF(b, codegen.JT(b))
		} else {
			codegen.SetJT(b, codegen.JF(b))
		}
	}
}

// deadstmt checks if a statement's definition overwrites a previous unused definition.
func (os *OptState) deadstmt(s *codegen.Stmt, last []*codegen.Stmt) {
	atom := Atomuse(s)
	if atom >= 0 {
		if atom == AXAtom {
			last[XAtom] = nil
			last[AAtom] = nil
		} else {
			last[atom] = nil
		}
	}
	atom = Atomdef(s)
	if atom >= 0 {
		if last[atom] != nil {
			os.NonBranchMovementPerformed = true
			os.Done = false
			last[atom].Code = codegen.NOP
		}
		last[atom] = s
	}
}

// OptDeadstores removes dead store statements from a block.
// Port of opt_deadstores() from optimize.c.
func (os *OptState) OptDeadstores(b *codegen.Block) {
	var last [codegen.NAtoms]*codegen.Stmt

	for s := b.Stmts; s != nil; s = s.Next {
		os.deadstmt(&s.S, last[:])
	}
	os.deadstmt(&b.S, last[:])

	for atom := 0; atom < codegen.NAtoms; atom++ {
		if last[atom] != nil && !AtomElem(b.OutUse, atom) {
			last[atom].Code = codegen.NOP
			os.vstore(nil, &b.Val[atom], ValUnknown, false)
			os.NonBranchMovementPerformed = true
			os.Done = false
		}
	}
}
