package main

import (
	"time"
)

type VerifierStats struct {
	VerificationTime time.Duration
	StackDepth
	InstructionsProcessed
	InstructionsProcessedLimit
	MaxStatesPerInstruction
	TotalStats
	PeakStates int
}

//go:generate go run functions.go ../bytecode/build/co-re
//go:generate go fmt programs.go
func main() {

}
