// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import "fmt"

// Stmt is a single BPF statement in an instruction block.
// Port of struct stmt from gencode.h.
type Stmt struct {
	Code int
	Jt   *SList // relative jump within block (for computed jumps)
	Jf   *SList
	K    uint32
}

// SList is a linked list of statements.
// Port of struct slist from gencode.h.
type SList struct {
	S    Stmt
	Next *SList
}

// Edge is a directed edge in the control flow graph.
// Port of struct edge from gencode.h.
type Edge struct {
	ID   uint
	Code int
	Succ *Block // successor vertex
	Pred *Block // predecessor vertex
	Next *Edge  // linked list of incoming edges for a node
}

// Block is a vertex in the control flow graph.
// It has a list of side-effect statements and a final branch statement.
// Port of struct block from gencode.h.
type Block struct {
	ID     uint
	Stmts  *SList // side-effect statements
	S      Stmt   // branch statement
	Mark   int
	LongJt uint // jt branch requires long jump
	LongJf uint // jf branch requires long jump
	Level  int
	Offset int
	Sense  bool // for negation handling

	Et Edge // edge for jt branch
	Ef Edge // edge for jf branch

	Head    *Block
	Link    *Block // used by optimizer
	InEdges *Edge  // incoming edges

	// Optimizer data flow analysis fields
	Dom     []uint32
	Closure []uint32
	Def     uint32
	Kill    uint32
	InUse   uint32
	OutUse  uint32
	Oval    int
	Val     [NAtoms]uint32
}

// JT returns the true-branch successor block.
func JT(b *Block) *Block { return b.Et.Succ }

// JF returns the false-branch successor block.
func JF(b *Block) *Block { return b.Ef.Succ }

// SetJT sets the true-branch successor.
func SetJT(b *Block, target *Block) { b.Et.Succ = target }

// SetJF sets the false-branch successor.
func SetJF(b *Block, target *Block) { b.Ef.Succ = target }

// Arth represents an arithmetic expression result.
// It carries the protocol checks (b), instruction sequence (s),
// and the virtual register number holding the result.
// Port of struct arth from gencode.h.
type Arth struct {
	B     *Block // protocol checks required before evaluating
	S     *SList // instruction sequence computing the value
	Regno int    // virtual register number holding result
}

// Qual holds the qualifiers for a filter expression:
// address type, protocol, and direction.
// Port of struct qual from gencode.h.
type Qual struct {
	Addr  uint8
	Proto uint8
	Dir   uint8
}

// ICode is the intermediate code representation — a CFG with a root block.
// Port of struct icode from gencode.h.
type ICode struct {
	Root    *Block
	CurMark int
}

// IsMarked returns whether a block is marked in the current traversal.
func (ic *ICode) IsMarked(b *Block) bool { return b.Mark == ic.CurMark }

// UnMarkAll increments the mark counter, effectively unmarking all blocks.
func (ic *ICode) UnMarkAll() { ic.CurMark++ }

// MarkBlock marks a block as visited in the current traversal.
func (ic *ICode) MarkBlock(b *Block) { b.Mark = ic.CurMark }

// OffsetRel describes what an offset is relative to.
type OffsetRel int

const (
	OrPacket      OffsetRel = iota // absolute packet offset
	OrLinkhdr                      // link-layer header
	OrLinktype                     // link-type field
	OrLinkpl                       // link-layer payload
	OrLLC                          // 802.2 LLC header
	OrPrevlinkhdr                  // previous link header (encapsulation)
	OrPrevmplshdr                  // previous MPLS header
	OrLinkplNosnap                 // link payload without SNAP
	OrTranIPv4                     // transport layer (IPv4)
	OrTranIPv6                     // transport layer (IPv6)
)

// AbsOffset tracks an absolute offset in the packet, possibly with
// a variable component stored in a BPF register.
type AbsOffset struct {
	ConstPart    int  // fixed byte offset
	IsVariable   bool // true if there's a variable part
	Reg          int  // BPF register holding variable part (if IsVariable)
}

// CompilerState holds the entire state of a filter compilation.
// Port of struct _compiler_state from gencode.c.
type CompilerState struct {
	IC       ICode
	Snaplen  int
	Linktype int
	Netmask  uint32

	// Link-layer offset tracking
	OffLinkhdr    AbsOffset
	OffLinktype   AbsOffset
	OffLinkpl     AbsOffset
	OffNl         int // network layer offset from link payload
	OffNlNosnap   int // same but without SNAP
	OffLL         AbsOffset
	OffPrevlinkhdr AbsOffset

	// Flags
	IsGeneve    bool
	LabelCount  int
	VLANStackDepth int
	MPLSStackDepth int

	// Register allocation
	RegUsed  [NAtoms]bool
	CurrReg  int

	// Block allocation
	NextBlockID uint

	// Name resolver
	Resolver NameResolver

	// Qualifier stack for inherited attributes (replaces yacc $0 idiom)
	qualStack []Qual

	// Error handling
	Err error
}

// PushQual pushes a qualifier onto the stack (used by head → id inheritance).
func (cs *CompilerState) PushQual(q Qual) {
	cs.qualStack = append(cs.qualStack, q)
}

// PopQual pops the qualifier stack.
func (cs *CompilerState) PopQual() {
	if len(cs.qualStack) > 0 {
		cs.qualStack = cs.qualStack[:len(cs.qualStack)-1]
	}
}

// PeekQual returns the current qualifier from the stack, or a default.
func (cs *CompilerState) PeekQual() Qual {
	if len(cs.qualStack) > 0 {
		return cs.qualStack[len(cs.qualStack)-1]
	}
	return Qual{Addr: QUndef, Proto: QUndef, Dir: QUndef}
}

// NameResolver resolves symbolic names to addresses, ports, and protocols.
type NameResolver interface {
	LookupHost(name string) ([]uint32, error)       // IPv4 addresses
	LookupHost6(name string) ([][16]byte, error)     // IPv6 addresses
	LookupPort(name string, proto int) (int, error)  // service → port
	LookupProto(name string) (int, error)            // protocol name → number
	LookupEProto(name string) (int, error)           // ethernet protocol name → ethertype
	LookupLLC(name string) (int, error)              // LLC name → SAP
	LookupNet(name string) (uint32, uint32, error)   // network name → (addr, mask)
	LookupEther(name string) ([]byte, error)         // hostname → MAC address
	LookupPortRange(name string, proto int) (int, int, error) // port range
}

// NewCompilerState creates a new compiler state for the given link type and snaplen.
func NewCompilerState(linktype, snaplen int, netmask uint32, resolver NameResolver) *CompilerState {
	return &CompilerState{
		Snaplen:  snaplen,
		Linktype: linktype,
		Netmask:  netmask,
		Resolver: resolver,
	}
}

// NewBlock allocates a new block with a unique ID.
func (cs *CompilerState) NewBlock(code int, k uint32) *Block {
	b := &Block{
		ID: cs.NextBlockID,
		S:  Stmt{Code: code, K: k},
	}
	b.Et.Pred = b
	b.Ef.Pred = b
	cs.NextBlockID++
	return b
}

// AllocReg allocates a virtual BPF register. Returns -1 if none available.
func (cs *CompilerState) AllocReg() int {
	for i := 0; i < len(cs.RegUsed); i++ {
		if !cs.RegUsed[i] {
			cs.RegUsed[i] = true
			cs.CurrReg = i
			return i
		}
	}
	cs.SetError(fmt.Errorf("too many registers needed"))
	return -1
}

// FreeReg frees a virtual BPF register.
func (cs *CompilerState) FreeReg(n int) {
	if n >= 0 && n < len(cs.RegUsed) {
		cs.RegUsed[n] = false
	}
}

// SetError sets the compiler error if not already set.
func (cs *CompilerState) SetError(err error) {
	if cs.Err == nil {
		cs.Err = err
	}
}

// Sappend appends s1 to the end of linked list s0.
func Sappend(s0, s1 *SList) {
	if s0 == nil {
		return
	}
	for s0.Next != nil {
		s0 = s0.Next
	}
	s0.Next = s1
}

// NewStmt creates a new single-element SList with the given opcode and k value.
func NewStmt(code int, k uint32) *SList {
	return &SList{S: Stmt{Code: code, K: k}}
}
