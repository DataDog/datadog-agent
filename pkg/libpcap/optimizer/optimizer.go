// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package optimizer

import (
	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
)

// optLoop runs optimization passes in a loop until a fixed point is reached.
// doStmts controls whether statement-level optimization (peephole, dead stores) is applied.
// Port of opt_loop() from optimize.c.
func (os *OptState) optLoop(ic *codegen.ICode, doStmts bool) {
	loopCount := 0
	for {
		os.Done = true
		os.NonBranchMovementPerformed = false

		FindLevels(os, ic)
		FindDom(os, ic.Root)
		FindClosure(os, ic.Root)
		FindUD(os, ic.Root)
		FindEdom(os, ic.Root)
		os.OptBlks(ic, doStmts)

		if os.Err != nil {
			return
		}

		if os.Done {
			break
		}

		if os.NonBranchMovementPerformed {
			loopCount = 0
		} else {
			loopCount++
			if loopCount >= 100 {
				// Probably in a cycle — give up
				os.Done = true
				break
			}
		}
	}
}

// Optimize runs the BPF optimizer on the intermediate code.
// It modifies the CFG in place: constant folding, dead code elimination,
// jump threading, predicate assertion, and block interning.
// Port of bpf_optimize() from optimize.c.
func Optimize(ic *codegen.ICode) error {
	if ic.Root == nil {
		return nil
	}

	os := &OptState{}
	if err := OptInit(os, ic); err != nil {
		return err
	}

	// Pass 1: structural optimization (block-level, no statement changes)
	os.optLoop(ic, false)
	if os.Err != nil {
		return os.Err
	}

	// Pass 2: statement-level optimization (peephole + dead stores)
	os.optLoop(ic, true)
	if os.Err != nil {
		return os.Err
	}

	// Unify identical blocks
	os.InternBlocks(ic)

	// Simplify root
	OptRoot(&ic.Root)

	return nil
}
