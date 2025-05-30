package dyninst

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/network/go/dwarfutils/locexpr"
)

func a() {
	// IR for main_test_single_byte
	main_test_single_byte_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_byte",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_byte",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7f40, 0x3d7f50},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7f40, 0x3d7f50},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 1, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x2},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x1,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7f40},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_byte",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7f40, 0x3d7f50},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint8", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7f40, 0x3d7f50},
								Pieces: []locexpr.LocationPiece{
									{Size: 1, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x2},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x1,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_single_byte_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x94, 0x3b, 0x33, 0x88, 0xc3, 0x60, 0x45, 0xe6, 0xd7, 0x8d, 0xfd, 0xa3, 0x3e, 0x6b, 0x0, 0x0, 0x40, 0x7f, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x61, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_single_rune
	main_test_single_rune_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_rune",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_rune",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7f50, 0x3d7f60},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int32", ByteSize: 0x4},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7f50, 0x3d7f60},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 4, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x5},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x4,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7f50},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_rune",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7f50, 0x3d7f60},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int32", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7f50, 0x3d7f60},
								Pieces: []locexpr.LocationPiece{
									{Size: 4, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int32", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x5},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x4,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_single_rune_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0x12, 0x92, 0x42, 0x6a, 0x3d, 0x42, 0x8d, 0x15, 0xd, 0x4e, 0xaa, 0x3e, 0x6b, 0x0, 0x0, 0x50, 0x7f, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x5, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_single_bool
	main_test_single_bool_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_bool",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_bool",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7f60, 0x3d7f70},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "bool", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7f60, 0x3d7f70},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 1, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x2},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x1,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7f60},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_bool",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7f60, 0x3d7f70},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "bool", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7f60, 0x3d7f70},
								Pieces: []locexpr.LocationPiece{
									{Size: 1, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "bool", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x2},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x1,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_single_bool_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x86, 0x68, 0x4a, 0xf0, 0x46, 0x51, 0xed, 0xbd, 0xd3, 0xc4, 0xbe, 0xc7, 0x3e, 0x6b, 0x0, 0x0, 0x60, 0x7f, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_single_int
	main_test_single_int_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_int",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_int",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7f70, 0x3d7f80},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7f70, 0x3d7f80},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x9},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7f70},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_int",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7f70, 0x3d7f80},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7f70, 0x3d7f80},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x9},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_single_int_bytes := []byte{0x60, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xe1, 0x2, 0xaa, 0xd1, 0xd6, 0x76, 0x3d, 0xf, 0xc5, 0x61, 0x7, 0xe6, 0x3e, 0x6b, 0x0, 0x0, 0x70, 0x7f, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x18, 0xfa, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_single_int8
	main_test_single_int8_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_int8",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_int8",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7f80, 0x3d7f90},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45440, GoKind: 0x3},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7f80, 0x3d7f90},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 1, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x2},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x1,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7f80},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_int8",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7f80, 0x3d7f90},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int8", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45440, GoKind: 0x3},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7f80, 0x3d7f90},
								Pieces: []locexpr.LocationPiece{
									{Size: 1, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45440, GoKind: 0x3},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x2},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x1,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_single_int8_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x28, 0xf0, 0x87, 0x9a, 0x54, 0xf1, 0x27, 0x85, 0x9, 0x3b, 0xcd, 0xeb, 0x3e, 0x6b, 0x0, 0x0, 0x80, 0x7f, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0xf8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_single_int16
	main_test_single_int16_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_int16",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_int16",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7f90, 0x3d7fa0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int16", ByteSize: 0x2},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x454c0, GoKind: 0x4},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7f90, 0x3d7fa0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 2, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x3},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x2,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7f90},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_int16",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7f90, 0x3d7fa0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int16", ByteSize: 0x2},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x454c0, GoKind: 0x4},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7f90, 0x3d7fa0},
								Pieces: []locexpr.LocationPiece{
									{Size: 2, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int16", ByteSize: 0x2},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x454c0, GoKind: 0x4},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x3},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x2,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_single_int16_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x73, 0x53, 0x99, 0x4b, 0x64, 0xf6, 0x2, 0xc6, 0xb6, 0xa8, 0xd4, 0xf1, 0x3e, 0x6b, 0x0, 0x0, 0x90, 0x7f, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0xf0, 0xff, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_single_int32
	main_test_single_int32_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_int32",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_int32",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7fa0, 0x3d7fb0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int32", ByteSize: 0x4},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7fa0, 0x3d7fb0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 4, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x5},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x4,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7fa0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_int32",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7fa0, 0x3d7fb0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int32", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7fa0, 0x3d7fb0},
								Pieces: []locexpr.LocationPiece{
									{Size: 4, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int32", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x5},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x4,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_single_int32_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xfa, 0xb3, 0xa5, 0x98, 0x38, 0xa2, 0x82, 0xaa, 0xb7, 0xa0, 0x16, 0xf8, 0x3e, 0x6b, 0x0, 0x0, 0xa0, 0x7f, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x5, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0xe0, 0xff, 0xff, 0xff, 0x0, 0x0, 0x0}

	// IR for main_test_single_int64
	main_test_single_int64_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_int64",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_int64",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7fb0, 0x3d7fc0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int64", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x455c0, GoKind: 0x6},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7fb0, 0x3d7fc0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x9},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7fb0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_int64",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7fb0, 0x3d7fc0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int64", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x455c0, GoKind: 0x6},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7fb0, 0x3d7fc0},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int64", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x455c0, GoKind: 0x6},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x9},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_single_int64_bytes := []byte{0x60, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa5, 0xa6, 0x3f, 0x0, 0xf1, 0xea, 0xd7, 0x75, 0x1d, 0xce, 0x92, 0xfd, 0x3e, 0x6b, 0x0, 0x0, 0xb0, 0x7f, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0xc0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_single_uint
	main_test_single_uint_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_uint",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_uint",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7fc0, 0x3d7fd0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7fc0, 0x3d7fd0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x9},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7fc0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_uint",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7fc0, 0x3d7fd0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7fc0, 0x3d7fd0},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x9},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_single_uint_bytes := []byte{0x60, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xfc, 0x9a, 0xa1, 0x45, 0x89, 0xd9, 0x11, 0xa6, 0x6a, 0xfa, 0x68, 0x3, 0x3f, 0x6b, 0x0, 0x0, 0xc0, 0x7f, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0xe8, 0x5, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_single_uint8
	main_test_single_uint8_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_uint8",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_uint8",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7fd0, 0x3d7fe0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7fd0, 0x3d7fe0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 1, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x2},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x1,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7fd0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_uint8",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7fd0, 0x3d7fe0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint8", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7fd0, 0x3d7fe0},
								Pieces: []locexpr.LocationPiece{
									{Size: 1, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x2},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x1,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_single_uint8_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x97, 0xc6, 0xfb, 0xa3, 0xab, 0x25, 0x2e, 0x75, 0x33, 0x66, 0xea, 0x20, 0x3f, 0x6b, 0x0, 0x0, 0xd0, 0x7f, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_single_uint16
	main_test_single_uint16_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_uint16",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_uint16",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7fe0, 0x3d7ff0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint16", ByteSize: 0x2},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7fe0, 0x3d7ff0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 2, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x3},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x2,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7fe0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_uint16",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7fe0, 0x3d7ff0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint16", ByteSize: 0x2},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7fe0, 0x3d7ff0},
								Pieces: []locexpr.LocationPiece{
									{Size: 2, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint16", ByteSize: 0x2},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x3},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x2,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_single_uint16_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xae, 0xf, 0x86, 0xa0, 0x7e, 0xa8, 0xf1, 0x9c, 0x95, 0x3f, 0xff, 0x26, 0x3f, 0x6b, 0x0, 0x0, 0xe0, 0x7f, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x10, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_single_uint32
	main_test_single_uint32_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_uint32",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_uint32",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7ff0, 0x3d8000},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint32", ByteSize: 0x4},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7ff0, 0x3d8000},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 4, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x5},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x4,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7ff0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_uint32",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7ff0, 0x3d8000},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint32", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7ff0, 0x3d8000},
								Pieces: []locexpr.LocationPiece{
									{Size: 4, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint32", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x5},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x4,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_single_uint32_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x89, 0xad, 0x75, 0x57, 0x71, 0x38, 0xdf, 0x66, 0x74, 0xaf, 0xd6, 0x2c, 0x3f, 0x6b, 0x0, 0x0, 0xf0, 0x7f, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x5, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_single_uint64
	main_test_single_uint64_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_uint64",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_uint64",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d8000, 0x3d8010},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint64", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8000, 0x3d8010},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x9},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d8000},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_uint64",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d8000, 0x3d8010},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint64", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8000, 0x3d8010},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint64", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x9},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_single_uint64_bytes := []byte{0x60, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x70, 0xae, 0xcf, 0x83, 0x6d, 0x43, 0xc0, 0x54, 0x79, 0xb0, 0x9e, 0x32, 0x3f, 0x6b, 0x0, 0x0, 0x0, 0x80, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x40, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_single_float32
	main_test_single_float32_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_float32",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_float32",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d8010, 0x3d8020},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "float32", ByteSize: 0x4},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45780, GoKind: 0xd},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8010, 0x3d8020},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 4, InReg: true, StackOffset: 0, Register: 64},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x5},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x4,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d8010},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_float32",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d8010, 0x3d8020},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "float32", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45780, GoKind: 0xd},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8010, 0x3d8020},
								Pieces: []locexpr.LocationPiece{
									{Size: 4, InReg: true, StackOffset: 0, Register: 64},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "float32", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45780, GoKind: 0xd},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x5},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x4,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_single_float32_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xbb, 0xc4, 0x6e, 0x20, 0x92, 0xc1, 0xc9, 0x6c, 0x7d, 0x5, 0x76, 0x38, 0x3f, 0x6b, 0x0, 0x0, 0x10, 0x80, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x5, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_single_float64
	main_test_single_float64_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_float64",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_float64",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d8020, 0x3d8030},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "float64", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x457c0, GoKind: 0xe},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8020, 0x3d8030},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 64},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x9},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d8020},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_float64",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d8020, 0x3d8030},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "float64", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x457c0, GoKind: 0xe},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8020, 0x3d8030},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 64},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "float64", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x457c0, GoKind: 0xe},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x9},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_single_float64_bytes := []byte{0x60, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x82, 0xc2, 0x13, 0x1e, 0x24, 0x43, 0xf, 0x6, 0x90, 0xe8, 0x14, 0x56, 0x3f, 0x6b, 0x0, 0x0, 0x20, 0x80, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_type_alias
	main_test_type_alias_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_type_alias",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_type_alias",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d8030, 0x3d8040},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.typeAlias", ByteSize: 0x2},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x9},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8030, 0x3d8040},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 2, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x3},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x2,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d8030},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_type_alias",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d8030, 0x3d8040},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.typeAlias", ByteSize: 0x2},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x9},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8030, 0x3d8040},
								Pieces: []locexpr.LocationPiece{
									{Size: 2, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.typeAlias", ByteSize: 0x2},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x9},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x3},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x2,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_type_alias_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa6, 0x97, 0x13, 0xcf, 0x52, 0x37, 0xb2, 0x59, 0xb6, 0x24, 0xb, 0x5c, 0x3f, 0x6b, 0x0, 0x0, 0x30, 0x80, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_single_string
	main_test_single_string_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_single_string",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_single_string",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9c50, 0x3d9c60},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9c50, 0x3d9c60},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x7, Name: "ProbeEvent", ByteSize: 0x11},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x10,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d9c50},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_single_string",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9c50, 0x3d9c60},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "string", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
							Fields: []ir.Field{
								ir.Field{
									Name:   "str",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9c50, 0x3d9c60},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee: &ir.GoStringDataType{
									TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x2: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
				},
			},
			0x3: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x5: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
			},
			0x6: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0x7: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x7, Name: "ProbeEvent", ByteSize: 0x11},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x10,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x7,
	}
	main_test_single_string_bytes := []byte{0x68, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x94, 0xfb, 0x19, 0x33, 0x32, 0x66, 0x26, 0x1c, 0x50, 0x15, 0x0, 0x62, 0x3f, 0x6b, 0x0, 0x0, 0x50, 0x9c, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x98, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7, 0x0, 0x0, 0x0, 0x11, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_three_strings
	main_test_three_strings_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_three_strings",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_three_strings",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9c60, 0x3d9c70},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9c60, 0x3d9c70},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "y",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9c60, 0x3d9c70},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 3},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "z",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9c60, 0x3d9c70},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 4},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 5},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x7, Name: "ProbeEvent", ByteSize: 0x31},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x10,
											},
										},
									},
								},
								&ir.RootExpression{
									Name:   "y",
									Offset: 0x11,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x10,
											},
										},
									},
								},
								&ir.RootExpression{
									Name:   "z",
									Offset: 0x21,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x10,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d9c60},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_three_strings",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9c60, 0x3d9c70},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "string", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
							Fields: []ir.Field{
								ir.Field{
									Name:   "str",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9c60, 0x3d9c70},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "y",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "string", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
							Fields: []ir.Field{
								ir.Field{
									Name:   "str",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9c60, 0x3d9c70},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 2},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 3},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "z",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "string", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
							Fields: []ir.Field{
								ir.Field{
									Name:   "str",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9c60, 0x3d9c70},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 4},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 5},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee: &ir.GoStringDataType{
									TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x2: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
				},
			},
			0x3: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x5: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
			},
			0x6: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0x7: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x7, Name: "ProbeEvent", ByteSize: 0x31},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x10,
								},
							},
						},
					},
					&ir.RootExpression{
						Name:   "y",
						Offset: 0x11,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x10,
								},
							},
						},
					},
					&ir.RootExpression{
						Name:   "z",
						Offset: 0x21,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x10,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x7,
	}
	main_test_three_strings_bytes := []byte{0x88, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5d, 0x56, 0x5f, 0x85, 0xaa, 0x61, 0xb7, 0x43, 0xe6, 0x4a, 0x39, 0x80, 0x3f, 0x6b, 0x0, 0x0, 0x60, 0x9c, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x98, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7, 0x0, 0x0, 0x0, 0x31, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_three_strings_in_struct
	main_test_three_strings_in_struct_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_three_strings_in_struct",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_three_strings_in_struct",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9c70, 0x3d9c90},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "a",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.threeStringStruct", ByteSize: 0x30},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "a",
										Offset: 0x0,
										Type: &ir.GoStringHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
												Fields: []ir.Field{
													ir.Field{
														Name:   "str",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{},
															Pointee: &ir.GoStringDataType{
																TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
															},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
														},
													},
												},
											},
											Data: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "b",
										Offset: 0x10,
										Type: &ir.GoStringHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
												Fields: []ir.Field{
													ir.Field{
														Name:   "str",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{},
															Pointee:          &ir.GoStringDataType{},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
														},
													},
												},
											},
											Data: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "c",
										Offset: 0x20,
										Type: &ir.GoStringHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
												Fields: []ir.Field{
													ir.Field{
														Name:   "str",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{},
															Pointee:          &ir.GoStringDataType{},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
														},
													},
												},
											},
											Data: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9c70, 0x3d9c90},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 3},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 4},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 5},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x8, Name: "ProbeEvent", ByteSize: 0x31},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "a",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x30,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d9c70},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_three_strings_in_struct",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9c70, 0x3d9c90},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "a",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.threeStringStruct", ByteSize: 0x30},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
							Fields: []ir.Field{
								ir.Field{
									Name:   "a",
									Offset: 0x0,
									Type: &ir.GoStringHeaderType{
										StructureType: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
											Fields: []ir.Field{
												ir.Field{
													Name:   "str",
													Offset: 0x0,
													Type: &ir.PointerType{
														TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{},
														Pointee:          &ir.GoStringDataType{},
													},
												},
												ir.Field{
													Name:   "len",
													Offset: 0x8,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
													},
												},
											},
										},
										Data: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
								ir.Field{
									Name:   "b",
									Offset: 0x10,
									Type: &ir.GoStringHeaderType{
										StructureType: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
											Fields: []ir.Field{
												ir.Field{
													Name:   "str",
													Offset: 0x0,
													Type: &ir.PointerType{
														TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{},
														Pointee:          &ir.GoStringDataType{},
													},
												},
												ir.Field{
													Name:   "len",
													Offset: 0x8,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
													},
												},
											},
										},
										Data: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
								ir.Field{
									Name:   "c",
									Offset: 0x20,
									Type: &ir.GoStringHeaderType{
										StructureType: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
											Fields: []ir.Field{
												ir.Field{
													Name:   "str",
													Offset: 0x0,
													Type: &ir.PointerType{
														TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{},
														Pointee:          &ir.GoStringDataType{},
													},
												},
												ir.Field{
													Name:   "len",
													Offset: 0x8,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
													},
												},
											},
										},
										Data: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9c70, 0x3d9c90},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 3},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 4},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 5},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.threeStringStruct", ByteSize: 0x30},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "a",
						Offset: 0x0,
						Type: &ir.GoStringHeaderType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee:          &ir.GoStringDataType{},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Data: &ir.GoStringDataType{
								TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
							},
						},
					},
					ir.Field{
						Name:   "b",
						Offset: 0x10,
						Type: &ir.GoStringHeaderType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee:          &ir.GoStringDataType{},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Data: &ir.GoStringDataType{
								TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
							},
						},
					},
					ir.Field{
						Name:   "c",
						Offset: 0x20,
						Type: &ir.GoStringHeaderType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee:          &ir.GoStringDataType{},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Data: &ir.GoStringDataType{
								TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
							},
						},
					},
				},
			},
			0x2: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee:          &ir.GoStringDataType{},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x3: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
				},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x5: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x6: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
			},
			0x7: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0x8: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x8, Name: "ProbeEvent", ByteSize: 0x31},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "a",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x30,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x8,
	}
	main_test_three_strings_in_struct_bytes := []byte{0x88, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x39, 0xdc, 0x13, 0x4f, 0xbc, 0x92, 0xeb, 0xd8, 0x97, 0xaa, 0xa, 0x9f, 0x3f, 0x6b, 0x0, 0x0, 0x70, 0x9c, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x98, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x8, 0x0, 0x0, 0x0, 0x31, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_three_strings_in_struct_pointer
	main_test_three_strings_in_struct_pointer_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_three_strings_in_struct_pointer",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_three_strings_in_struct_pointer",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9c90, 0x3d9ca0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "a",
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "*main.threeStringStruct", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x16},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "main.threeStringStruct", ByteSize: 0x30},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "a",
											Offset: 0x0,
											Type: &ir.GoStringHeaderType{
												StructureType: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
													Fields: []ir.Field{
														ir.Field{
															Name:   "str",
															Offset: 0x0,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{},
																Pointee: &ir.GoStringDataType{
																	TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
																},
															},
														},
														ir.Field{
															Name:   "len",
															Offset: 0x8,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
															},
														},
													},
												},
												Data: &ir.GoStringDataType{
													TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
												},
											},
										},
										ir.Field{
											Name:   "b",
											Offset: 0x10,
											Type: &ir.GoStringHeaderType{
												StructureType: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
													Fields: []ir.Field{
														ir.Field{
															Name:   "str",
															Offset: 0x0,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{},
																Pointee:          &ir.GoStringDataType{},
															},
														},
														ir.Field{
															Name:   "len",
															Offset: 0x8,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
															},
														},
													},
												},
												Data: &ir.GoStringDataType{
													TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
												},
											},
										},
										ir.Field{
											Name:   "c",
											Offset: 0x20,
											Type: &ir.GoStringHeaderType{
												StructureType: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
													Fields: []ir.Field{
														ir.Field{
															Name:   "str",
															Offset: 0x0,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{},
																Pointee:          &ir.GoStringDataType{},
															},
														},
														ir.Field{
															Name:   "len",
															Offset: 0x8,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
															},
														},
													},
												},
												Data: &ir.GoStringDataType{
													TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
												},
											},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9c90, 0x3d9ca0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x9, Name: "ProbeEvent", ByteSize: 0x9},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "a",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.PointerType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d9c90},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_three_strings_in_struct_pointer",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9c90, 0x3d9ca0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "a",
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "*main.threeStringStruct", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x16},
							Pointee: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "main.threeStringStruct", ByteSize: 0x30},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "a",
										Offset: 0x0,
										Type: &ir.GoStringHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
												Fields: []ir.Field{
													ir.Field{
														Name:   "str",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{},
															Pointee:          &ir.GoStringDataType{},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
														},
													},
												},
											},
											Data: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "b",
										Offset: 0x10,
										Type: &ir.GoStringHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
												Fields: []ir.Field{
													ir.Field{
														Name:   "str",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{},
															Pointee:          &ir.GoStringDataType{},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
														},
													},
												},
											},
											Data: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "c",
										Offset: 0x20,
										Type: &ir.GoStringHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
												Fields: []ir.Field{
													ir.Field{
														Name:   "str",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{},
															Pointee:          &ir.GoStringDataType{},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
														},
													},
												},
											},
											Data: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9c90, 0x3d9ca0},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "*main.threeStringStruct", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x16},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "main.threeStringStruct", ByteSize: 0x30},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "a",
							Offset: 0x0,
							Type: &ir.GoStringHeaderType{
								StructureType: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
									Fields: []ir.Field{
										ir.Field{
											Name:   "str",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{},
												Pointee:          &ir.GoStringDataType{},
											},
										},
										ir.Field{
											Name:   "len",
											Offset: 0x8,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
											},
										},
									},
								},
								Data: &ir.GoStringDataType{
									TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
								},
							},
						},
						ir.Field{
							Name:   "b",
							Offset: 0x10,
							Type: &ir.GoStringHeaderType{
								StructureType: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
									Fields: []ir.Field{
										ir.Field{
											Name:   "str",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{},
												Pointee:          &ir.GoStringDataType{},
											},
										},
										ir.Field{
											Name:   "len",
											Offset: 0x8,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
											},
										},
									},
								},
								Data: &ir.GoStringDataType{
									TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
								},
							},
						},
						ir.Field{
							Name:   "c",
							Offset: 0x20,
							Type: &ir.GoStringHeaderType{
								StructureType: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
									Fields: []ir.Field{
										ir.Field{
											Name:   "str",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{},
												Pointee:          &ir.GoStringDataType{},
											},
										},
										ir.Field{
											Name:   "len",
											Offset: 0x8,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
											},
										},
									},
								},
								Data: &ir.GoStringDataType{
									TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
								},
							},
						},
					},
				},
			},
			0x2: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "main.threeStringStruct", ByteSize: 0x30},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "a",
						Offset: 0x0,
						Type: &ir.GoStringHeaderType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee:          &ir.GoStringDataType{},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Data: &ir.GoStringDataType{
								TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
							},
						},
					},
					ir.Field{
						Name:   "b",
						Offset: 0x10,
						Type: &ir.GoStringHeaderType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee:          &ir.GoStringDataType{},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Data: &ir.GoStringDataType{
								TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
							},
						},
					},
					ir.Field{
						Name:   "c",
						Offset: 0x20,
						Type: &ir.GoStringHeaderType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee:          &ir.GoStringDataType{},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Data: &ir.GoStringDataType{
								TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
							},
						},
					},
				},
			},
			0x3: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee:          &ir.GoStringDataType{},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x4: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
				},
			},
			0x5: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x6: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x7: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
			},
			0x8: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0x9: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x9, Name: "ProbeEvent", ByteSize: 0x9},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "a",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.PointerType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x9,
	}
	main_test_three_strings_in_struct_pointer_bytes := []byte{0x60, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xb7, 0x82, 0xd9, 0xb1, 0x50, 0x8c, 0xf5, 0x6d, 0xc1, 0x9c, 0xf9, 0xbd, 0x3f, 0x6b, 0x0, 0x0, 0x90, 0x9c, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x98, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x58, 0xbd, 0x15, 0x0, 0x40, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_one_string_in_struct_pointer
	main_test_one_string_in_struct_pointer_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_one_string_in_struct_pointer",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_one_string_in_struct_pointer",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9ca0, 0x3d9cb0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "a",
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "*main.oneStringStruct", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x16},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "main.oneStringStruct", ByteSize: 0x10},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "a",
											Offset: 0x0,
											Type: &ir.GoStringHeaderType{
												StructureType: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
													Fields: []ir.Field{
														ir.Field{
															Name:   "str",
															Offset: 0x0,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{},
																Pointee: &ir.GoStringDataType{
																	TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
																},
															},
														},
														ir.Field{
															Name:   "len",
															Offset: 0x8,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
															},
														},
													},
												},
												Data: &ir.GoStringDataType{
													TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
												},
											},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9ca0, 0x3d9cb0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x9, Name: "ProbeEvent", ByteSize: 0x9},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "a",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.PointerType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d9ca0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_one_string_in_struct_pointer",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9ca0, 0x3d9cb0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "a",
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "*main.oneStringStruct", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x16},
							Pointee: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "main.oneStringStruct", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "a",
										Offset: 0x0,
										Type: &ir.GoStringHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
												Fields: []ir.Field{
													ir.Field{
														Name:   "str",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{},
															Pointee:          &ir.GoStringDataType{},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
														},
													},
												},
											},
											Data: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9ca0, 0x3d9cb0},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "*main.oneStringStruct", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x16},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "main.oneStringStruct", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "a",
							Offset: 0x0,
							Type: &ir.GoStringHeaderType{
								StructureType: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
									Fields: []ir.Field{
										ir.Field{
											Name:   "str",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{},
												Pointee:          &ir.GoStringDataType{},
											},
										},
										ir.Field{
											Name:   "len",
											Offset: 0x8,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
											},
										},
									},
								},
								Data: &ir.GoStringDataType{
									TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
								},
							},
						},
					},
				},
			},
			0x2: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "main.oneStringStruct", ByteSize: 0x10},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "a",
						Offset: 0x0,
						Type: &ir.GoStringHeaderType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee:          &ir.GoStringDataType{},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Data: &ir.GoStringDataType{
								TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
							},
						},
					},
				},
			},
			0x3: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee:          &ir.GoStringDataType{},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x4: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
				},
			},
			0x5: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x6: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x7: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
			},
			0x8: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0x9: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x9, Name: "ProbeEvent", ByteSize: 0x9},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "a",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.PointerType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x9,
	}
	main_test_one_string_in_struct_pointer_bytes := []byte{0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x87, 0x62, 0xb3, 0x60, 0x83, 0x6b, 0xba, 0xf0, 0x40, 0x57, 0xde, 0x3f, 0x6b, 0x0, 0x0, 0xa0, 0x9c, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x98, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x48, 0x3d, 0xe, 0x0, 0x40, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x10, 0x0, 0x0, 0x0, 0x48, 0x3d, 0xe, 0x0, 0x40, 0x0, 0x0, 0x0, 0xbb, 0xd2, 0x53, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_byte_array
	main_test_byte_array_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_byte_array",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_byte_array",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7b10, 0x3d7b20},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint8", ByteSize: 0x2},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d420, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint8", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7b10, 0x3d7b20},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 2, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x3},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x2,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7b10},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_byte_array",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7b10, 0x3d7b20},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint8", ByteSize: 0x2},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d420, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7b10, 0x3d7b20},
								Pieces: []locexpr.LocationPiece{
									{Size: 2, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint8", ByteSize: 0x2},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d420, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x3},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x2,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_byte_array_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0xc5, 0x8b, 0x84, 0x3c, 0x9, 0xdb, 0xf6, 0x41, 0x98, 0x4, 0x0, 0x40, 0x6b, 0x0, 0x0, 0x10, 0x7b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_rune_array
	main_test_rune_array_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_rune_array",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_rune_array",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7b20, 0x3d7b30},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int32", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d480, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int32", ByteSize: 0x4},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7b20, 0x3d7b30},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x9},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7b20},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_rune_array",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7b20, 0x3d7b30},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int32", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d480, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int32", ByteSize: 0x4},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7b20, 0x3d7b30},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int32", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d480, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int32", ByteSize: 0x4},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int32", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x9},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_rune_array_bytes := []byte{0x60, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x32, 0xa7, 0xcc, 0x5f, 0xde, 0xf3, 0x4, 0x97, 0xee, 0x6, 0xf7, 0x5, 0x40, 0x6b, 0x0, 0x0, 0x20, 0x7b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_string_array
	main_test_string_array_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_string_array",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_string_array",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7b30, 0x3d7b40},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]string", ByteSize: 0x20},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d4e0, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.GoStringHeaderType{
									StructureType: &ir.StructureType{
										TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
										Fields: []ir.Field{
											ir.Field{
												Name:   "str",
												Offset: 0x0,
												Type: &ir.PointerType{
													TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{},
													Pointee: &ir.GoStringDataType{
														TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
													},
												},
											},
											ir.Field{
												Name:   "len",
												Offset: 0x8,
												Type: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
												},
											},
										},
									},
									Data: &ir.GoStringDataType{
										TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7b30, 0x3d7b40},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 32, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x8, Name: "ProbeEvent", ByteSize: 0x21},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x20,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7b30},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_string_array",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7b30, 0x3d7b40},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]string", ByteSize: 0x20},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d4e0, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.GoStringHeaderType{
								StructureType: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
									Fields: []ir.Field{
										ir.Field{
											Name:   "str",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{},
												Pointee:          &ir.GoStringDataType{},
											},
										},
										ir.Field{
											Name:   "len",
											Offset: 0x8,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
											},
										},
									},
								},
								Data: &ir.GoStringDataType{
									TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7b30, 0x3d7b40},
								Pieces: []locexpr.LocationPiece{
									{Size: 32, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]string", ByteSize: 0x20},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d4e0, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.GoStringHeaderType{
					StructureType: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
						Fields: []ir.Field{
							ir.Field{
								Name:   "str",
								Offset: 0x0,
								Type: &ir.PointerType{
									TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{},
									Pointee:          &ir.GoStringDataType{},
								},
							},
							ir.Field{
								Name:   "len",
								Offset: 0x8,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
								},
							},
						},
					},
					Data: &ir.GoStringDataType{
						TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
					},
				},
			},
			0x2: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee:          &ir.GoStringDataType{},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x3: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
				},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x5: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x6: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
			},
			0x7: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0x8: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x8, Name: "ProbeEvent", ByteSize: 0x21},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x20,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x8,
	}
	main_test_string_array_bytes := []byte{0x78, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3d, 0x59, 0xab, 0xd, 0x12, 0x24, 0xa1, 0xb6, 0x1f, 0x75, 0x9e, 0x25, 0x40, 0x6b, 0x0, 0x0, 0x30, 0x7b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x8, 0x0, 0x0, 0x0, 0x21, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_bool_array
	main_test_bool_array_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_bool_array",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_bool_array",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7b40, 0x3d7b50},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]bool", ByteSize: 0x2},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "bool", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7b40, 0x3d7b50},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 2, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x3},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x2,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7b40},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_bool_array",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7b40, 0x3d7b50},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]bool", ByteSize: 0x2},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "bool", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7b40, 0x3d7b50},
								Pieces: []locexpr.LocationPiece{
									{Size: 2, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]bool", ByteSize: 0x2},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "bool", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "bool", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x3},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x2,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_bool_array_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xdc, 0x70, 0x9b, 0xc, 0xb8, 0x51, 0xbf, 0xca, 0x49, 0x4a, 0x96, 0x2b, 0x40, 0x6b, 0x0, 0x0, 0x40, 0x7b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_int_array
	main_test_int_array_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_int_array",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_int_array",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7b50, 0x3d7b60},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d540, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7b50, 0x3d7b60},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 16, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x11},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x10,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7b50},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_int_array",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7b50, 0x3d7b60},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d540, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7b50, 0x3d7b60},
								Pieces: []locexpr.LocationPiece{
									{Size: 16, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int", ByteSize: 0x10},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d540, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int", ByteSize: 0x8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x11},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x10,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_int_array_bytes := []byte{0x68, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1f, 0x64, 0xe9, 0xf5, 0x95, 0x27, 0xfe, 0x81, 0x5d, 0xe4, 0xb9, 0x31, 0x40, 0x6b, 0x0, 0x0, 0x50, 0x7b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x11, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_int8_array
	main_test_int8_array_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_int8_array",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_int8_array",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7b60, 0x3d7b70},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int8", ByteSize: 0x2},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int8", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45440, GoKind: 0x3},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7b60, 0x3d7b70},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 2, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x3},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x2,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7b60},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_int8_array",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7b60, 0x3d7b70},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int8", ByteSize: 0x2},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45440, GoKind: 0x3},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7b60, 0x3d7b70},
								Pieces: []locexpr.LocationPiece{
									{Size: 2, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int8", ByteSize: 0x2},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45440, GoKind: 0x3},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45440, GoKind: 0x3},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x3},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x2,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_int8_array_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7e, 0xfe, 0x8, 0xb4, 0xac, 0xdc, 0x27, 0xcd, 0x25, 0x14, 0xbf, 0x37, 0x40, 0x6b, 0x0, 0x0, 0x60, 0x7b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_int16_array
	main_test_int16_array_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_int16_array",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_int16_array",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7b70, 0x3d7b80},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int16", ByteSize: 0x4},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int16", ByteSize: 0x2},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x454c0, GoKind: 0x4},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7b70, 0x3d7b80},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 4, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x5},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x4,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7b70},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_int16_array",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7b70, 0x3d7b80},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int16", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int16", ByteSize: 0x2},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x454c0, GoKind: 0x4},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7b70, 0x3d7b80},
								Pieces: []locexpr.LocationPiece{
									{Size: 4, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int16", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int16", ByteSize: 0x2},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x454c0, GoKind: 0x4},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int16", ByteSize: 0x2},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x454c0, GoKind: 0x4},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x5},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x4,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_int16_array_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xda, 0x50, 0x9b, 0xf5, 0x43, 0x45, 0xf9, 0x6f, 0x8b, 0x1, 0x27, 0x5b, 0x40, 0x6b, 0x0, 0x0, 0x70, 0x7b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x5, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_int32_array
	main_test_int32_array_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_int32_array",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_int32_array",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7b80, 0x3d7b90},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int32", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d480, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int32", ByteSize: 0x4},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7b80, 0x3d7b90},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x9},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7b80},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_int32_array",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7b80, 0x3d7b90},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int32", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d480, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int32", ByteSize: 0x4},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7b80, 0x3d7b90},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int32", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d480, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int32", ByteSize: 0x4},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int32", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x9},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_int32_array_bytes := []byte{0x60, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa5, 0x4b, 0xa4, 0xec, 0x6d, 0xa6, 0xe1, 0x78, 0x64, 0x4d, 0xce, 0x7a, 0x40, 0x6b, 0x0, 0x0, 0x80, 0x7b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_int64_array
	main_test_int64_array_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_int64_array",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_int64_array",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7b90, 0x3d7ba0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int64", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int64", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x455c0, GoKind: 0x6},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7b90, 0x3d7ba0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 16, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x11},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x10,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7b90},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_int64_array",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7b90, 0x3d7ba0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int64", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int64", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x455c0, GoKind: 0x6},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7b90, 0x3d7ba0},
								Pieces: []locexpr.LocationPiece{
									{Size: 16, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]int64", ByteSize: 0x10},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int64", ByteSize: 0x8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x455c0, GoKind: 0x6},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "int64", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x455c0, GoKind: 0x6},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x11},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x10,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_int64_array_bytes := []byte{0x68, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xe4, 0xa6, 0xb1, 0xd6, 0x7c, 0x5f, 0x36, 0xa5, 0x20, 0x23, 0xd1, 0x80, 0x40, 0x6b, 0x0, 0x0, 0x90, 0x7b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x11, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_uint_array
	main_test_uint_array_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_uint_array",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_uint_array",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7ba0, 0x3d7bb0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7ba0, 0x3d7bb0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 16, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x11},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x10,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7ba0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_uint_array",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7ba0, 0x3d7bb0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7ba0, 0x3d7bb0},
								Pieces: []locexpr.LocationPiece{
									{Size: 16, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint", ByteSize: 0x10},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint", ByteSize: 0x8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x11},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x10,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_uint_array_bytes := []byte{0x68, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc7, 0x58, 0x6e, 0x92, 0x99, 0x66, 0x80, 0x65, 0xc6, 0x88, 0xd4, 0xa0, 0x40, 0x6b, 0x0, 0x0, 0xa0, 0x7b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x11, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_uint8_array
	main_test_uint8_array_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_uint8_array",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_uint8_array",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7bb0, 0x3d7bc0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint8", ByteSize: 0x2},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d420, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint8", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7bb0, 0x3d7bc0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 2, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x3},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x2,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7bb0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_uint8_array",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7bb0, 0x3d7bc0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint8", ByteSize: 0x2},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d420, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7bb0, 0x3d7bc0},
								Pieces: []locexpr.LocationPiece{
									{Size: 2, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint8", ByteSize: 0x2},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d420, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x3},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x2,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_uint8_array_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa6, 0x56, 0x64, 0x1b, 0x70, 0x27, 0xcd, 0x9f, 0x6e, 0x92, 0xe6, 0xa6, 0x40, 0x6b, 0x0, 0x0, 0xb0, 0x7b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_uint16_array
	main_test_uint16_array_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_uint16_array",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_uint16_array",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7bc0, 0x3d7bd0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint16", ByteSize: 0x4},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint16", ByteSize: 0x2},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7bc0, 0x3d7bd0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 4, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x5},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x4,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7bc0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_uint16_array",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7bc0, 0x3d7bd0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint16", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint16", ByteSize: 0x2},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7bc0, 0x3d7bd0},
								Pieces: []locexpr.LocationPiece{
									{Size: 4, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint16", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint16", ByteSize: 0x2},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint16", ByteSize: 0x2},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x5},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x4,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_uint16_array_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x81, 0xf0, 0x47, 0xcb, 0x68, 0x81, 0x80, 0x3d, 0xc2, 0xa2, 0x47, 0xc7, 0x40, 0x6b, 0x0, 0x0, 0xc0, 0x7b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x5, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_uint32_array
	main_test_uint32_array_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_uint32_array",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_uint32_array",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7bd0, 0x3d7be0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint32", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint32", ByteSize: 0x4},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7bd0, 0x3d7be0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x9},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7bd0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_uint32_array",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7bd0, 0x3d7be0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint32", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint32", ByteSize: 0x4},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7bd0, 0x3d7be0},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint32", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint32", ByteSize: 0x4},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint32", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x9},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_uint32_array_bytes := []byte{0x60, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xb0, 0x36, 0x29, 0x4b, 0x7b, 0x2, 0x99, 0x40, 0x93, 0xc9, 0x5a, 0xcd, 0x40, 0x6b, 0x0, 0x0, 0xd0, 0x7b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_uint64_array
	main_test_uint64_array_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_uint64_array",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_uint64_array",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7be0, 0x3d7bf0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint64", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d5a0, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint64", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7be0, 0x3d7bf0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 16, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x11},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x10,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7be0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_uint64_array",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7be0, 0x3d7bf0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint64", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d5a0, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint64", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7be0, 0x3d7bf0},
								Pieces: []locexpr.LocationPiece{
									{Size: 16, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]uint64", ByteSize: 0x10},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d5a0, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint64", ByteSize: 0x8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint64", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x11},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x10,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_uint64_array_bytes := []byte{0x68, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x83, 0x45, 0xbe, 0x31, 0xd4, 0x77, 0x7d, 0x33, 0x60, 0xa6, 0x6f, 0xd3, 0x40, 0x6b, 0x0, 0x0, 0xe0, 0x7b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x11, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_array_of_arrays
	main_test_array_of_arrays_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_array_of_arrays",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_array_of_arrays",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7bf0, 0x3d7c00},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "a",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2][2]int", ByteSize: 0x20},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.ArrayType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "[2]int", ByteSize: 0x10},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d540, GoKind: 0x11},
									Count:            0x2,
									HasCount:         true,
									Element: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7bf0, 0x3d7c00},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 32, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x4, Name: "ProbeEvent", ByteSize: 0x21},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "a",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x20,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7bf0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_array_of_arrays",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7bf0, 0x3d7c00},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "a",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2][2]int", ByteSize: 0x20},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "[2]int", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d540, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "int", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7bf0, 0x3d7c00},
								Pieces: []locexpr.LocationPiece{
									{Size: 32, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2][2]int", ByteSize: 0x20},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.ArrayType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "[2]int", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d540, GoKind: 0x11},
					Count:            0x2,
					HasCount:         true,
					Element: &ir.BaseType{
						TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "int", ByteSize: 0x8},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
					},
				},
			},
			0x2: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "[2]int", ByteSize: 0x10},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d540, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "int", ByteSize: 0x8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
				},
			},
			0x3: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x4: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x4, Name: "ProbeEvent", ByteSize: 0x21},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "a",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x20,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x4,
	}
	main_test_array_of_arrays_bytes := []byte{0x78, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xb2, 0x2e, 0x40, 0x8a, 0xe, 0xc4, 0x35, 0x7, 0xee, 0x51, 0xf9, 0xf7, 0x40, 0x6b, 0x0, 0x0, 0xf0, 0x7b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4, 0x0, 0x0, 0x0, 0x21, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_array_of_strings
	main_test_array_of_strings_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_array_of_strings",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_array_of_strings",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7c00, 0x3d7c10},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "a",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]string", ByteSize: 0x20},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d4e0, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.GoStringHeaderType{
									StructureType: &ir.StructureType{
										TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
										Fields: []ir.Field{
											ir.Field{
												Name:   "str",
												Offset: 0x0,
												Type: &ir.PointerType{
													TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{},
													Pointee: &ir.GoStringDataType{
														TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
													},
												},
											},
											ir.Field{
												Name:   "len",
												Offset: 0x8,
												Type: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
												},
											},
										},
									},
									Data: &ir.GoStringDataType{
										TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7c00, 0x3d7c10},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 32, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x8, Name: "ProbeEvent", ByteSize: 0x21},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "a",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x20,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7c00},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_array_of_strings",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7c00, 0x3d7c10},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "a",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]string", ByteSize: 0x20},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d4e0, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.GoStringHeaderType{
								StructureType: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
									Fields: []ir.Field{
										ir.Field{
											Name:   "str",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{},
												Pointee:          &ir.GoStringDataType{},
											},
										},
										ir.Field{
											Name:   "len",
											Offset: 0x8,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
											},
										},
									},
								},
								Data: &ir.GoStringDataType{
									TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7c00, 0x3d7c10},
								Pieces: []locexpr.LocationPiece{
									{Size: 32, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]string", ByteSize: 0x20},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d4e0, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.GoStringHeaderType{
					StructureType: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
						Fields: []ir.Field{
							ir.Field{
								Name:   "str",
								Offset: 0x0,
								Type: &ir.PointerType{
									TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{},
									Pointee:          &ir.GoStringDataType{},
								},
							},
							ir.Field{
								Name:   "len",
								Offset: 0x8,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
								},
							},
						},
					},
					Data: &ir.GoStringDataType{
						TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
					},
				},
			},
			0x2: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee:          &ir.GoStringDataType{},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x3: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
				},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x5: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x6: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
			},
			0x7: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0x8: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x8, Name: "ProbeEvent", ByteSize: 0x21},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "a",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x20,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x8,
	}
	main_test_array_of_strings_bytes := []byte{0x78, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5d, 0x92, 0x83, 0x11, 0x9d, 0x58, 0x45, 0xa8, 0xd1, 0xdc, 0x18, 0x18, 0x41, 0x6b, 0x0, 0x0, 0x0, 0x7c, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x8, 0x0, 0x0, 0x0, 0x21, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_array_of_arrays_of_arrays
	main_test_array_of_arrays_of_arrays_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_array_of_arrays_of_arrays",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_array_of_arrays_of_arrays",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7c10, 0x3d7c20},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "b",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2][2][2]int", ByteSize: 0x40},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.ArrayType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "[2][2]int", ByteSize: 0x20},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
									Count:            0x2,
									HasCount:         true,
									Element: &ir.ArrayType{
										TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "[2]int", ByteSize: 0x10},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d540, GoKind: 0x11},
										Count:            0x2,
										HasCount:         true,
										Element: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7c10, 0x3d7c20},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 64, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x5, Name: "ProbeEvent", ByteSize: 0x41},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "b",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x40,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7c10},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_array_of_arrays_of_arrays",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7c10, 0x3d7c20},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "b",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2][2][2]int", ByteSize: 0x40},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "[2][2]int", ByteSize: 0x20},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.ArrayType{
									TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "[2]int", ByteSize: 0x10},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d540, GoKind: 0x11},
									Count:            0x2,
									HasCount:         true,
									Element: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7c10, 0x3d7c20},
								Pieces: []locexpr.LocationPiece{
									{Size: 64, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2][2][2]int", ByteSize: 0x40},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.ArrayType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "[2][2]int", ByteSize: 0x20},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
					Count:            0x2,
					HasCount:         true,
					Element: &ir.ArrayType{
						TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "[2]int", ByteSize: 0x10},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d540, GoKind: 0x11},
						Count:            0x2,
						HasCount:         true,
						Element: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
						},
					},
				},
			},
			0x2: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "[2][2]int", ByteSize: 0x20},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.ArrayType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "[2]int", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d540, GoKind: 0x11},
					Count:            0x2,
					HasCount:         true,
					Element: &ir.BaseType{
						TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
					},
				},
			},
			0x3: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "[2]int", ByteSize: 0x10},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d540, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
				},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x5: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x5, Name: "ProbeEvent", ByteSize: 0x41},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "b",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x40,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x5,
	}
	main_test_array_of_arrays_of_arrays_bytes := []byte{0x98, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xfc, 0xe7, 0xd2, 0xf1, 0x81, 0x8d, 0xda, 0x92, 0x8b, 0x85, 0xd6, 0x39, 0x41, 0x6b, 0x0, 0x0, 0x10, 0x7c, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5, 0x0, 0x0, 0x0, 0x41, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_array_of_structs
	main_test_array_of_structs_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_array_of_structs",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_array_of_structs",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d7c20, 0x3d7c30},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "a",
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]main.nestedStruct", ByteSize: 0x30},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
								Count:            0x2,
								HasCount:         true,
								Element: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "main.nestedStruct", ByteSize: 0x18},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x9e460, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "anotherInt",
											Offset: 0x0,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "int", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
											},
										},
										ir.Field{
											Name:   "anotherString",
											Offset: 0x8,
											Type: &ir.GoStringHeaderType{
												StructureType: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "string", ByteSize: 0x10},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
													Fields: []ir.Field{
														ir.Field{
															Name:   "str",
															Offset: 0x0,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{},
																Pointee: &ir.GoStringDataType{
																	TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
																},
															},
														},
														ir.Field{
															Name:   "len",
															Offset: 0x8,
															Type:   &ir.BaseType{},
														},
													},
												},
												Data: &ir.GoStringDataType{
													TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
												},
											},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d7c20, 0x3d7c30},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 48, InReg: false, StackOffset: 16, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x9, Name: "ProbeEvent", ByteSize: 0x31},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "a",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.ArrayType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x30,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d7c20},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_array_of_structs",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d7c20, 0x3d7c30},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "a",
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]main.nestedStruct", ByteSize: 0x30},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
							Count:            0x2,
							HasCount:         true,
							Element: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "main.nestedStruct", ByteSize: 0x18},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x9e460, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "anotherInt",
										Offset: 0x0,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
									ir.Field{
										Name:   "anotherString",
										Offset: 0x8,
										Type: &ir.GoStringHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "string", ByteSize: 0x10},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
												Fields: []ir.Field{
													ir.Field{
														Name:   "str",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{},
															Pointee:          &ir.GoStringDataType{},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type:   &ir.BaseType{},
													},
												},
											},
											Data: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d7c20, 0x3d7c30},
								Pieces: []locexpr.LocationPiece{
									{Size: 48, InReg: false, StackOffset: 16, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[2]main.nestedStruct", ByteSize: 0x30},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x11},
				Count:            0x2,
				HasCount:         true,
				Element: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "main.nestedStruct", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x9e460, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "anotherInt",
							Offset: 0x0,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
						ir.Field{
							Name:   "anotherString",
							Offset: 0x8,
							Type: &ir.GoStringHeaderType{
								StructureType: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "string", ByteSize: 0x10},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
									Fields: []ir.Field{
										ir.Field{
											Name:   "str",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{},
												Pointee:          &ir.GoStringDataType{},
											},
										},
										ir.Field{
											Name:   "len",
											Offset: 0x8,
											Type:   &ir.BaseType{},
										},
									},
								},
								Data: &ir.GoStringDataType{
									TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
								},
							},
						},
					},
				},
			},
			0x2: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "main.nestedStruct", ByteSize: 0x18},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x9e460, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "anotherInt",
						Offset: 0x0,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "int", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
						},
					},
					ir.Field{
						Name:   "anotherString",
						Offset: 0x8,
						Type: &ir.GoStringHeaderType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee:          &ir.GoStringDataType{},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type:   &ir.BaseType{},
									},
								},
							},
							Data: &ir.GoStringDataType{
								TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
							},
						},
					},
				},
			},
			0x3: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x4: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee:          &ir.GoStringDataType{},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type:   &ir.BaseType{},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x5: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "uint8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
				},
			},
			0x6: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x7: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x7, Name: "string.str", ByteSize: 0x0},
			},
			0x8: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0x9: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x9, Name: "ProbeEvent", ByteSize: 0x31},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "a",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.ArrayType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x30,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x9,
	}
	main_test_array_of_structs_bytes := []byte{0x88, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff, 0x51, 0x51, 0x6e, 0xab, 0xde, 0xbf, 0x65, 0xa0, 0x38, 0xe7, 0x59, 0x41, 0x6b, 0x0, 0x0, 0x20, 0x7c, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x31, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_uint_slice
	main_test_uint_slice_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_uint_slice",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_uint_slice",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d97e0, 0x3d97f0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "u",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]uint", ByteSize: 0x18},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x395c0, GoKind: 0x17},
								Fields: []ir.Field{
									ir.Field{
										Name:   "array",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*uint", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
											Pointee: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
									ir.Field{
										Name:   "cap",
										Offset: 0x10,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d97e0, 0x3d97f0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x7, Name: "ProbeEvent", ByteSize: 0x19},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "u",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x18,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d97e0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_uint_slice",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d97e0, 0x3d97f0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "u",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]uint", ByteSize: 0x18},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x395c0, GoKind: 0x17},
							Fields: []ir.Field{
								ir.Field{
									Name:   "array",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*uint", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
										Pointee: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
								ir.Field{
									Name:   "cap",
									Offset: 0x10,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d97e0, 0x3d97f0},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.GoSliceHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]uint", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x395c0, GoKind: 0x17},
					Fields: []ir.Field{
						ir.Field{
							Name:   "array",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*uint", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
								Pointee: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
						ir.Field{
							Name:   "cap",
							Offset: 0x10,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x5, Name: "[]uint.array", ByteSize: 0x0},
					Element: &ir.BaseType{
						TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint", ByteSize: 0x8},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
					},
				},
			},
			0x2: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*uint", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint", ByteSize: 0x8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
				},
			},
			0x3: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x5: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x5, Name: "[]uint.array", ByteSize: 0x0},
				Element:    &ir.BaseType{},
			},
			0x6: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*[]uint.array", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoSliceDataType{},
			},
			0x7: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x7, Name: "ProbeEvent", ByteSize: 0x19},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "u",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x18,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x7,
	}
	main_test_uint_slice_bytes := []byte{0x88, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x92, 0x9c, 0xb7, 0xcd, 0x64, 0xd, 0xb9, 0x35, 0x2f, 0xbb, 0x79, 0x41, 0x6b, 0x0, 0x0, 0xe0, 0x97, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa0, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7, 0x0, 0x0, 0x0, 0x19, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x80, 0x8, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_empty_slice
	main_test_empty_slice_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_empty_slice",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_empty_slice",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d97f0, 0x3d9800},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "u",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]uint", ByteSize: 0x18},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x395c0, GoKind: 0x17},
								Fields: []ir.Field{
									ir.Field{
										Name:   "array",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*uint", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
											Pointee: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
									ir.Field{
										Name:   "cap",
										Offset: 0x10,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d97f0, 0x3d9800},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x7, Name: "ProbeEvent", ByteSize: 0x19},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "u",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x18,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d97f0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_empty_slice",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d97f0, 0x3d9800},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "u",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]uint", ByteSize: 0x18},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x395c0, GoKind: 0x17},
							Fields: []ir.Field{
								ir.Field{
									Name:   "array",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*uint", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
										Pointee: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
								ir.Field{
									Name:   "cap",
									Offset: 0x10,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d97f0, 0x3d9800},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.GoSliceHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]uint", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x395c0, GoKind: 0x17},
					Fields: []ir.Field{
						ir.Field{
							Name:   "array",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*uint", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
								Pointee: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
						ir.Field{
							Name:   "cap",
							Offset: 0x10,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x5, Name: "[]uint.array", ByteSize: 0x0},
					Element: &ir.BaseType{
						TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint", ByteSize: 0x8},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
					},
				},
			},
			0x2: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*uint", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint", ByteSize: 0x8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
				},
			},
			0x3: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x5: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x5, Name: "[]uint.array", ByteSize: 0x0},
				Element:    &ir.BaseType{},
			},
			0x6: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*[]uint.array", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoSliceDataType{},
			},
			0x7: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x7, Name: "ProbeEvent", ByteSize: 0x19},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "u",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x18,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x7,
	}
	main_test_empty_slice_bytes := []byte{0x70, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xc3, 0xab, 0x80, 0xad, 0xba, 0xc9, 0x60, 0x35, 0x96, 0xe1, 0x1e, 0x98, 0x41, 0x6b, 0x0, 0x0, 0xf0, 0x97, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa0, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7, 0x0, 0x0, 0x0, 0x19, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_slice_of_slices
	main_test_slice_of_slices_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_slice_of_slices",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_slice_of_slices",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9800, 0x3d9810},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "u",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[][]uint", ByteSize: 0x18},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x17},
								Fields: []ir.Field{
									ir.Field{
										Name:   "array",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*[]uint", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.GoSliceHeaderType{
												StructureType: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "[]uint", ByteSize: 0x18},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x395c0, GoKind: 0x17},
													Fields: []ir.Field{
														ir.Field{
															Name:   "array",
															Offset: 0x0,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*uint", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
																Pointee: &ir.BaseType{
																	TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint", ByteSize: 0x8},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
																},
															},
														},
														ir.Field{
															Name:   "len",
															Offset: 0x8,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
															},
														},
														ir.Field{
															Name:   "cap",
															Offset: 0x10,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
															},
														},
													},
												},
												Data: &ir.GoSliceDataType{
													TypeCommon: ir.TypeCommon{ID: 0x9, Name: "[]uint.array", ByteSize: 0x0},
													Element: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
									ir.Field{
										Name:   "cap",
										Offset: 0x10,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9800, 0x3d9810},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0xb, Name: "ProbeEvent", ByteSize: 0x19},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "u",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x18,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d9800},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_slice_of_slices",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9800, 0x3d9810},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "u",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[][]uint", ByteSize: 0x18},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x17},
							Fields: []ir.Field{
								ir.Field{
									Name:   "array",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*[]uint", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.GoSliceHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "[]uint", ByteSize: 0x18},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x395c0, GoKind: 0x17},
												Fields: []ir.Field{
													ir.Field{
														Name:   "array",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*uint", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
															Pointee: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
															},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "cap",
														Offset: 0x10,
														Type:   &ir.BaseType{},
													},
												},
											},
											Data: &ir.GoSliceDataType{
												TypeCommon: ir.TypeCommon{ID: 0x9, Name: "[]uint.array", ByteSize: 0x0},
												Element: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
												},
											},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
								ir.Field{
									Name:   "cap",
									Offset: 0x10,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9800, 0x3d9810},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.GoSliceHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[][]uint", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x17},
					Fields: []ir.Field{
						ir.Field{
							Name:   "array",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*[]uint", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee: &ir.GoSliceHeaderType{
									StructureType: &ir.StructureType{
										TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "[]uint", ByteSize: 0x18},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x395c0, GoKind: 0x17},
										Fields: []ir.Field{
											ir.Field{
												Name:   "array",
												Offset: 0x0,
												Type: &ir.PointerType{
													TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*uint", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
													Pointee:          &ir.BaseType{},
												},
											},
											ir.Field{
												Name:   "len",
												Offset: 0x8,
												Type:   &ir.BaseType{},
											},
											ir.Field{
												Name:   "cap",
												Offset: 0x10,
												Type:   &ir.BaseType{},
											},
										},
									},
									Data: &ir.GoSliceDataType{
										TypeCommon: ir.TypeCommon{ID: 0x9, Name: "[]uint.array", ByteSize: 0x0},
										Element: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
						ir.Field{
							Name:   "cap",
							Offset: 0x10,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x7, Name: "[][]uint.array", ByteSize: 0x0},
					Element: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "[]uint", ByteSize: 0x18},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x395c0, GoKind: 0x17},
						Fields: []ir.Field{
							ir.Field{
								Name:   "array",
								Offset: 0x0,
								Type: &ir.PointerType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*uint", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
									Pointee: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
									},
								},
							},
							ir.Field{
								Name:   "len",
								Offset: 0x8,
								Type:   &ir.BaseType{},
							},
							ir.Field{
								Name:   "cap",
								Offset: 0x10,
								Type:   &ir.BaseType{},
							},
						},
					},
				},
			},
			0x2: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*[]uint", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee: &ir.GoSliceHeaderType{
					StructureType: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "[]uint", ByteSize: 0x18},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x395c0, GoKind: 0x17},
						Fields: []ir.Field{
							ir.Field{
								Name:   "array",
								Offset: 0x0,
								Type: &ir.PointerType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*uint", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
									Pointee: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
									},
								},
							},
							ir.Field{
								Name:   "len",
								Offset: 0x8,
								Type:   &ir.BaseType{},
							},
							ir.Field{
								Name:   "cap",
								Offset: 0x10,
								Type:   &ir.BaseType{},
							},
						},
					},
					Data: &ir.GoSliceDataType{
						TypeCommon: ir.TypeCommon{ID: 0x9, Name: "[]uint.array", ByteSize: 0x0},
						Element: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
						},
					},
				},
			},
			0x3: &ir.GoSliceHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "[]uint", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x395c0, GoKind: 0x17},
					Fields: []ir.Field{
						ir.Field{
							Name:   "array",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*uint", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
								Pointee: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
						ir.Field{
							Name:   "cap",
							Offset: 0x10,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x9, Name: "[]uint.array", ByteSize: 0x0},
					Element: &ir.BaseType{
						TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint", ByteSize: 0x8},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
					},
				},
			},
			0x4: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*uint", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint", ByteSize: 0x8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
				},
			},
			0x5: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
			},
			0x6: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x7: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x7, Name: "[][]uint.array", ByteSize: 0x0},
				Element: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "[]uint", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x395c0, GoKind: 0x17},
					Fields: []ir.Field{
						ir.Field{
							Name:   "array",
							Offset: 0x0,
							Type:   &ir.PointerType{},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type:   &ir.BaseType{},
						},
						ir.Field{
							Name:   "cap",
							Offset: 0x10,
							Type:   &ir.BaseType{},
						},
					},
				},
			},
			0x8: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*[][]uint.array", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoSliceDataType{},
			},
			0x9: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x9, Name: "[]uint.array", ByteSize: 0x0},
				Element:    &ir.BaseType{},
			},
			0xa: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "*[]uint.array", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoSliceDataType{},
			},
			0xb: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0xb, Name: "ProbeEvent", ByteSize: 0x19},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "u",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x18,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0xb,
	}
	main_test_slice_of_slices_bytes := []byte{0x70, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0x4f, 0x8a, 0x92, 0x6a, 0x97, 0xb8, 0x5, 0x14, 0x32, 0xd0, 0xb6, 0x41, 0x6b, 0x0, 0x0, 0x0, 0x98, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa0, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0xb, 0x0, 0x0, 0x0, 0x19, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_struct_slice
	main_test_struct_slice_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_struct_slice",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_struct_slice",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9810, 0x3d9820},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "xs",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]main.structWithNoStrings", ByteSize: 0x18},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x17},
								Fields: []ir.Field{
									ir.Field{
										Name:   "array",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.structWithNoStrings", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "aUint8",
														Offset: 0x0,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "aBool",
														Offset: 0x1,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
														},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
									ir.Field{
										Name:   "cap",
										Offset: 0x10,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9810, 0x3d9820},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "a",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9810, 0x3d9820},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 3},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x9, Name: "ProbeEvent", ByteSize: 0x21},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "xs",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x18,
											},
										},
									},
								},
								&ir.RootExpression{
									Name:   "a",
									Offset: 0x19,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d9810},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_struct_slice",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9810, 0x3d9820},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "xs",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]main.structWithNoStrings", ByteSize: 0x18},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x17},
							Fields: []ir.Field{
								ir.Field{
									Name:   "array",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.structWithNoStrings", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
											Fields: []ir.Field{
												ir.Field{
													Name:   "aUint8",
													Offset: 0x0,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
													},
												},
												ir.Field{
													Name:   "aBool",
													Offset: 0x1,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
													},
												},
											},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
								ir.Field{
									Name:   "cap",
									Offset: 0x10,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9810, 0x3d9820},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "a",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9810, 0x3d9820},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 3},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.GoSliceHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]main.structWithNoStrings", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x17},
					Fields: []ir.Field{
						ir.Field{
							Name:   "array",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.structWithNoStrings", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "aUint8",
											Offset: 0x0,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "aBool",
											Offset: 0x1,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
											},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type:   &ir.BaseType{},
						},
						ir.Field{
							Name:   "cap",
							Offset: 0x10,
							Type:   &ir.BaseType{},
						},
					},
				},
				Data: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x7, Name: "[]main.structWithNoStrings.array", ByteSize: 0x0},
					Element: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
						Fields: []ir.Field{
							ir.Field{
								Name:   "aUint8",
								Offset: 0x0,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
								},
							},
							ir.Field{
								Name:   "aBool",
								Offset: 0x1,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
								},
							},
						},
					},
				},
			},
			0x2: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.structWithNoStrings", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "aUint8",
							Offset: 0x0,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
							},
						},
						ir.Field{
							Name:   "aBool",
							Offset: 0x1,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
							},
						},
					},
				},
			},
			0x3: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "aUint8",
						Offset: 0x0,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
						},
					},
					ir.Field{
						Name:   "aBool",
						Offset: 0x1,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
						},
					},
				},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x5: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
			},
			0x6: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x7: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x7, Name: "[]main.structWithNoStrings.array", ByteSize: 0x0},
				Element:    &ir.StructureType{},
			},
			0x8: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*[]main.structWithNoStrings.array", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoSliceDataType{},
			},
			0x9: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x9, Name: "ProbeEvent", ByteSize: 0x21},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "xs",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x18,
								},
							},
						},
					},
					&ir.RootExpression{
						Name:   "a",
						Offset: 0x19,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x9,
	}
	main_test_struct_slice_bytes := []byte{0x78, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xb5, 0xc0, 0xd2, 0xc0, 0x76, 0x63, 0xed, 0x50, 0xfa, 0x5, 0x1f, 0xd6, 0x41, 0x6b, 0x0, 0x0, 0x10, 0x98, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa0, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x21, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x2, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_empty_slice_of_structs
	main_test_empty_slice_of_structs_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_empty_slice_of_structs",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_empty_slice_of_structs",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9820, 0x3d9830},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "xs",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]main.structWithNoStrings", ByteSize: 0x18},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x17},
								Fields: []ir.Field{
									ir.Field{
										Name:   "array",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.structWithNoStrings", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "aUint8",
														Offset: 0x0,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "aBool",
														Offset: 0x1,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
														},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
									ir.Field{
										Name:   "cap",
										Offset: 0x10,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9820, 0x3d9830},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "a",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9820, 0x3d9830},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 3},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x9, Name: "ProbeEvent", ByteSize: 0x21},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "xs",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x18,
											},
										},
									},
								},
								&ir.RootExpression{
									Name:   "a",
									Offset: 0x19,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d9820},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_empty_slice_of_structs",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9820, 0x3d9830},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "xs",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]main.structWithNoStrings", ByteSize: 0x18},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x17},
							Fields: []ir.Field{
								ir.Field{
									Name:   "array",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.structWithNoStrings", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
											Fields: []ir.Field{
												ir.Field{
													Name:   "aUint8",
													Offset: 0x0,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
													},
												},
												ir.Field{
													Name:   "aBool",
													Offset: 0x1,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
													},
												},
											},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
								ir.Field{
									Name:   "cap",
									Offset: 0x10,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9820, 0x3d9830},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "a",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9820, 0x3d9830},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 3},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.GoSliceHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]main.structWithNoStrings", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x17},
					Fields: []ir.Field{
						ir.Field{
							Name:   "array",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.structWithNoStrings", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "aUint8",
											Offset: 0x0,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "aBool",
											Offset: 0x1,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
											},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type:   &ir.BaseType{},
						},
						ir.Field{
							Name:   "cap",
							Offset: 0x10,
							Type:   &ir.BaseType{},
						},
					},
				},
				Data: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x7, Name: "[]main.structWithNoStrings.array", ByteSize: 0x0},
					Element: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
						Fields: []ir.Field{
							ir.Field{
								Name:   "aUint8",
								Offset: 0x0,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
								},
							},
							ir.Field{
								Name:   "aBool",
								Offset: 0x1,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
								},
							},
						},
					},
				},
			},
			0x2: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.structWithNoStrings", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "aUint8",
							Offset: 0x0,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
							},
						},
						ir.Field{
							Name:   "aBool",
							Offset: 0x1,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
							},
						},
					},
				},
			},
			0x3: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "aUint8",
						Offset: 0x0,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
						},
					},
					ir.Field{
						Name:   "aBool",
						Offset: 0x1,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
						},
					},
				},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x5: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
			},
			0x6: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x7: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x7, Name: "[]main.structWithNoStrings.array", ByteSize: 0x0},
				Element:    &ir.StructureType{},
			},
			0x8: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*[]main.structWithNoStrings.array", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoSliceDataType{},
			},
			0x9: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x9, Name: "ProbeEvent", ByteSize: 0x21},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "xs",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x18,
								},
							},
						},
					},
					&ir.RootExpression{
						Name:   "a",
						Offset: 0x19,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x9,
	}
	main_test_empty_slice_of_structs_bytes := []byte{0x78, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x36, 0x30, 0xbe, 0x27, 0x66, 0xe, 0xb6, 0xaa, 0xca, 0x75, 0x54, 0xf8, 0x41, 0x6b, 0x0, 0x0, 0x20, 0x98, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa0, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x21, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_nil_slice_of_structs
	main_test_nil_slice_of_structs_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_nil_slice_of_structs",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_nil_slice_of_structs",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9830, 0x3d9840},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "xs",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]main.structWithNoStrings", ByteSize: 0x18},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x17},
								Fields: []ir.Field{
									ir.Field{
										Name:   "array",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.structWithNoStrings", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "aUint8",
														Offset: 0x0,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "aBool",
														Offset: 0x1,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
														},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
									ir.Field{
										Name:   "cap",
										Offset: 0x10,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9830, 0x3d9840},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "a",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9830, 0x3d9840},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 3},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x9, Name: "ProbeEvent", ByteSize: 0x21},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "xs",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x18,
											},
										},
									},
								},
								&ir.RootExpression{
									Name:   "a",
									Offset: 0x19,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d9830},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_nil_slice_of_structs",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9830, 0x3d9840},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "xs",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]main.structWithNoStrings", ByteSize: 0x18},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x17},
							Fields: []ir.Field{
								ir.Field{
									Name:   "array",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.structWithNoStrings", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
											Fields: []ir.Field{
												ir.Field{
													Name:   "aUint8",
													Offset: 0x0,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
													},
												},
												ir.Field{
													Name:   "aBool",
													Offset: 0x1,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
													},
												},
											},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
								ir.Field{
									Name:   "cap",
									Offset: 0x10,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9830, 0x3d9840},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "a",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9830, 0x3d9840},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 3},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.GoSliceHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]main.structWithNoStrings", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x17},
					Fields: []ir.Field{
						ir.Field{
							Name:   "array",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.structWithNoStrings", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "aUint8",
											Offset: 0x0,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "aBool",
											Offset: 0x1,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
											},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type:   &ir.BaseType{},
						},
						ir.Field{
							Name:   "cap",
							Offset: 0x10,
							Type:   &ir.BaseType{},
						},
					},
				},
				Data: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x7, Name: "[]main.structWithNoStrings.array", ByteSize: 0x0},
					Element: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
						Fields: []ir.Field{
							ir.Field{
								Name:   "aUint8",
								Offset: 0x0,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
								},
							},
							ir.Field{
								Name:   "aBool",
								Offset: 0x1,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
								},
							},
						},
					},
				},
			},
			0x2: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.structWithNoStrings", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "aUint8",
							Offset: 0x0,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
							},
						},
						ir.Field{
							Name:   "aBool",
							Offset: 0x1,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
							},
						},
					},
				},
			},
			0x3: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "main.structWithNoStrings", ByteSize: 0x2},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "aUint8",
						Offset: 0x0,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
						},
					},
					ir.Field{
						Name:   "aBool",
						Offset: 0x1,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
						},
					},
				},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x5: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "bool", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
			},
			0x6: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x7: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x7, Name: "[]main.structWithNoStrings.array", ByteSize: 0x0},
				Element:    &ir.StructureType{},
			},
			0x8: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*[]main.structWithNoStrings.array", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoSliceDataType{},
			},
			0x9: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x9, Name: "ProbeEvent", ByteSize: 0x21},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "xs",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x18,
								},
							},
						},
					},
					&ir.RootExpression{
						Name:   "a",
						Offset: 0x19,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x9,
	}
	main_test_nil_slice_of_structs_bytes := []byte{0x78, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x37, 0xa, 0xed, 0xc3, 0x99, 0x2c, 0x4a, 0x5a, 0xc5, 0xc9, 0x43, 0x17, 0x42, 0x6b, 0x0, 0x0, 0x30, 0x98, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa0, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x21, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_string_slice
	main_test_string_slice_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_string_slice",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_string_slice",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9840, 0x3d9850},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "s",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]string", ByteSize: 0x18},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x39700, GoKind: 0x17},
								Fields: []ir.Field{
									ir.Field{
										Name:   "array",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*string", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
											Pointee: &ir.GoStringHeaderType{
												StructureType: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
													Fields: []ir.Field{
														ir.Field{
															Name:   "str",
															Offset: 0x0,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "*string.str", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{},
																Pointee: &ir.GoStringDataType{
																	TypeCommon: ir.TypeCommon{ID: 0x9, Name: "string.str", ByteSize: 0x0},
																},
															},
														},
														ir.Field{
															Name:   "len",
															Offset: 0x8,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
															},
														},
													},
												},
												Data: &ir.GoStringDataType{
													TypeCommon: ir.TypeCommon{ID: 0x9, Name: "string.str", ByteSize: 0x0},
												},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
									ir.Field{
										Name:   "cap",
										Offset: 0x10,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9840, 0x3d9850},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0xb, Name: "ProbeEvent", ByteSize: 0x19},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "s",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x18,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d9840},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_string_slice",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9840, 0x3d9850},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "s",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]string", ByteSize: 0x18},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x39700, GoKind: 0x17},
							Fields: []ir.Field{
								ir.Field{
									Name:   "array",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*string", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
										Pointee: &ir.GoStringHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
												Fields: []ir.Field{
													ir.Field{
														Name:   "str",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "*string.str", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{},
															Pointee:          &ir.GoStringDataType{},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type:   &ir.BaseType{},
													},
												},
											},
											Data: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x9, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
								ir.Field{
									Name:   "cap",
									Offset: 0x10,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9840, 0x3d9850},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.GoSliceHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]string", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x39700, GoKind: 0x17},
					Fields: []ir.Field{
						ir.Field{
							Name:   "array",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*string", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
								Pointee: &ir.GoStringHeaderType{
									StructureType: &ir.StructureType{
										TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
										Fields: []ir.Field{
											ir.Field{
												Name:   "str",
												Offset: 0x0,
												Type: &ir.PointerType{
													TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "*string.str", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{},
													Pointee:          &ir.GoStringDataType{},
												},
											},
											ir.Field{
												Name:   "len",
												Offset: 0x8,
												Type:   &ir.BaseType{},
											},
										},
									},
									Data: &ir.GoStringDataType{
										TypeCommon: ir.TypeCommon{ID: 0x9, Name: "string.str", ByteSize: 0x0},
									},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
						ir.Field{
							Name:   "cap",
							Offset: 0x10,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x7, Name: "[]string.array", ByteSize: 0x0},
					Element: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
						Fields: []ir.Field{
							ir.Field{
								Name:   "str",
								Offset: 0x0,
								Type: &ir.PointerType{
									TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "*string.str", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{},
									Pointee: &ir.GoStringDataType{
										TypeCommon: ir.TypeCommon{ID: 0x9, Name: "string.str", ByteSize: 0x0},
									},
								},
							},
							ir.Field{
								Name:   "len",
								Offset: 0x8,
								Type:   &ir.BaseType{},
							},
						},
					},
				},
			},
			0x2: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*string", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
				Pointee: &ir.GoStringHeaderType{
					StructureType: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
						Fields: []ir.Field{
							ir.Field{
								Name:   "str",
								Offset: 0x0,
								Type: &ir.PointerType{
									TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "*string.str", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{},
									Pointee: &ir.GoStringDataType{
										TypeCommon: ir.TypeCommon{ID: 0x9, Name: "string.str", ByteSize: 0x0},
									},
								},
							},
							ir.Field{
								Name:   "len",
								Offset: 0x8,
								Type:   &ir.BaseType{},
							},
						},
					},
					Data: &ir.GoStringDataType{
						TypeCommon: ir.TypeCommon{ID: 0x9, Name: "string.str", ByteSize: 0x0},
					},
				},
			},
			0x3: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee:          &ir.GoStringDataType{},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x9, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x4: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
				},
			},
			0x5: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x6: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x7: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x7, Name: "[]string.array", ByteSize: 0x0},
				Element: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee:          &ir.GoStringDataType{},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type:   &ir.BaseType{},
						},
					},
				},
			},
			0x8: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*[]string.array", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoSliceDataType{},
			},
			0x9: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x9, Name: "string.str", ByteSize: 0x0},
			},
			0xa: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0xb: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0xb, Name: "ProbeEvent", ByteSize: 0x19},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "s",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x18,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0xb,
	}
	main_test_string_slice_bytes := []byte{0x70, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x18, 0xbb, 0xac, 0x0, 0x1f, 0x7b, 0xb6, 0x69, 0x98, 0x68, 0x23, 0x36, 0x42, 0x6b, 0x0, 0x0, 0x40, 0x98, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa0, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0xb, 0x0, 0x0, 0x0, 0x19, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_nil_slice_with_other_params
	main_test_nil_slice_with_other_params_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_nil_slice_with_other_params",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_nil_slice_with_other_params",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9850, 0x3d9860},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "a",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45440, GoKind: 0x3},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9850, 0x3d9860},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 1, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "s",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "[]bool", ByteSize: 0x18},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x396c0, GoKind: 0x17},
								Fields: []ir.Field{
									ir.Field{
										Name:   "array",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*bool", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e480, GoKind: 0x16},
											Pointee: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "bool", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
									ir.Field{
										Name:   "cap",
										Offset: 0x10,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9850, 0x3d9860},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 3},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "uint", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9850, 0x3d9860},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 4},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x9, Name: "ProbeEvent", ByteSize: 0x22},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "a",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x1,
											},
										},
									},
								},
								&ir.RootExpression{
									Name:   "s",
									Offset: 0x2,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x18,
											},
										},
									},
								},
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1a,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d9850},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_nil_slice_with_other_params",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9850, 0x3d9860},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "a",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int8", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45440, GoKind: 0x3},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9850, 0x3d9860},
								Pieces: []locexpr.LocationPiece{
									{Size: 1, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "s",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "[]bool", ByteSize: 0x18},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x396c0, GoKind: 0x17},
							Fields: []ir.Field{
								ir.Field{
									Name:   "array",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*bool", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e480, GoKind: 0x16},
										Pointee: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "bool", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
								ir.Field{
									Name:   "cap",
									Offset: 0x10,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9850, 0x3d9860},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 3},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "uint", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9850, 0x3d9860},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 4},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "int8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45440, GoKind: 0x3},
			},
			0x2: &ir.GoSliceHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "[]bool", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x396c0, GoKind: 0x17},
					Fields: []ir.Field{
						ir.Field{
							Name:   "array",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*bool", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e480, GoKind: 0x16},
								Pointee: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "bool", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
						ir.Field{
							Name:   "cap",
							Offset: 0x10,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x7, Name: "[]bool.array", ByteSize: 0x0},
					Element: &ir.BaseType{
						TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "bool", ByteSize: 0x1},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
					},
				},
			},
			0x3: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*bool", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e480, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "bool", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
				},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "bool", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
			},
			0x5: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x6: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "uint", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
			},
			0x7: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x7, Name: "[]bool.array", ByteSize: 0x0},
				Element:    &ir.BaseType{},
			},
			0x8: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "*[]bool.array", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoSliceDataType{},
			},
			0x9: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x9, Name: "ProbeEvent", ByteSize: 0x22},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "a",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x1,
								},
							},
						},
					},
					&ir.RootExpression{
						Name:   "s",
						Offset: 0x2,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x18,
								},
							},
						},
					},
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1a,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x9,
	}
	main_test_nil_slice_with_other_params_bytes := []byte{0x78, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x19, 0xd4, 0xd8, 0xdc, 0x24, 0x30, 0x96, 0x14, 0x54, 0x7e, 0x7d, 0x55, 0x42, 0x6b, 0x0, 0x0, 0x50, 0x98, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa0, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x22, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_nil_slice
	main_test_nil_slice_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_nil_slice",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_nil_slice",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9860, 0x3d9870},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "xs",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]uint16", ByteSize: 0x18},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x39400, GoKind: 0x17},
								Fields: []ir.Field{
									ir.Field{
										Name:   "array",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*uint16", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e180, GoKind: 0x16},
											Pointee: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint16", ByteSize: 0x2},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
									ir.Field{
										Name:   "cap",
										Offset: 0x10,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9860, 0x3d9870},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x7, Name: "ProbeEvent", ByteSize: 0x19},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "xs",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x18,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d9860},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_nil_slice",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9860, 0x3d9870},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "xs",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]uint16", ByteSize: 0x18},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x39400, GoKind: 0x17},
							Fields: []ir.Field{
								ir.Field{
									Name:   "array",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*uint16", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e180, GoKind: 0x16},
										Pointee: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint16", ByteSize: 0x2},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
								ir.Field{
									Name:   "cap",
									Offset: 0x10,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9860, 0x3d9870},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.GoSliceHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "[]uint16", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x39400, GoKind: 0x17},
					Fields: []ir.Field{
						ir.Field{
							Name:   "array",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*uint16", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e180, GoKind: 0x16},
								Pointee: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint16", ByteSize: 0x2},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
						ir.Field{
							Name:   "cap",
							Offset: 0x10,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x5, Name: "[]uint16.array", ByteSize: 0x0},
					Element: &ir.BaseType{
						TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint16", ByteSize: 0x2},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
					},
				},
			},
			0x2: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*uint16", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e180, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint16", ByteSize: 0x2},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
				},
			},
			0x3: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint16", ByteSize: 0x2},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x5: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x5, Name: "[]uint16.array", ByteSize: 0x0},
				Element:    &ir.BaseType{},
			},
			0x6: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*[]uint16.array", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoSliceDataType{},
			},
			0x7: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x7, Name: "ProbeEvent", ByteSize: 0x19},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "xs",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x18,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x7,
	}
	main_test_nil_slice_bytes := []byte{0x70, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5a, 0x6c, 0xec, 0x74, 0xbd, 0xd6, 0xeb, 0x25, 0x2e, 0xa6, 0x30, 0x74, 0x42, 0x6b, 0x0, 0x0, 0x60, 0x98, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa0, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7, 0x0, 0x0, 0x0, 0x19, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_struct
	main_test_struct_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_struct",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_struct",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9e80, 0x3d9ea0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.aStruct", ByteSize: 0x38},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "aBool",
										Offset: 0x0,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "bool", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
										},
									},
									ir.Field{
										Name:   "aString",
										Offset: 0x8,
										Type: &ir.GoStringHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
												Fields: []ir.Field{
													ir.Field{
														Name:   "str",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "*string.str", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{},
															Pointee: &ir.GoStringDataType{
																TypeCommon: ir.TypeCommon{ID: 0x8, Name: "string.str", ByteSize: 0x0},
															},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
														},
													},
												},
											},
											Data: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x8, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "aNumber",
										Offset: 0x18,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
									ir.Field{
										Name:   "nested",
										Offset: 0x20,
										Type: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "main.nestedStruct", ByteSize: 0x18},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x9e460, GoKind: 0x19},
											Fields: []ir.Field{
												ir.Field{
													Name:   "anotherInt",
													Offset: 0x0,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "anotherString",
													Offset: 0x8,
													Type:   &ir.GoStringHeaderType{},
												},
											},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d9e80, 0x3d9ea0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 1, InReg: true, StackOffset: 0, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 3},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 4},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 5},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 6},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0xa, Name: "ProbeEvent", ByteSize: 0x39},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x38,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d9e80},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_struct",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9e80, 0x3d9ea0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.aStruct", ByteSize: 0x38},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
							Fields: []ir.Field{
								ir.Field{
									Name:   "aBool",
									Offset: 0x0,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "bool", ByteSize: 0x1},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
									},
								},
								ir.Field{
									Name:   "aString",
									Offset: 0x8,
									Type: &ir.GoStringHeaderType{
										StructureType: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
											Fields: []ir.Field{
												ir.Field{
													Name:   "str",
													Offset: 0x0,
													Type: &ir.PointerType{
														TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "*string.str", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{},
														Pointee:          &ir.GoStringDataType{},
													},
												},
												ir.Field{
													Name:   "len",
													Offset: 0x8,
													Type:   &ir.BaseType{},
												},
											},
										},
										Data: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x8, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
								ir.Field{
									Name:   "aNumber",
									Offset: 0x18,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
								ir.Field{
									Name:   "nested",
									Offset: 0x20,
									Type: &ir.StructureType{
										TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "main.nestedStruct", ByteSize: 0x18},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x9e460, GoKind: 0x19},
										Fields: []ir.Field{
											ir.Field{
												Name:   "anotherInt",
												Offset: 0x0,
												Type:   &ir.BaseType{},
											},
											ir.Field{
												Name:   "anotherString",
												Offset: 0x8,
												Type:   &ir.GoStringHeaderType{},
											},
										},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d9e80, 0x3d9ea0},
								Pieces: []locexpr.LocationPiece{
									{Size: 1, InReg: true, StackOffset: 0, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 3},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 4},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 5},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 6},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.aStruct", ByteSize: 0x38},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "aBool",
						Offset: 0x0,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "bool", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
						},
					},
					ir.Field{
						Name:   "aString",
						Offset: 0x8,
						Type: &ir.GoStringHeaderType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee:          &ir.GoStringDataType{},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type:   &ir.BaseType{},
									},
								},
							},
							Data: &ir.GoStringDataType{
								TypeCommon: ir.TypeCommon{ID: 0x8, Name: "string.str", ByteSize: 0x0},
							},
						},
					},
					ir.Field{
						Name:   "aNumber",
						Offset: 0x18,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
						},
					},
					ir.Field{
						Name:   "nested",
						Offset: 0x20,
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "main.nestedStruct", ByteSize: 0x18},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x9e460, GoKind: 0x19},
							Fields: []ir.Field{
								ir.Field{
									Name:   "anotherInt",
									Offset: 0x0,
									Type:   &ir.BaseType{},
								},
								ir.Field{
									Name:   "anotherString",
									Offset: 0x8,
									Type:   &ir.GoStringHeaderType{},
								},
							},
						},
					},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "bool", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
			},
			0x3: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee:          &ir.GoStringDataType{},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type:   &ir.BaseType{},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x8, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x4: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
				},
			},
			0x5: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x6: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x7: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "main.nestedStruct", ByteSize: 0x18},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x9e460, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "anotherInt",
						Offset: 0x0,
						Type:   &ir.BaseType{},
					},
					ir.Field{
						Name:   "anotherString",
						Offset: 0x8,
						Type:   &ir.GoStringHeaderType{},
					},
				},
			},
			0x8: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x8, Name: "string.str", ByteSize: 0x0},
			},
			0x9: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0xa: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0xa, Name: "ProbeEvent", ByteSize: 0x39},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x38,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0xa,
	}
	main_test_struct_bytes := []byte{0x90, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x90, 0x41, 0xfb, 0xc1, 0xb4, 0xde, 0x20, 0x12, 0xf0, 0x8, 0x1, 0x93, 0x42, 0x6b, 0x0, 0x0, 0x80, 0x9e, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa4, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa, 0x0, 0x0, 0x0, 0x39, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_empty_struct
	main_test_empty_struct_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_empty_struct",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_empty_struct",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9f80, 0x3d9f90},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "e",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.emptyStruct", ByteSize: 0x0},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
								Fields:           []ir.Field{},
							},
							Locations: []ir.Location{
								ir.Location{
									Range:  ir.PCRange{0x3d9f80, 0x3d9f90},
									Pieces: []locexpr.LocationPiece{},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x1},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "e",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x0,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d9f80},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_empty_struct",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9f80, 0x3d9f90},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "e",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.emptyStruct", ByteSize: 0x0},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
							Fields:           []ir.Field{},
						},
						Locations: []ir.Location{
							{
								Range:  ir.PCRange{0x3d9f80, 0x3d9f90},
								Pieces: []locexpr.LocationPiece{},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.emptyStruct", ByteSize: 0x0},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
				Fields:           []ir.Field{},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x1},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "e",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x0,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_empty_struct_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3d, 0x42, 0x33, 0x15, 0xc7, 0x77, 0xac, 0x10, 0x81, 0xfb, 0x3c, 0xb2, 0x42, 0x6b, 0x0, 0x0, 0x80, 0x9f, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa4, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_uint_pointer
	main_test_uint_pointer_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_uint_pointer",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_uint_pointer",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d8ed0, 0x3d8ee0},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "*uint", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
								Pointee: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8ed0, 0x3d8ee0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x9},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.PointerType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d8ed0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_uint_pointer",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d8ed0, 0x3d8ee0},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "*uint", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
							Pointee: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8ed0, 0x3d8ee0},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "*uint", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e300, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint", ByteSize: 0x8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x9},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.PointerType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_uint_pointer_bytes := []byte{0x78, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xfd, 0x47, 0x68, 0x1c, 0x69, 0xf5, 0xc7, 0x16, 0xf6, 0x78, 0x1c, 0xd1, 0x42, 0x6b, 0x0, 0x0, 0xd0, 0x8e, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xac, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x28, 0xbc, 0x15, 0x0, 0x40, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x8, 0x0, 0x0, 0x0, 0x28, 0xbc, 0x15, 0x0, 0x40, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_string_pointer
	main_test_string_pointer_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_string_pointer",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_string_pointer",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d8f40, 0x3d8f50},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "z",
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "*string", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
								Pointee: &ir.GoStringHeaderType{
									StructureType: &ir.StructureType{
										TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
										Fields: []ir.Field{
											ir.Field{
												Name:   "str",
												Offset: 0x0,
												Type: &ir.PointerType{
													TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{},
													Pointee: &ir.GoStringDataType{
														TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
													},
												},
											},
											ir.Field{
												Name:   "len",
												Offset: 0x8,
												Type: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
												},
											},
										},
									},
									Data: &ir.GoStringDataType{
										TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8f40, 0x3d8f50},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x8, Name: "ProbeEvent", ByteSize: 0x9},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "z",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.PointerType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d8f40},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_string_pointer",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d8f40, 0x3d8f50},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "z",
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "*string", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
							Pointee: &ir.GoStringHeaderType{
								StructureType: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
									Fields: []ir.Field{
										ir.Field{
											Name:   "str",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{},
												Pointee:          &ir.GoStringDataType{},
											},
										},
										ir.Field{
											Name:   "len",
											Offset: 0x8,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
											},
										},
									},
								},
								Data: &ir.GoStringDataType{
									TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8f40, 0x3d8f50},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "*string", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
				Pointee: &ir.GoStringHeaderType{
					StructureType: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
						Fields: []ir.Field{
							ir.Field{
								Name:   "str",
								Offset: 0x0,
								Type: &ir.PointerType{
									TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{},
									Pointee:          &ir.GoStringDataType{},
								},
							},
							ir.Field{
								Name:   "len",
								Offset: 0x8,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
								},
							},
						},
					},
					Data: &ir.GoStringDataType{
						TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
					},
				},
			},
			0x2: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee:          &ir.GoStringDataType{},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x3: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
				},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x5: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x6: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x6, Name: "string.str", ByteSize: 0x0},
			},
			0x7: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0x8: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x8, Name: "ProbeEvent", ByteSize: 0x9},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "z",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.PointerType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x8,
	}
	main_test_string_pointer_bytes := []byte{0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2c, 0x60, 0xca, 0x66, 0xc7, 0x1c, 0xe5, 0x23, 0xeb, 0x21, 0xa5, 0xf4, 0x42, 0x6b, 0x0, 0x0, 0x40, 0x8f, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xac, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x8, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0xd0, 0xbc, 0x15, 0x0, 0x40, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x10, 0x0, 0x0, 0x0, 0xd0, 0xbc, 0x15, 0x0, 0x40, 0x0, 0x0, 0x0, 0x9a, 0xd2, 0x53, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_nil_pointer
	main_test_nil_pointer_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_nil_pointer",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_nil_pointer",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d8f60, 0x3d8f70},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "z",
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "*bool", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e480, GoKind: 0x16},
								Pointee: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "bool", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8f60, 0x3d8f70},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "a",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8f60, 0x3d8f70},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x4, Name: "ProbeEvent", ByteSize: 0x11},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "z",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.PointerType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
								&ir.RootExpression{
									Name:   "a",
									Offset: 0x9,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d8f60},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_nil_pointer",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d8f60, 0x3d8f70},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "z",
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "*bool", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e480, GoKind: 0x16},
							Pointee: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "bool", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8f60, 0x3d8f70},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "a",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8f60, 0x3d8f70},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 1},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "*bool", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e480, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "bool", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
				},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "bool", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
			},
			0x3: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
			},
			0x4: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x4, Name: "ProbeEvent", ByteSize: 0x11},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "z",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.PointerType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
					&ir.RootExpression{
						Name:   "a",
						Offset: 0x9,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x4,
	}
	main_test_nil_pointer_bytes := []byte{0x68, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xce, 0xad, 0xbd, 0xf9, 0x2, 0x39, 0x8a, 0xba, 0xc6, 0xc, 0xf7, 0x15, 0x43, 0x6b, 0x0, 0x0, 0x60, 0x8f, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xac, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4, 0x0, 0x0, 0x0, 0x11, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_combined_byte
	main_test_combined_byte_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_combined_byte",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_combined_byte",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d8a70, 0x3d8a80},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "w",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8a70, 0x3d8a80},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 1, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "x",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8a70, 0x3d8a80},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 1, InReg: true, StackOffset: 0, Register: 1},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "y",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "float32", ByteSize: 0x4},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45780, GoKind: 0xd},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8a70, 0x3d8a80},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 4, InReg: true, StackOffset: 0, Register: 64},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x7},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "w",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x1,
											},
										},
									},
								},
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x2,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x1,
											},
										},
									},
								},
								&ir.RootExpression{
									Name:   "y",
									Offset: 0x3,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x4,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d8a70},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_combined_byte",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d8a70, 0x3d8a80},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "w",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint8", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8a70, 0x3d8a80},
								Pieces: []locexpr.LocationPiece{
									{Size: 1, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "x",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint8", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8a70, 0x3d8a80},
								Pieces: []locexpr.LocationPiece{
									{Size: 1, InReg: true, StackOffset: 0, Register: 1},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "y",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "float32", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45780, GoKind: 0xd},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8a70, 0x3d8a80},
								Pieces: []locexpr.LocationPiece{
									{Size: 4, InReg: true, StackOffset: 0, Register: 64},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "float32", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45780, GoKind: 0xd},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x7},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "w",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x1,
								},
							},
						},
					},
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x2,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x1,
								},
							},
						},
					},
					&ir.RootExpression{
						Name:   "y",
						Offset: 0x3,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x4,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_combined_byte_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2b, 0x3b, 0x1f, 0xa8, 0xe, 0xaf, 0x21, 0x4e, 0xb0, 0x96, 0xcc, 0x3a, 0x43, 0x6b, 0x0, 0x0, 0x70, 0x8a, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x94, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x7, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x2, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_multiple_simple_params
	main_test_multiple_simple_params_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_multiple_simple_params",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_multiple_simple_params",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d8b50, 0x3d8b60},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "a",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "bool", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8b50, 0x3d8b60},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 1, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "b",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8b50, 0x3d8b60},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 1, InReg: true, StackOffset: 0, Register: 1},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "c",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "int32", ByteSize: 0x4},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8b50, 0x3d8b60},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 4, InReg: true, StackOffset: 0, Register: 2},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "d",
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8b50, 0x3d8b60},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 3},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "e",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x8, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8b50, 0x3d8b60},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 4},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 5},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0xa, Name: "ProbeEvent", ByteSize: 0x1f},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "a",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x1,
											},
										},
									},
								},
								&ir.RootExpression{
									Name:   "b",
									Offset: 0x2,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x1,
											},
										},
									},
								},
								&ir.RootExpression{
									Name:   "c",
									Offset: 0x3,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x4,
											},
										},
									},
								},
								&ir.RootExpression{
									Name:   "d",
									Offset: 0x7,
									Expression: ir.Expression{
										Type: &ir.BaseType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
								&ir.RootExpression{
									Name:   "e",
									Offset: 0xf,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x10,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d8b50},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_multiple_simple_params",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d8b50, 0x3d8b60},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "a",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "bool", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8b50, 0x3d8b60},
								Pieces: []locexpr.LocationPiece{
									{Size: 1, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "b",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint8", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8b50, 0x3d8b60},
								Pieces: []locexpr.LocationPiece{
									{Size: 1, InReg: true, StackOffset: 0, Register: 1},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "c",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "int32", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8b50, 0x3d8b60},
								Pieces: []locexpr.LocationPiece{
									{Size: 4, InReg: true, StackOffset: 0, Register: 2},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "d",
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8b50, 0x3d8b60},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 3},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "e",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "string", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
							Fields: []ir.Field{
								ir.Field{
									Name:   "str",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "*string.str", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x8, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8b50, 0x3d8b60},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 4},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 5},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "bool", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45800, GoKind: 0x1},
			},
			0x2: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x3: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "int32", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45540, GoKind: 0x5},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45680, GoKind: 0x7},
			},
			0x5: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee: &ir.GoStringDataType{
									TypeCommon: ir.TypeCommon{ID: 0x8, Name: "string.str", ByteSize: 0x0},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x8, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x6: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee:          &ir.BaseType{},
			},
			0x7: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x8: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x8, Name: "string.str", ByteSize: 0x0},
			},
			0x9: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0xa: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0xa, Name: "ProbeEvent", ByteSize: 0x1f},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "a",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x1,
								},
							},
						},
					},
					&ir.RootExpression{
						Name:   "b",
						Offset: 0x2,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x1,
								},
							},
						},
					},
					&ir.RootExpression{
						Name:   "c",
						Offset: 0x3,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x4,
								},
							},
						},
					},
					&ir.RootExpression{
						Name:   "d",
						Offset: 0x7,
						Expression: ir.Expression{
							Type: &ir.BaseType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
					&ir.RootExpression{
						Name:   "e",
						Offset: 0xf,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x10,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0xa,
	}
	main_test_multiple_simple_params_bytes := []byte{0x70, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xbc, 0x5f, 0xf0, 0xe6, 0xf, 0xbf, 0x54, 0xd0, 0xd3, 0xd5, 0xe, 0x5c, 0x43, 0x6b, 0x0, 0x0, 0x50, 0x8b, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x94, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa, 0x0, 0x0, 0x0, 0x1f, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1f, 0x0, 0x2a, 0x7a, 0x0, 0x0, 0x0, 0x39, 0x5, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_map_string_to_int
	main_test_map_string_to_int_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_map_string_to_int",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_map_string_to_int",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d8610, 0x3d8620},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "m",
							Type: &ir.GoMapType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "map[string]int", ByteSize: 0x0},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x77a20, GoKind: 0x15},
								HeaderType: &ir.GoSwissMapHeaderType{
									StructureType: &ir.StructureType{
										TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "map<string,int>", ByteSize: 0x30},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
										Fields: []ir.Field{
											ir.Field{
												Name:   "used",
												Offset: 0x0,
												Type: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
												},
											},
											ir.Field{
												Name:   "seed",
												Offset: 0x8,
												Type: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uintptr", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
												},
											},
											ir.Field{
												Name:   "dirPtr",
												Offset: 0x10,
												Type: &ir.PointerType{
													TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "**table<string,int>", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{},
													Pointee: &ir.PointerType{
														TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*table<string,int>", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{},
														Pointee: &ir.StructureType{
															TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "table<string,int>", ByteSize: 0x20},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
															Fields: []ir.Field{
																ir.Field{
																	Name:   "used",
																	Offset: 0x0,
																	Type:   nil,
																},
																ir.Field{
																	Name:   "capacity",
																	Offset: 0x2,
																	Type:   nil,
																},
																ir.Field{
																	Name:   "growthLeft",
																	Offset: 0x4,
																	Type:   nil,
																},
																ir.Field{
																	Name:   "localDepth",
																	Offset: 0x6,
																	Type:   nil,
																},
																ir.Field{
																	Name:   "index",
																	Offset: 0x8,
																	Type:   nil,
																},
																ir.Field{
																	Name:   "groups",
																	Offset: 0x10,
																	Type:   nil,
																},
															},
														},
													},
												},
											},
											ir.Field{
												Name:   "dirLen",
												Offset: 0x18,
												Type: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "int", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
												},
											},
											ir.Field{
												Name:   "globalDepth",
												Offset: 0x20,
												Type: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
												},
											},
											ir.Field{
												Name:   "globalShift",
												Offset: 0x21,
												Type: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
												},
											},
											ir.Field{
												Name:   "writing",
												Offset: 0x22,
												Type: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
												},
											},
											ir.Field{
												Name:   "clearSeq",
												Offset: 0x28,
												Type: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
												},
											},
										},
									},
									TablePtrSliceType: &ir.GoSliceDataType{
										TypeCommon: ir.TypeCommon{ID: 0x13, Name: "[]*table<string,int>.array", ByteSize: 0x0},
										Element: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*table<string,int>", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "table<string,int>", ByteSize: 0x20},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "used",
														Offset: 0x0,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
														},
													},
													ir.Field{
														Name:   "capacity",
														Offset: 0x2,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
														},
													},
													ir.Field{
														Name:   "growthLeft",
														Offset: 0x4,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
														},
													},
													ir.Field{
														Name:   "localDepth",
														Offset: 0x6,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "index",
														Offset: 0x8,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "groups",
														Offset: 0x10,
														Type: &ir.GoSwissMapGroupsType{
															StructureType:  nil,
															GroupType:      nil,
															GroupSliceType: nil,
														},
													},
												},
											},
										},
									},
									GroupType: &ir.StructureType{
										TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "noalg.map.group[string]int", ByteSize: 0xc8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88bc0, GoKind: 0x19},
										Fields: []ir.Field{
											ir.Field{
												Name:   "ctrl",
												Offset: 0x0,
												Type: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
												},
											},
											ir.Field{
												Name:   "slots",
												Offset: 0x8,
												Type: &ir.ArrayType{
													TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "noalg.[8]struct { key string; elem int }", ByteSize: 0xc0},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d300, GoKind: 0x11},
													Count:            0x8,
													HasCount:         true,
													Element: &ir.StructureType{
														TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "noalg.struct { key string; elem int }", ByteSize: 0x18},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88b40, GoKind: 0x19},
														Fields: []ir.Field{
															ir.Field{
																Name:   "key",
																Offset: 0x0,
																Type: &ir.GoStringHeaderType{
																	StructureType: nil,
																	Data:          nil,
																},
															},
															ir.Field{
																Name:   "elem",
																Offset: 0x10,
																Type:   &ir.BaseType{},
															},
														},
													},
												},
											},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8610, 0x3d8620},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 0, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x17, Name: "ProbeEvent", ByteSize: 0x1},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "m",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.GoMapType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x0,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d8610},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_map_string_to_int",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d8610, 0x3d8620},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "m",
						Type: &ir.GoMapType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "map[string]int", ByteSize: 0x0},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x77a20, GoKind: 0x15},
							HeaderType: &ir.GoSwissMapHeaderType{
								StructureType: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "map<string,int>", ByteSize: 0x30},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "used",
											Offset: 0x0,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
											},
										},
										ir.Field{
											Name:   "seed",
											Offset: 0x8,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uintptr", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
											},
										},
										ir.Field{
											Name:   "dirPtr",
											Offset: 0x10,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "**table<string,int>", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{},
												Pointee: &ir.PointerType{
													TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*table<string,int>", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{},
													Pointee: &ir.StructureType{
														TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "table<string,int>", ByteSize: 0x20},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
														Fields: []ir.Field{
															ir.Field{
																Name:   "used",
																Offset: 0x0,
																Type: &ir.BaseType{
																	TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
																},
															},
															ir.Field{
																Name:   "capacity",
																Offset: 0x2,
																Type: &ir.BaseType{
																	TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
																},
															},
															ir.Field{
																Name:   "growthLeft",
																Offset: 0x4,
																Type: &ir.BaseType{
																	TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
																},
															},
															ir.Field{
																Name:   "localDepth",
																Offset: 0x6,
																Type:   &ir.BaseType{},
															},
															ir.Field{
																Name:   "index",
																Offset: 0x8,
																Type:   &ir.BaseType{},
															},
															ir.Field{
																Name:   "groups",
																Offset: 0x10,
																Type: &ir.GoSwissMapGroupsType{
																	StructureType:  nil,
																	GroupType:      nil,
																	GroupSliceType: nil,
																},
															},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "dirLen",
											Offset: 0x18,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "int", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
											},
										},
										ir.Field{
											Name:   "globalDepth",
											Offset: 0x20,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "globalShift",
											Offset: 0x21,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "writing",
											Offset: 0x22,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "clearSeq",
											Offset: 0x28,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
											},
										},
									},
								},
								TablePtrSliceType: &ir.GoSliceDataType{
									TypeCommon: ir.TypeCommon{ID: 0x13, Name: "[]*table<string,int>.array", ByteSize: 0x0},
									Element: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*table<string,int>", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "table<string,int>", ByteSize: 0x20},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
											Fields: []ir.Field{
												ir.Field{
													Name:   "used",
													Offset: 0x0,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
													},
												},
												ir.Field{
													Name:   "capacity",
													Offset: 0x2,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
													},
												},
												ir.Field{
													Name:   "growthLeft",
													Offset: 0x4,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
													},
												},
												ir.Field{
													Name:   "localDepth",
													Offset: 0x6,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "index",
													Offset: 0x8,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "groups",
													Offset: 0x10,
													Type: &ir.GoSwissMapGroupsType{
														StructureType: &ir.StructureType{
															TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "groupReference<string,int>", ByteSize: 0x10},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
															Fields: []ir.Field{
																ir.Field{
																	Name:   "data",
																	Offset: 0x0,
																	Type:   nil,
																},
																ir.Field{
																	Name:   "lengthMask",
																	Offset: 0x8,
																	Type:   nil,
																},
															},
														},
														GroupType: &ir.StructureType{},
														GroupSliceType: &ir.GoSliceDataType{
															TypeCommon: ir.TypeCommon{ID: 0x14, Name: "[]noalg.map.group[string]int.array", ByteSize: 0x0},
															Element:    nil,
														},
													},
												},
											},
										},
									},
								},
								GroupType: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "noalg.map.group[string]int", ByteSize: 0xc8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88bc0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "ctrl",
											Offset: 0x0,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
											},
										},
										ir.Field{
											Name:   "slots",
											Offset: 0x8,
											Type: &ir.ArrayType{
												TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "noalg.[8]struct { key string; elem int }", ByteSize: 0xc0},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d300, GoKind: 0x11},
												Count:            0x8,
												HasCount:         true,
												Element: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "noalg.struct { key string; elem int }", ByteSize: 0x18},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88b40, GoKind: 0x19},
													Fields: []ir.Field{
														ir.Field{
															Name:   "key",
															Offset: 0x0,
															Type: &ir.GoStringHeaderType{
																StructureType: &ir.StructureType{
																	TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "string", ByteSize: 0x10},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
																	Fields: []ir.Field{
																		ir.Field{
																			Name:   "str",
																			Offset: 0x0,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "len",
																			Offset: 0x8,
																			Type:   nil,
																		},
																	},
																},
																Data: &ir.GoStringDataType{
																	TypeCommon: ir.TypeCommon{ID: 0x15, Name: "string.str", ByteSize: 0x0},
																},
															},
														},
														ir.Field{
															Name:   "elem",
															Offset: 0x10,
															Type:   &ir.BaseType{},
														},
													},
												},
											},
										},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8610, 0x3d8620},
								Pieces: []locexpr.LocationPiece{
									{Size: 0, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.GoMapType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "map[string]int", ByteSize: 0x0},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x77a20, GoKind: 0x15},
				HeaderType: &ir.GoSwissMapHeaderType{
					StructureType: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "map<string,int>", ByteSize: 0x30},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
						Fields: []ir.Field{
							ir.Field{
								Name:   "used",
								Offset: 0x0,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
								},
							},
							ir.Field{
								Name:   "seed",
								Offset: 0x8,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uintptr", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
								},
							},
							ir.Field{
								Name:   "dirPtr",
								Offset: 0x10,
								Type: &ir.PointerType{
									TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "**table<string,int>", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{},
									Pointee: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*table<string,int>", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "table<string,int>", ByteSize: 0x20},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
											Fields: []ir.Field{
												ir.Field{
													Name:   "used",
													Offset: 0x0,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
													},
												},
												ir.Field{
													Name:   "capacity",
													Offset: 0x2,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
													},
												},
												ir.Field{
													Name:   "growthLeft",
													Offset: 0x4,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
													},
												},
												ir.Field{
													Name:   "localDepth",
													Offset: 0x6,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "index",
													Offset: 0x8,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "groups",
													Offset: 0x10,
													Type: &ir.GoSwissMapGroupsType{
														StructureType: &ir.StructureType{
															TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "groupReference<string,int>", ByteSize: 0x10},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
															Fields: []ir.Field{
																ir.Field{
																	Name:   "data",
																	Offset: 0x0,
																	Type:   nil,
																},
																ir.Field{
																	Name:   "lengthMask",
																	Offset: 0x8,
																	Type:   nil,
																},
															},
														},
														GroupType: &ir.StructureType{},
														GroupSliceType: &ir.GoSliceDataType{
															TypeCommon: ir.TypeCommon{ID: 0x14, Name: "[]noalg.map.group[string]int.array", ByteSize: 0x0},
															Element:    nil,
														},
													},
												},
											},
										},
									},
								},
							},
							ir.Field{
								Name:   "dirLen",
								Offset: 0x18,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "int", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
								},
							},
							ir.Field{
								Name:   "globalDepth",
								Offset: 0x20,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
								},
							},
							ir.Field{
								Name:   "globalShift",
								Offset: 0x21,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
								},
							},
							ir.Field{
								Name:   "writing",
								Offset: 0x22,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
								},
							},
							ir.Field{
								Name:   "clearSeq",
								Offset: 0x28,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
								},
							},
						},
					},
					TablePtrSliceType: &ir.GoSliceDataType{
						TypeCommon: ir.TypeCommon{ID: 0x13, Name: "[]*table<string,int>.array", ByteSize: 0x0},
						Element: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*table<string,int>", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{},
							Pointee: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "table<string,int>", ByteSize: 0x20},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "used",
										Offset: 0x0,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
										},
									},
									ir.Field{
										Name:   "capacity",
										Offset: 0x2,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
										},
									},
									ir.Field{
										Name:   "growthLeft",
										Offset: 0x4,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
										},
									},
									ir.Field{
										Name:   "localDepth",
										Offset: 0x6,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "index",
										Offset: 0x8,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "groups",
										Offset: 0x10,
										Type: &ir.GoSwissMapGroupsType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "groupReference<string,int>", ByteSize: 0x10},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "data",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*noalg.map.group[string]int", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{},
															Pointee:          nil,
														},
													},
													ir.Field{
														Name:   "lengthMask",
														Offset: 0x8,
														Type:   &ir.BaseType{},
													},
												},
											},
											GroupType: &ir.StructureType{},
											GroupSliceType: &ir.GoSliceDataType{
												TypeCommon: ir.TypeCommon{ID: 0x14, Name: "[]noalg.map.group[string]int.array", ByteSize: 0x0},
												Element:    &ir.StructureType{},
											},
										},
									},
								},
							},
						},
					},
					GroupType: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "noalg.map.group[string]int", ByteSize: 0xc8},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88bc0, GoKind: 0x19},
						Fields: []ir.Field{
							ir.Field{
								Name:   "ctrl",
								Offset: 0x0,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
								},
							},
							ir.Field{
								Name:   "slots",
								Offset: 0x8,
								Type: &ir.ArrayType{
									TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "noalg.[8]struct { key string; elem int }", ByteSize: 0xc0},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d300, GoKind: 0x11},
									Count:            0x8,
									HasCount:         true,
									Element: &ir.StructureType{
										TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "noalg.struct { key string; elem int }", ByteSize: 0x18},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88b40, GoKind: 0x19},
										Fields: []ir.Field{
											ir.Field{
												Name:   "key",
												Offset: 0x0,
												Type: &ir.GoStringHeaderType{
													StructureType: &ir.StructureType{
														TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "string", ByteSize: 0x10},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
														Fields: []ir.Field{
															ir.Field{
																Name:   "str",
																Offset: 0x0,
																Type: &ir.PointerType{
																	TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*string.str", ByteSize: 0x8},
																	GoTypeAttributes: ir.GoTypeAttributes{},
																	Pointee:          nil,
																},
															},
															ir.Field{
																Name:   "len",
																Offset: 0x8,
																Type:   &ir.BaseType{},
															},
														},
													},
													Data: &ir.GoStringDataType{
														TypeCommon: ir.TypeCommon{ID: 0x15, Name: "string.str", ByteSize: 0x0},
													},
												},
											},
											ir.Field{
												Name:   "elem",
												Offset: 0x10,
												Type:   &ir.BaseType{},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			0x2: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*map<string,int>", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee: &ir.GoSwissMapHeaderType{
					StructureType: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "map<string,int>", ByteSize: 0x30},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
						Fields: []ir.Field{
							ir.Field{
								Name:   "used",
								Offset: 0x0,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
								},
							},
							ir.Field{
								Name:   "seed",
								Offset: 0x8,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uintptr", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
								},
							},
							ir.Field{
								Name:   "dirPtr",
								Offset: 0x10,
								Type: &ir.PointerType{
									TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "**table<string,int>", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{},
									Pointee:          &ir.PointerType{},
								},
							},
							ir.Field{
								Name:   "dirLen",
								Offset: 0x18,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "int", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
								},
							},
							ir.Field{
								Name:   "globalDepth",
								Offset: 0x20,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
								},
							},
							ir.Field{
								Name:   "globalShift",
								Offset: 0x21,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
								},
							},
							ir.Field{
								Name:   "writing",
								Offset: 0x22,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
								},
							},
							ir.Field{
								Name:   "clearSeq",
								Offset: 0x28,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
								},
							},
						},
					},
					TablePtrSliceType: &ir.GoSliceDataType{
						TypeCommon: ir.TypeCommon{ID: 0x13, Name: "[]*table<string,int>.array", ByteSize: 0x0},
						Element: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*table<string,int>", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{},
							Pointee: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "table<string,int>", ByteSize: 0x20},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "used",
										Offset: 0x0,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
										},
									},
									ir.Field{
										Name:   "capacity",
										Offset: 0x2,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
										},
									},
									ir.Field{
										Name:   "growthLeft",
										Offset: 0x4,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
										},
									},
									ir.Field{
										Name:   "localDepth",
										Offset: 0x6,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "index",
										Offset: 0x8,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "groups",
										Offset: 0x10,
										Type: &ir.GoSwissMapGroupsType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "groupReference<string,int>", ByteSize: 0x10},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "data",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*noalg.map.group[string]int", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{},
															Pointee:          nil,
														},
													},
													ir.Field{
														Name:   "lengthMask",
														Offset: 0x8,
														Type:   &ir.BaseType{},
													},
												},
											},
											GroupType: &ir.StructureType{},
											GroupSliceType: &ir.GoSliceDataType{
												TypeCommon: ir.TypeCommon{ID: 0x14, Name: "[]noalg.map.group[string]int.array", ByteSize: 0x0},
												Element:    &ir.StructureType{},
											},
										},
									},
								},
							},
						},
					},
					GroupType: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "noalg.map.group[string]int", ByteSize: 0xc8},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88bc0, GoKind: 0x19},
						Fields: []ir.Field{
							ir.Field{
								Name:   "ctrl",
								Offset: 0x0,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
								},
							},
							ir.Field{
								Name:   "slots",
								Offset: 0x8,
								Type: &ir.ArrayType{
									TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "noalg.[8]struct { key string; elem int }", ByteSize: 0xc0},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d300, GoKind: 0x11},
									Count:            0x8,
									HasCount:         true,
									Element: &ir.StructureType{
										TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "noalg.struct { key string; elem int }", ByteSize: 0x18},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88b40, GoKind: 0x19},
										Fields: []ir.Field{
											ir.Field{
												Name:   "key",
												Offset: 0x0,
												Type: &ir.GoStringHeaderType{
													StructureType: &ir.StructureType{
														TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "string", ByteSize: 0x10},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
														Fields: []ir.Field{
															ir.Field{
																Name:   "str",
																Offset: 0x0,
																Type: &ir.PointerType{
																	TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*string.str", ByteSize: 0x8},
																	GoTypeAttributes: ir.GoTypeAttributes{},
																	Pointee:          nil,
																},
															},
															ir.Field{
																Name:   "len",
																Offset: 0x8,
																Type:   &ir.BaseType{},
															},
														},
													},
													Data: &ir.GoStringDataType{
														TypeCommon: ir.TypeCommon{ID: 0x15, Name: "string.str", ByteSize: 0x0},
													},
												},
											},
											ir.Field{
												Name:   "elem",
												Offset: 0x10,
												Type:   &ir.BaseType{},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			0x3: &ir.GoSwissMapHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "map<string,int>", ByteSize: 0x30},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "used",
							Offset: 0x0,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
							},
						},
						ir.Field{
							Name:   "seed",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uintptr", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
							},
						},
						ir.Field{
							Name:   "dirPtr",
							Offset: 0x10,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "**table<string,int>", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee: &ir.PointerType{
									TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*table<string,int>", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{},
									Pointee: &ir.StructureType{
										TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "table<string,int>", ByteSize: 0x20},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
										Fields: []ir.Field{
											ir.Field{
												Name:   "used",
												Offset: 0x0,
												Type: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
												},
											},
											ir.Field{
												Name:   "capacity",
												Offset: 0x2,
												Type: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
												},
											},
											ir.Field{
												Name:   "growthLeft",
												Offset: 0x4,
												Type: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
												},
											},
											ir.Field{
												Name:   "localDepth",
												Offset: 0x6,
												Type:   &ir.BaseType{},
											},
											ir.Field{
												Name:   "index",
												Offset: 0x8,
												Type:   &ir.BaseType{},
											},
											ir.Field{
												Name:   "groups",
												Offset: 0x10,
												Type: &ir.GoSwissMapGroupsType{
													StructureType: &ir.StructureType{
														TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "groupReference<string,int>", ByteSize: 0x10},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
														Fields: []ir.Field{
															ir.Field{
																Name:   "data",
																Offset: 0x0,
																Type: &ir.PointerType{
																	TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*noalg.map.group[string]int", ByteSize: 0x8},
																	GoTypeAttributes: ir.GoTypeAttributes{},
																	Pointee:          nil,
																},
															},
															ir.Field{
																Name:   "lengthMask",
																Offset: 0x8,
																Type:   &ir.BaseType{},
															},
														},
													},
													GroupType: &ir.StructureType{},
													GroupSliceType: &ir.GoSliceDataType{
														TypeCommon: ir.TypeCommon{ID: 0x14, Name: "[]noalg.map.group[string]int.array", ByteSize: 0x0},
														Element:    &ir.StructureType{},
													},
												},
											},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "dirLen",
							Offset: 0x18,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
						ir.Field{
							Name:   "globalDepth",
							Offset: 0x20,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
							},
						},
						ir.Field{
							Name:   "globalShift",
							Offset: 0x21,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
							},
						},
						ir.Field{
							Name:   "writing",
							Offset: 0x22,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
							},
						},
						ir.Field{
							Name:   "clearSeq",
							Offset: 0x28,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
							},
						},
					},
				},
				TablePtrSliceType: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x13, Name: "[]*table<string,int>.array", ByteSize: 0x0},
					Element: &ir.PointerType{
						TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*table<string,int>", ByteSize: 0x8},
						GoTypeAttributes: ir.GoTypeAttributes{},
						Pointee: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "table<string,int>", ByteSize: 0x20},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
							Fields: []ir.Field{
								ir.Field{
									Name:   "used",
									Offset: 0x0,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
									},
								},
								ir.Field{
									Name:   "capacity",
									Offset: 0x2,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
									},
								},
								ir.Field{
									Name:   "growthLeft",
									Offset: 0x4,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
									},
								},
								ir.Field{
									Name:   "localDepth",
									Offset: 0x6,
									Type:   &ir.BaseType{},
								},
								ir.Field{
									Name:   "index",
									Offset: 0x8,
									Type:   &ir.BaseType{},
								},
								ir.Field{
									Name:   "groups",
									Offset: 0x10,
									Type: &ir.GoSwissMapGroupsType{
										StructureType: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "groupReference<string,int>", ByteSize: 0x10},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
											Fields: []ir.Field{
												ir.Field{
													Name:   "data",
													Offset: 0x0,
													Type: &ir.PointerType{
														TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*noalg.map.group[string]int", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{},
														Pointee:          &ir.StructureType{},
													},
												},
												ir.Field{
													Name:   "lengthMask",
													Offset: 0x8,
													Type:   &ir.BaseType{},
												},
											},
										},
										GroupType: &ir.StructureType{},
										GroupSliceType: &ir.GoSliceDataType{
											TypeCommon: ir.TypeCommon{ID: 0x14, Name: "[]noalg.map.group[string]int.array", ByteSize: 0x0},
											Element:    &ir.StructureType{},
										},
									},
								},
							},
						},
					},
				},
				GroupType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "noalg.map.group[string]int", ByteSize: 0xc8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88bc0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "ctrl",
							Offset: 0x0,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
							},
						},
						ir.Field{
							Name:   "slots",
							Offset: 0x8,
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "noalg.[8]struct { key string; elem int }", ByteSize: 0xc0},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d300, GoKind: 0x11},
								Count:            0x8,
								HasCount:         true,
								Element: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "noalg.struct { key string; elem int }", ByteSize: 0x18},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88b40, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "key",
											Offset: 0x0,
											Type: &ir.GoStringHeaderType{
												StructureType: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "string", ByteSize: 0x10},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
													Fields: []ir.Field{
														ir.Field{
															Name:   "str",
															Offset: 0x0,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*string.str", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{},
																Pointee:          &ir.GoStringDataType{},
															},
														},
														ir.Field{
															Name:   "len",
															Offset: 0x8,
															Type:   &ir.BaseType{},
														},
													},
												},
												Data: &ir.GoStringDataType{
													TypeCommon: ir.TypeCommon{ID: 0x15, Name: "string.str", ByteSize: 0x0},
												},
											},
										},
										ir.Field{
											Name:   "elem",
											Offset: 0x10,
											Type:   &ir.BaseType{},
										},
									},
								},
							},
						},
					},
				},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "uint64", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45600, GoKind: 0xb},
			},
			0x5: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "uintptr", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
			},
			0x6: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "**table<string,int>", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee: &ir.PointerType{
					TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*table<string,int>", ByteSize: 0x8},
					GoTypeAttributes: ir.GoTypeAttributes{},
					Pointee: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "table<string,int>", ByteSize: 0x20},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
						Fields: []ir.Field{
							ir.Field{
								Name:   "used",
								Offset: 0x0,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
								},
							},
							ir.Field{
								Name:   "capacity",
								Offset: 0x2,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
								},
							},
							ir.Field{
								Name:   "growthLeft",
								Offset: 0x4,
								Type: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
								},
							},
							ir.Field{
								Name:   "localDepth",
								Offset: 0x6,
								Type:   &ir.BaseType{},
							},
							ir.Field{
								Name:   "index",
								Offset: 0x8,
								Type:   &ir.BaseType{},
							},
							ir.Field{
								Name:   "groups",
								Offset: 0x10,
								Type: &ir.GoSwissMapGroupsType{
									StructureType: &ir.StructureType{
										TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "groupReference<string,int>", ByteSize: 0x10},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
										Fields: []ir.Field{
											ir.Field{
												Name:   "data",
												Offset: 0x0,
												Type: &ir.PointerType{
													TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*noalg.map.group[string]int", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{},
													Pointee:          &ir.StructureType{},
												},
											},
											ir.Field{
												Name:   "lengthMask",
												Offset: 0x8,
												Type:   &ir.BaseType{},
											},
										},
									},
									GroupType: &ir.StructureType{},
									GroupSliceType: &ir.GoSliceDataType{
										TypeCommon: ir.TypeCommon{ID: 0x14, Name: "[]noalg.map.group[string]int.array", ByteSize: 0x0},
										Element:    &ir.StructureType{},
									},
								},
							},
						},
					},
				},
			},
			0x7: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "*table<string,int>", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "table<string,int>", ByteSize: 0x20},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "used",
							Offset: 0x0,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
							},
						},
						ir.Field{
							Name:   "capacity",
							Offset: 0x2,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
							},
						},
						ir.Field{
							Name:   "growthLeft",
							Offset: 0x4,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
							},
						},
						ir.Field{
							Name:   "localDepth",
							Offset: 0x6,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
							},
						},
						ir.Field{
							Name:   "index",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
						ir.Field{
							Name:   "groups",
							Offset: 0x10,
							Type: &ir.GoSwissMapGroupsType{
								StructureType: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "groupReference<string,int>", ByteSize: 0x10},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "data",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*noalg.map.group[string]int", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{},
												Pointee:          &ir.StructureType{},
											},
										},
										ir.Field{
											Name:   "lengthMask",
											Offset: 0x8,
											Type:   &ir.BaseType{},
										},
									},
								},
								GroupType: &ir.StructureType{},
								GroupSliceType: &ir.GoSliceDataType{
									TypeCommon: ir.TypeCommon{ID: 0x14, Name: "[]noalg.map.group[string]int.array", ByteSize: 0x0},
									Element:    &ir.StructureType{},
								},
							},
						},
					},
				},
			},
			0x8: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "table<string,int>", ByteSize: 0x20},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "used",
						Offset: 0x0,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
						},
					},
					ir.Field{
						Name:   "capacity",
						Offset: 0x2,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
						},
					},
					ir.Field{
						Name:   "growthLeft",
						Offset: 0x4,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
						},
					},
					ir.Field{
						Name:   "localDepth",
						Offset: 0x6,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
						},
					},
					ir.Field{
						Name:   "index",
						Offset: 0x8,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "int", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
						},
					},
					ir.Field{
						Name:   "groups",
						Offset: 0x10,
						Type: &ir.GoSwissMapGroupsType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "groupReference<string,int>", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "data",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*noalg.map.group[string]int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee:          &ir.StructureType{},
										},
									},
									ir.Field{
										Name:   "lengthMask",
										Offset: 0x8,
										Type:   &ir.BaseType{},
									},
								},
							},
							GroupType: &ir.StructureType{},
							GroupSliceType: &ir.GoSliceDataType{
								TypeCommon: ir.TypeCommon{ID: 0x14, Name: "[]noalg.map.group[string]int.array", ByteSize: 0x0},
								Element:    &ir.StructureType{},
							},
						},
					},
				},
			},
			0x9: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint16", ByteSize: 0x2},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45500, GoKind: 0x9},
			},
			0xa: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0xb: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0xc: &ir.GoSwissMapGroupsType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "groupReference<string,int>", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "data",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*noalg.map.group[string]int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee:          &ir.StructureType{},
							},
						},
						ir.Field{
							Name:   "lengthMask",
							Offset: 0x8,
							Type:   &ir.BaseType{},
						},
					},
				},
				GroupType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "noalg.map.group[string]int", ByteSize: 0xc8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88bc0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "ctrl",
							Offset: 0x0,
							Type:   &ir.BaseType{},
						},
						ir.Field{
							Name:   "slots",
							Offset: 0x8,
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "noalg.[8]struct { key string; elem int }", ByteSize: 0xc0},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d300, GoKind: 0x11},
								Count:            0x8,
								HasCount:         true,
								Element: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "noalg.struct { key string; elem int }", ByteSize: 0x18},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88b40, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "key",
											Offset: 0x0,
											Type: &ir.GoStringHeaderType{
												StructureType: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "string", ByteSize: 0x10},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
													Fields: []ir.Field{
														ir.Field{
															Name:   "str",
															Offset: 0x0,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*string.str", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{},
																Pointee:          &ir.GoStringDataType{},
															},
														},
														ir.Field{
															Name:   "len",
															Offset: 0x8,
															Type:   &ir.BaseType{},
														},
													},
												},
												Data: &ir.GoStringDataType{
													TypeCommon: ir.TypeCommon{ID: 0x15, Name: "string.str", ByteSize: 0x0},
												},
											},
										},
										ir.Field{
											Name:   "elem",
											Offset: 0x10,
											Type:   &ir.BaseType{},
										},
									},
								},
							},
						},
					},
				},
				GroupSliceType: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x14, Name: "[]noalg.map.group[string]int.array", ByteSize: 0x0},
					Element:    &ir.StructureType{},
				},
			},
			0xd: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*noalg.map.group[string]int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "noalg.map.group[string]int", ByteSize: 0xc8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88bc0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "ctrl",
							Offset: 0x0,
							Type:   &ir.BaseType{},
						},
						ir.Field{
							Name:   "slots",
							Offset: 0x8,
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "noalg.[8]struct { key string; elem int }", ByteSize: 0xc0},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d300, GoKind: 0x11},
								Count:            0x8,
								HasCount:         true,
								Element: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "noalg.struct { key string; elem int }", ByteSize: 0x18},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88b40, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "key",
											Offset: 0x0,
											Type: &ir.GoStringHeaderType{
												StructureType: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "string", ByteSize: 0x10},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
													Fields: []ir.Field{
														ir.Field{
															Name:   "str",
															Offset: 0x0,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*string.str", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{},
																Pointee:          &ir.GoStringDataType{},
															},
														},
														ir.Field{
															Name:   "len",
															Offset: 0x8,
															Type:   &ir.BaseType{},
														},
													},
												},
												Data: &ir.GoStringDataType{
													TypeCommon: ir.TypeCommon{ID: 0x15, Name: "string.str", ByteSize: 0x0},
												},
											},
										},
										ir.Field{
											Name:   "elem",
											Offset: 0x10,
											Type:   &ir.BaseType{},
										},
									},
								},
							},
						},
					},
				},
			},
			0xe: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "noalg.map.group[string]int", ByteSize: 0xc8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88bc0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "ctrl",
						Offset: 0x0,
						Type:   &ir.BaseType{},
					},
					ir.Field{
						Name:   "slots",
						Offset: 0x8,
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "noalg.[8]struct { key string; elem int }", ByteSize: 0xc0},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d300, GoKind: 0x11},
							Count:            0x8,
							HasCount:         true,
							Element: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "noalg.struct { key string; elem int }", ByteSize: 0x18},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88b40, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "key",
										Offset: 0x0,
										Type: &ir.GoStringHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "string", ByteSize: 0x10},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
												Fields: []ir.Field{
													ir.Field{
														Name:   "str",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*string.str", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{},
															Pointee:          &ir.GoStringDataType{},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type:   &ir.BaseType{},
													},
												},
											},
											Data: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x15, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "elem",
										Offset: 0x10,
										Type:   &ir.BaseType{},
									},
								},
							},
						},
					},
				},
			},
			0xf: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "noalg.[8]struct { key string; elem int }", ByteSize: 0xc0},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d300, GoKind: 0x11},
				Count:            0x8,
				HasCount:         true,
				Element: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "noalg.struct { key string; elem int }", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88b40, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "key",
							Offset: 0x0,
							Type: &ir.GoStringHeaderType{
								StructureType: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "string", ByteSize: 0x10},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
									Fields: []ir.Field{
										ir.Field{
											Name:   "str",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*string.str", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{},
												Pointee:          &ir.GoStringDataType{},
											},
										},
										ir.Field{
											Name:   "len",
											Offset: 0x8,
											Type:   &ir.BaseType{},
										},
									},
								},
								Data: &ir.GoStringDataType{
									TypeCommon: ir.TypeCommon{ID: 0x15, Name: "string.str", ByteSize: 0x0},
								},
							},
						},
						ir.Field{
							Name:   "elem",
							Offset: 0x10,
							Type:   &ir.BaseType{},
						},
					},
				},
			},
			0x10: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "noalg.struct { key string; elem int }", ByteSize: 0x18},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x88b40, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "key",
						Offset: 0x0,
						Type: &ir.GoStringHeaderType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee:          &ir.GoStringDataType{},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type:   &ir.BaseType{},
									},
								},
							},
							Data: &ir.GoStringDataType{
								TypeCommon: ir.TypeCommon{ID: 0x15, Name: "string.str", ByteSize: 0x0},
							},
						},
					},
					ir.Field{
						Name:   "elem",
						Offset: 0x10,
						Type:   &ir.BaseType{},
					},
				},
			},
			0x11: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee:          &ir.GoStringDataType{},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type:   &ir.BaseType{},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x15, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x12: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee:          &ir.BaseType{},
			},
			0x13: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x13, Name: "[]*table<string,int>.array", ByteSize: 0x0},
				Element:    &ir.PointerType{},
			},
			0x14: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x14, Name: "[]noalg.map.group[string]int.array", ByteSize: 0x0},
				Element:    &ir.StructureType{},
			},
			0x15: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x15, Name: "string.str", ByteSize: 0x0},
			},
			0x16: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0x17: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x17, Name: "ProbeEvent", ByteSize: 0x1},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "m",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.GoMapType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x0,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x17,
	}
	main_test_map_string_to_int_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x60, 0xde, 0x53, 0x84, 0x39, 0x74, 0xaa, 0xf5, 0xf2, 0x81, 0xad, 0x82, 0x43, 0x6b, 0x0, 0x0, 0x10, 0x86, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xb8, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x17, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_interface
	main_test_interface_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_interface",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_interface",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d8360, 0x3d8550},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "b",
							Type: &ir.GoInterfaceType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.behavior", ByteSize: 0x0},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x6d240, GoKind: 0x14},
								UnderlyingStructure: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "runtime.iface", ByteSize: 0x10},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "tab",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*internal/abi.ITab", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
												Pointee: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "internal/abi.ITab", ByteSize: 0x20},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
													Fields: []ir.Field{
														ir.Field{
															Name:   "Inter",
															Offset: 0x0,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
																Pointee: &ir.StructureType{
																	TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
																	Fields: []ir.Field{
																		ir.Field{
																			Name:   "Type",
																			Offset: 0x0,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "PkgPath",
																			Offset: 0x30,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Methods",
																			Offset: 0x38,
																			Type:   nil,
																		},
																	},
																},
															},
														},
														ir.Field{
															Name:   "Type",
															Offset: 0x8,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*internal/abi.Type", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
																Pointee: &ir.StructureType{
																	TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
																	Fields: []ir.Field{
																		ir.Field{
																			Name:   "Size_",
																			Offset: 0x0,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "PtrBytes",
																			Offset: 0x8,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Hash",
																			Offset: 0x10,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "TFlag",
																			Offset: 0x14,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Align_",
																			Offset: 0x15,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "FieldAlign_",
																			Offset: 0x16,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Kind_",
																			Offset: 0x17,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Equal",
																			Offset: 0x18,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "GCData",
																			Offset: 0x20,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Str",
																			Offset: 0x28,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "PtrToThis",
																			Offset: 0x2c,
																			Type:   nil,
																		},
																	},
																},
															},
														},
														ir.Field{
															Name:   "Hash",
															Offset: 0x10,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
															},
														},
														ir.Field{
															Name:   "Fun",
															Offset: 0x18,
															Type: &ir.ArrayType{
																TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "[1]uintptr", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
																Count:            0x1,
																HasCount:         true,
																Element: &ir.BaseType{
																	TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
																},
															},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "data",
											Offset: 0x8,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "unsafe.Pointer", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
												Pointee:          nil,
											},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8360, 0x3d838c},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									},
								},
								ir.Location{
									Range: ir.PCRange{0x3d838c, 0x3d8390},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 0, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "~r0",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x1c, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range:  ir.PCRange{0x3d8360, 0x3d8550},
									Pieces: []locexpr.LocationPiece{},
								},
							},
							IsParameter: true,
							IsReturn:    true,
						},
						&ir.Variable{
							Name: "hash",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x1c, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d83c0, 0x3d83c4},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 16, InReg: true, StackOffset: 0, Register: 0},
									},
								},
								ir.Location{
									Range: ir.PCRange{0x3d83c4, 0x3d83d0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -88, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									},
								},
								ir.Location{
									Range: ir.PCRange{0x3d83d0, 0x3d83d4},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -88, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -128, Register: 0},
									},
								},
								ir.Location{
									Range: ir.PCRange{0x3d83d4, 0x3d8550},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -88, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -128, Register: 0},
									},
								},
							},
							IsParameter: false,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "inter",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x1c, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8404, 0x3d8408},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 16, InReg: true, StackOffset: 0, Register: 0},
									},
								},
								ir.Location{
									Range: ir.PCRange{0x3d8408, 0x3d8414},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -112, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									},
								},
								ir.Location{
									Range: ir.PCRange{0x3d8414, 0x3d8418},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -112, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -152, Register: 0},
									},
								},
								ir.Location{
									Range: ir.PCRange{0x3d8418, 0x3d8550},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -112, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -152, Register: 0},
									},
								},
							},
							IsParameter: false,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "iType",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x1c, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8448, 0x3d844c},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 16, InReg: true, StackOffset: 0, Register: 0},
									},
								},
								ir.Location{
									Range: ir.PCRange{0x3d844c, 0x3d8458},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -104, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									},
								},
								ir.Location{
									Range: ir.PCRange{0x3d8458, 0x3d8464},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -104, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -144, Register: 0},
									},
								},
								ir.Location{
									Range: ir.PCRange{0x3d8464, 0x3d8550},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -104, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -144, Register: 0},
									},
								},
							},
							IsParameter: false,
							IsReturn:    false,
						},
						&ir.Variable{
							Name: "iFun",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x1c, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8494, 0x3d8498},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 16, InReg: true, StackOffset: 0, Register: 0},
									},
								},
								ir.Location{
									Range: ir.PCRange{0x3d8498, 0x3d84b0},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -96, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									},
								},
								ir.Location{
									Range: ir.PCRange{0x3d84b0, 0x3d84b4},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -96, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -136, Register: 0},
									},
								},
								ir.Location{
									Range: ir.PCRange{0x3d84b4, 0x3d8550},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -96, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -136, Register: 0},
									},
								},
							},
							IsParameter: false,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x1e, Name: "ProbeEvent", ByteSize: 0x1},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "b",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.GoInterfaceType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x0,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d8370},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_interface",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d8360, 0x3d8550},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "b",
						Type: &ir.GoInterfaceType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.behavior", ByteSize: 0x0},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x6d240, GoKind: 0x14},
							UnderlyingStructure: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "runtime.iface", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "tab",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*internal/abi.ITab", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "internal/abi.ITab", ByteSize: 0x20},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Inter",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
															Pointee: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Type",
																		Offset: 0x0,
																		Type: &ir.StructureType{
																			TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
																			Fields: []ir.Field{
																				ir.Field{
																					Name:   "Size_",
																					Offset: 0x0,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "PtrBytes",
																					Offset: 0x8,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Hash",
																					Offset: 0x10,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "TFlag",
																					Offset: 0x14,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Align_",
																					Offset: 0x15,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "FieldAlign_",
																					Offset: 0x16,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Kind_",
																					Offset: 0x17,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Equal",
																					Offset: 0x18,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "GCData",
																					Offset: 0x20,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Str",
																					Offset: 0x28,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "PtrToThis",
																					Offset: 0x2c,
																					Type:   nil,
																				},
																			},
																		},
																	},
																	ir.Field{
																		Name:   "PkgPath",
																		Offset: 0x30,
																		Type: &ir.StructureType{
																			TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
																			Fields: []ir.Field{
																				ir.Field{
																					Name:   "Bytes",
																					Offset: 0x0,
																					Type:   nil,
																				},
																			},
																		},
																	},
																	ir.Field{
																		Name:   "Methods",
																		Offset: 0x38,
																		Type: &ir.GoSliceHeaderType{
																			StructureType: nil,
																			Data:          nil,
																		},
																	},
																},
															},
														},
													},
													ir.Field{
														Name:   "Type",
														Offset: 0x8,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*internal/abi.Type", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
															Pointee: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Size_",
																		Offset: 0x0,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
																		},
																	},
																	ir.Field{
																		Name:   "PtrBytes",
																		Offset: 0x8,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
																		},
																	},
																	ir.Field{
																		Name:   "Hash",
																		Offset: 0x10,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "TFlag",
																		Offset: 0x14,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Align_",
																		Offset: 0x15,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "FieldAlign_",
																		Offset: 0x16,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Kind_",
																		Offset: 0x17,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Equal",
																		Offset: 0x18,
																		Type: &ir.GoSubroutineType{
																			TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
																		},
																	},
																	ir.Field{
																		Name:   "GCData",
																		Offset: 0x20,
																		Type: &ir.PointerType{
																			TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
																			Pointee:          nil,
																		},
																	},
																	ir.Field{
																		Name:   "Str",
																		Offset: 0x28,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
																		},
																	},
																	ir.Field{
																		Name:   "PtrToThis",
																		Offset: 0x2c,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
																		},
																	},
																},
															},
														},
													},
													ir.Field{
														Name:   "Hash",
														Offset: 0x10,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
														},
													},
													ir.Field{
														Name:   "Fun",
														Offset: 0x18,
														Type: &ir.ArrayType{
															TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "[1]uintptr", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
															Count:            0x1,
															HasCount:         true,
															Element: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
															},
														},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "data",
										Offset: 0x8,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "unsafe.Pointer", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
											Pointee:          nil,
										},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8360, 0x3d838c},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
								},
							},
							ir.Location{
								Range: ir.PCRange{0x3d838c, 0x3d8390},
								Pieces: []locexpr.LocationPiece{
									{Size: 0, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "~r0",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "string", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
							Fields: []ir.Field{
								ir.Field{
									Name:   "str",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "*string.str", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x1c, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range:  ir.PCRange{0x3d8360, 0x3d8550},
								Pieces: []locexpr.LocationPiece{},
							},
						},
						IsParameter: true,
						IsReturn:    true,
					},
					&ir.Variable{
						Name: "hash",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "string", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
							Fields: []ir.Field{
								ir.Field{
									Name:   "str",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "*string.str", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x1c, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d83c0, 0x3d83c4},
								Pieces: []locexpr.LocationPiece{
									{Size: 16, InReg: true, StackOffset: 0, Register: 0},
								},
							},
							ir.Location{
								Range: ir.PCRange{0x3d83c4, 0x3d83d0},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: false, StackOffset: -88, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
								},
							},
							ir.Location{
								Range: ir.PCRange{0x3d83d0, 0x3d83d4},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: false, StackOffset: -88, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -128, Register: 0},
								},
							},
							ir.Location{
								Range: ir.PCRange{0x3d83d4, 0x3d8550},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: false, StackOffset: -88, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -128, Register: 0},
								},
							},
						},
						IsParameter: false,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "inter",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "string", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
							Fields: []ir.Field{
								ir.Field{
									Name:   "str",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "*string.str", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x1c, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8404, 0x3d8408},
								Pieces: []locexpr.LocationPiece{
									{Size: 16, InReg: true, StackOffset: 0, Register: 0},
								},
							},
							ir.Location{
								Range: ir.PCRange{0x3d8408, 0x3d8414},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: false, StackOffset: -112, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
								},
							},
							ir.Location{
								Range: ir.PCRange{0x3d8414, 0x3d8418},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: false, StackOffset: -112, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -152, Register: 0},
								},
							},
							ir.Location{
								Range: ir.PCRange{0x3d8418, 0x3d8550},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: false, StackOffset: -112, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -152, Register: 0},
								},
							},
						},
						IsParameter: false,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "iType",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "string", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
							Fields: []ir.Field{
								ir.Field{
									Name:   "str",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "*string.str", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x1c, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8448, 0x3d844c},
								Pieces: []locexpr.LocationPiece{
									{Size: 16, InReg: true, StackOffset: 0, Register: 0},
								},
							},
							ir.Location{
								Range: ir.PCRange{0x3d844c, 0x3d8458},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: false, StackOffset: -104, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
								},
							},
							ir.Location{
								Range: ir.PCRange{0x3d8458, 0x3d8464},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: false, StackOffset: -104, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -144, Register: 0},
								},
							},
							ir.Location{
								Range: ir.PCRange{0x3d8464, 0x3d8550},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: false, StackOffset: -104, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -144, Register: 0},
								},
							},
						},
						IsParameter: false,
						IsReturn:    false,
					},
					&ir.Variable{
						Name: "iFun",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "string", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
							Fields: []ir.Field{
								ir.Field{
									Name:   "str",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "*string.str", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x1c, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8494, 0x3d8498},
								Pieces: []locexpr.LocationPiece{
									{Size: 16, InReg: true, StackOffset: 0, Register: 0},
								},
							},
							ir.Location{
								Range: ir.PCRange{0x3d8498, 0x3d84b0},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: false, StackOffset: -96, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
								},
							},
							ir.Location{
								Range: ir.PCRange{0x3d84b0, 0x3d84b4},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: false, StackOffset: -96, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -136, Register: 0},
								},
							},
							ir.Location{
								Range: ir.PCRange{0x3d84b4, 0x3d8550},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: false, StackOffset: -96, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: false, StackOffset: -136, Register: 0},
								},
							},
						},
						IsParameter: false,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.GoInterfaceType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.behavior", ByteSize: 0x0},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x6d240, GoKind: 0x14},
				UnderlyingStructure: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "runtime.iface", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "tab",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*internal/abi.ITab", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "internal/abi.ITab", ByteSize: 0x20},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "Inter",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
												Pointee: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
													Fields: []ir.Field{
														ir.Field{
															Name:   "Type",
															Offset: 0x0,
															Type: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Size_",
																		Offset: 0x0,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "PtrBytes",
																		Offset: 0x8,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "Hash",
																		Offset: 0x10,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "TFlag",
																		Offset: 0x14,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Align_",
																		Offset: 0x15,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "FieldAlign_",
																		Offset: 0x16,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Kind_",
																		Offset: 0x17,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Equal",
																		Offset: 0x18,
																		Type: &ir.GoSubroutineType{
																			TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
																		},
																	},
																	ir.Field{
																		Name:   "GCData",
																		Offset: 0x20,
																		Type: &ir.PointerType{
																			TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
																			Pointee:          nil,
																		},
																	},
																	ir.Field{
																		Name:   "Str",
																		Offset: 0x28,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
																		},
																	},
																	ir.Field{
																		Name:   "PtrToThis",
																		Offset: 0x2c,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
																		},
																	},
																},
															},
														},
														ir.Field{
															Name:   "PkgPath",
															Offset: 0x30,
															Type: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Bytes",
																		Offset: 0x0,
																		Type: &ir.PointerType{
																			TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
																			Pointee:          nil,
																		},
																	},
																},
															},
														},
														ir.Field{
															Name:   "Methods",
															Offset: 0x38,
															Type: &ir.GoSliceHeaderType{
																StructureType: &ir.StructureType{
																	TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
																	Fields: []ir.Field{
																		ir.Field{
																			Name:   "array",
																			Offset: 0x0,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "len",
																			Offset: 0x8,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "cap",
																			Offset: 0x10,
																			Type:   nil,
																		},
																	},
																},
																Data: &ir.GoSliceDataType{
																	TypeCommon: ir.TypeCommon{ID: 0x1a, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
																	Element:    nil,
																},
															},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "Type",
											Offset: 0x8,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*internal/abi.Type", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
												Pointee: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
													Fields: []ir.Field{
														ir.Field{
															Name:   "Size_",
															Offset: 0x0,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
															},
														},
														ir.Field{
															Name:   "PtrBytes",
															Offset: 0x8,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
															},
														},
														ir.Field{
															Name:   "Hash",
															Offset: 0x10,
															Type:   &ir.BaseType{},
														},
														ir.Field{
															Name:   "TFlag",
															Offset: 0x14,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
															},
														},
														ir.Field{
															Name:   "Align_",
															Offset: 0x15,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
															},
														},
														ir.Field{
															Name:   "FieldAlign_",
															Offset: 0x16,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
															},
														},
														ir.Field{
															Name:   "Kind_",
															Offset: 0x17,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
															},
														},
														ir.Field{
															Name:   "Equal",
															Offset: 0x18,
															Type: &ir.GoSubroutineType{
																TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
															},
														},
														ir.Field{
															Name:   "GCData",
															Offset: 0x20,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
																Pointee:          &ir.BaseType{},
															},
														},
														ir.Field{
															Name:   "Str",
															Offset: 0x28,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
															},
														},
														ir.Field{
															Name:   "PtrToThis",
															Offset: 0x2c,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
															},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "Hash",
											Offset: 0x10,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
											},
										},
										ir.Field{
											Name:   "Fun",
											Offset: 0x18,
											Type: &ir.ArrayType{
												TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "[1]uintptr", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
												Count:            0x1,
												HasCount:         true,
												Element: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
												},
											},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "data",
							Offset: 0x8,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "unsafe.Pointer", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
								Pointee:          nil,
							},
						},
					},
				},
			},
			0x2: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "runtime.iface", ByteSize: 0x10},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "tab",
						Offset: 0x0,
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*internal/abi.ITab", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
							Pointee: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "internal/abi.ITab", ByteSize: 0x20},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "Inter",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Type",
														Offset: 0x0,
														Type: &ir.StructureType{
															TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
															Fields: []ir.Field{
																ir.Field{
																	Name:   "Size_",
																	Offset: 0x0,
																	Type:   &ir.BaseType{},
																},
																ir.Field{
																	Name:   "PtrBytes",
																	Offset: 0x8,
																	Type:   &ir.BaseType{},
																},
																ir.Field{
																	Name:   "Hash",
																	Offset: 0x10,
																	Type:   &ir.BaseType{},
																},
																ir.Field{
																	Name:   "TFlag",
																	Offset: 0x14,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
																	},
																},
																ir.Field{
																	Name:   "Align_",
																	Offset: 0x15,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
																	},
																},
																ir.Field{
																	Name:   "FieldAlign_",
																	Offset: 0x16,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
																	},
																},
																ir.Field{
																	Name:   "Kind_",
																	Offset: 0x17,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
																	},
																},
																ir.Field{
																	Name:   "Equal",
																	Offset: 0x18,
																	Type: &ir.GoSubroutineType{
																		TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
																	},
																},
																ir.Field{
																	Name:   "GCData",
																	Offset: 0x20,
																	Type: &ir.PointerType{
																		TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
																		Pointee:          &ir.BaseType{},
																	},
																},
																ir.Field{
																	Name:   "Str",
																	Offset: 0x28,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
																	},
																},
																ir.Field{
																	Name:   "PtrToThis",
																	Offset: 0x2c,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
																	},
																},
															},
														},
													},
													ir.Field{
														Name:   "PkgPath",
														Offset: 0x30,
														Type: &ir.StructureType{
															TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
															Fields: []ir.Field{
																ir.Field{
																	Name:   "Bytes",
																	Offset: 0x0,
																	Type: &ir.PointerType{
																		TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
																		Pointee:          &ir.BaseType{},
																	},
																},
															},
														},
													},
													ir.Field{
														Name:   "Methods",
														Offset: 0x38,
														Type: &ir.GoSliceHeaderType{
															StructureType: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "array",
																		Offset: 0x0,
																		Type: &ir.PointerType{
																			TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "*internal/abi.Imethod", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
																			Pointee:          nil,
																		},
																	},
																	ir.Field{
																		Name:   "len",
																		Offset: 0x8,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "cap",
																		Offset: 0x10,
																		Type:   &ir.BaseType{},
																	},
																},
															},
															Data: &ir.GoSliceDataType{
																TypeCommon: ir.TypeCommon{ID: 0x1a, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
																Element: &ir.StructureType{
																	TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
																	Fields: []ir.Field{
																		ir.Field{
																			Name:   "Name",
																			Offset: 0x0,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Typ",
																			Offset: 0x4,
																			Type:   nil,
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "Type",
										Offset: 0x8,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*internal/abi.Type", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Size_",
														Offset: 0x0,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
														},
													},
													ir.Field{
														Name:   "PtrBytes",
														Offset: 0x8,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
														},
													},
													ir.Field{
														Name:   "Hash",
														Offset: 0x10,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "TFlag",
														Offset: 0x14,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Align_",
														Offset: 0x15,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "FieldAlign_",
														Offset: 0x16,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Kind_",
														Offset: 0x17,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Equal",
														Offset: 0x18,
														Type: &ir.GoSubroutineType{
															TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
														},
													},
													ir.Field{
														Name:   "GCData",
														Offset: 0x20,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
															Pointee:          &ir.BaseType{},
														},
													},
													ir.Field{
														Name:   "Str",
														Offset: 0x28,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
														},
													},
													ir.Field{
														Name:   "PtrToThis",
														Offset: 0x2c,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
														},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "Hash",
										Offset: 0x10,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
										},
									},
									ir.Field{
										Name:   "Fun",
										Offset: 0x18,
										Type: &ir.ArrayType{
											TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "[1]uintptr", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
											Count:            0x1,
											HasCount:         true,
											Element: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
											},
										},
									},
								},
							},
						},
					},
					ir.Field{
						Name:   "data",
						Offset: 0x8,
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "unsafe.Pointer", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
							Pointee:          nil,
						},
					},
				},
			},
			0x3: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*internal/abi.ITab", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "internal/abi.ITab", ByteSize: 0x20},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "Inter",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "Type",
											Offset: 0x0,
											Type: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Size_",
														Offset: 0x0,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "PtrBytes",
														Offset: 0x8,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "Hash",
														Offset: 0x10,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "TFlag",
														Offset: 0x14,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Align_",
														Offset: 0x15,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "FieldAlign_",
														Offset: 0x16,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Kind_",
														Offset: 0x17,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Equal",
														Offset: 0x18,
														Type: &ir.GoSubroutineType{
															TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
														},
													},
													ir.Field{
														Name:   "GCData",
														Offset: 0x20,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
															Pointee:          &ir.BaseType{},
														},
													},
													ir.Field{
														Name:   "Str",
														Offset: 0x28,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
														},
													},
													ir.Field{
														Name:   "PtrToThis",
														Offset: 0x2c,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "PkgPath",
											Offset: 0x30,
											Type: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Bytes",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
															Pointee:          &ir.BaseType{},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "Methods",
											Offset: 0x38,
											Type: &ir.GoSliceHeaderType{
												StructureType: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
													Fields: []ir.Field{
														ir.Field{
															Name:   "array",
															Offset: 0x0,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "*internal/abi.Imethod", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
																Pointee: &ir.StructureType{
																	TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
																	Fields: []ir.Field{
																		ir.Field{
																			Name:   "Name",
																			Offset: 0x0,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Typ",
																			Offset: 0x4,
																			Type:   nil,
																		},
																	},
																},
															},
														},
														ir.Field{
															Name:   "len",
															Offset: 0x8,
															Type:   &ir.BaseType{},
														},
														ir.Field{
															Name:   "cap",
															Offset: 0x10,
															Type:   &ir.BaseType{},
														},
													},
												},
												Data: &ir.GoSliceDataType{
													TypeCommon: ir.TypeCommon{ID: 0x1a, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
													Element: &ir.StructureType{
														TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
														Fields: []ir.Field{
															ir.Field{
																Name:   "Name",
																Offset: 0x0,
																Type:   &ir.BaseType{},
															},
															ir.Field{
																Name:   "Typ",
																Offset: 0x4,
																Type:   &ir.BaseType{},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "Type",
							Offset: 0x8,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*internal/abi.Type", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "Size_",
											Offset: 0x0,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
											},
										},
										ir.Field{
											Name:   "PtrBytes",
											Offset: 0x8,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
											},
										},
										ir.Field{
											Name:   "Hash",
											Offset: 0x10,
											Type:   &ir.BaseType{},
										},
										ir.Field{
											Name:   "TFlag",
											Offset: 0x14,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "Align_",
											Offset: 0x15,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "FieldAlign_",
											Offset: 0x16,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "Kind_",
											Offset: 0x17,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "Equal",
											Offset: 0x18,
											Type: &ir.GoSubroutineType{
												TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
											},
										},
										ir.Field{
											Name:   "GCData",
											Offset: 0x20,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
												Pointee:          &ir.BaseType{},
											},
										},
										ir.Field{
											Name:   "Str",
											Offset: 0x28,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
											},
										},
										ir.Field{
											Name:   "PtrToThis",
											Offset: 0x2c,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
											},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "Hash",
							Offset: 0x10,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
							},
						},
						ir.Field{
							Name:   "Fun",
							Offset: 0x18,
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "[1]uintptr", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
								Count:            0x1,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
								},
							},
						},
					},
				},
			},
			0x4: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "internal/abi.ITab", ByteSize: 0x20},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "Inter",
						Offset: 0x0,
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
							Pointee: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "Type",
										Offset: 0x0,
										Type: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
											Fields: []ir.Field{
												ir.Field{
													Name:   "Size_",
													Offset: 0x0,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "PtrBytes",
													Offset: 0x8,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "Hash",
													Offset: 0x10,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "TFlag",
													Offset: 0x14,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
													},
												},
												ir.Field{
													Name:   "Align_",
													Offset: 0x15,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
													},
												},
												ir.Field{
													Name:   "FieldAlign_",
													Offset: 0x16,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
													},
												},
												ir.Field{
													Name:   "Kind_",
													Offset: 0x17,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
													},
												},
												ir.Field{
													Name:   "Equal",
													Offset: 0x18,
													Type: &ir.GoSubroutineType{
														TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
													},
												},
												ir.Field{
													Name:   "GCData",
													Offset: 0x20,
													Type: &ir.PointerType{
														TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
														Pointee:          &ir.BaseType{},
													},
												},
												ir.Field{
													Name:   "Str",
													Offset: 0x28,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
													},
												},
												ir.Field{
													Name:   "PtrToThis",
													Offset: 0x2c,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "PkgPath",
										Offset: 0x30,
										Type: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
											Fields: []ir.Field{
												ir.Field{
													Name:   "Bytes",
													Offset: 0x0,
													Type: &ir.PointerType{
														TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
														Pointee:          &ir.BaseType{},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "Methods",
										Offset: 0x38,
										Type: &ir.GoSliceHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
												Fields: []ir.Field{
													ir.Field{
														Name:   "array",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "*internal/abi.Imethod", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
															Pointee: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Name",
																		Offset: 0x0,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "Typ",
																		Offset: 0x4,
																		Type:   &ir.BaseType{},
																	},
																},
															},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "cap",
														Offset: 0x10,
														Type:   &ir.BaseType{},
													},
												},
											},
											Data: &ir.GoSliceDataType{
												TypeCommon: ir.TypeCommon{ID: 0x1a, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
												Element: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
													Fields: []ir.Field{
														ir.Field{
															Name:   "Name",
															Offset: 0x0,
															Type:   &ir.BaseType{},
														},
														ir.Field{
															Name:   "Typ",
															Offset: 0x4,
															Type:   &ir.BaseType{},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
					ir.Field{
						Name:   "Type",
						Offset: 0x8,
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*internal/abi.Type", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
							Pointee: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "Size_",
										Offset: 0x0,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
										},
									},
									ir.Field{
										Name:   "PtrBytes",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
										},
									},
									ir.Field{
										Name:   "Hash",
										Offset: 0x10,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "TFlag",
										Offset: 0x14,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Align_",
										Offset: 0x15,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "FieldAlign_",
										Offset: 0x16,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Kind_",
										Offset: 0x17,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Equal",
										Offset: 0x18,
										Type: &ir.GoSubroutineType{
											TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
										},
									},
									ir.Field{
										Name:   "GCData",
										Offset: 0x20,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
											Pointee:          &ir.BaseType{},
										},
									},
									ir.Field{
										Name:   "Str",
										Offset: 0x28,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
										},
									},
									ir.Field{
										Name:   "PtrToThis",
										Offset: 0x2c,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
										},
									},
								},
							},
						},
					},
					ir.Field{
						Name:   "Hash",
						Offset: 0x10,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
						},
					},
					ir.Field{
						Name:   "Fun",
						Offset: 0x18,
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "[1]uintptr", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
							Count:            0x1,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
							},
						},
					},
				},
			},
			0x5: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "Type",
							Offset: 0x0,
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "Size_",
										Offset: 0x0,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "PtrBytes",
										Offset: 0x8,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "Hash",
										Offset: 0x10,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "TFlag",
										Offset: 0x14,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Align_",
										Offset: 0x15,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "FieldAlign_",
										Offset: 0x16,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Kind_",
										Offset: 0x17,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Equal",
										Offset: 0x18,
										Type: &ir.GoSubroutineType{
											TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
										},
									},
									ir.Field{
										Name:   "GCData",
										Offset: 0x20,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
											Pointee:          &ir.BaseType{},
										},
									},
									ir.Field{
										Name:   "Str",
										Offset: 0x28,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
										},
									},
									ir.Field{
										Name:   "PtrToThis",
										Offset: 0x2c,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "PkgPath",
							Offset: 0x30,
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "Bytes",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
											Pointee:          &ir.BaseType{},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "Methods",
							Offset: 0x38,
							Type: &ir.GoSliceHeaderType{
								StructureType: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
									Fields: []ir.Field{
										ir.Field{
											Name:   "array",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "*internal/abi.Imethod", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
												Pointee: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
													Fields: []ir.Field{
														ir.Field{
															Name:   "Name",
															Offset: 0x0,
															Type:   &ir.BaseType{},
														},
														ir.Field{
															Name:   "Typ",
															Offset: 0x4,
															Type:   &ir.BaseType{},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "len",
											Offset: 0x8,
											Type:   &ir.BaseType{},
										},
										ir.Field{
											Name:   "cap",
											Offset: 0x10,
											Type:   &ir.BaseType{},
										},
									},
								},
								Data: &ir.GoSliceDataType{
									TypeCommon: ir.TypeCommon{ID: 0x1a, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
									Element: &ir.StructureType{
										TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
										Fields: []ir.Field{
											ir.Field{
												Name:   "Name",
												Offset: 0x0,
												Type:   &ir.BaseType{},
											},
											ir.Field{
												Name:   "Typ",
												Offset: 0x4,
												Type:   &ir.BaseType{},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			0x6: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "Type",
						Offset: 0x0,
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
							Fields: []ir.Field{
								ir.Field{
									Name:   "Size_",
									Offset: 0x0,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
									},
								},
								ir.Field{
									Name:   "PtrBytes",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
									},
								},
								ir.Field{
									Name:   "Hash",
									Offset: 0x10,
									Type:   &ir.BaseType{},
								},
								ir.Field{
									Name:   "TFlag",
									Offset: 0x14,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
									},
								},
								ir.Field{
									Name:   "Align_",
									Offset: 0x15,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
									},
								},
								ir.Field{
									Name:   "FieldAlign_",
									Offset: 0x16,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
									},
								},
								ir.Field{
									Name:   "Kind_",
									Offset: 0x17,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
									},
								},
								ir.Field{
									Name:   "Equal",
									Offset: 0x18,
									Type: &ir.GoSubroutineType{
										TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
									},
								},
								ir.Field{
									Name:   "GCData",
									Offset: 0x20,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
										Pointee:          &ir.BaseType{},
									},
								},
								ir.Field{
									Name:   "Str",
									Offset: 0x28,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
									},
								},
								ir.Field{
									Name:   "PtrToThis",
									Offset: 0x2c,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
									},
								},
							},
						},
					},
					ir.Field{
						Name:   "PkgPath",
						Offset: 0x30,
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
							Fields: []ir.Field{
								ir.Field{
									Name:   "Bytes",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
										Pointee:          &ir.BaseType{},
									},
								},
							},
						},
					},
					ir.Field{
						Name:   "Methods",
						Offset: 0x38,
						Type: &ir.GoSliceHeaderType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
								Fields: []ir.Field{
									ir.Field{
										Name:   "array",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "*internal/abi.Imethod", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Name",
														Offset: 0x0,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "Typ",
														Offset: 0x4,
														Type:   &ir.BaseType{},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "cap",
										Offset: 0x10,
										Type:   &ir.BaseType{},
									},
								},
							},
							Data: &ir.GoSliceDataType{
								TypeCommon: ir.TypeCommon{ID: 0x1a, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
								Element: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "Name",
											Offset: 0x0,
											Type:   &ir.BaseType{},
										},
										ir.Field{
											Name:   "Typ",
											Offset: 0x4,
											Type:   &ir.BaseType{},
										},
									},
								},
							},
						},
					},
				},
			},
			0x7: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "Size_",
						Offset: 0x0,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
						},
					},
					ir.Field{
						Name:   "PtrBytes",
						Offset: 0x8,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
						},
					},
					ir.Field{
						Name:   "Hash",
						Offset: 0x10,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
						},
					},
					ir.Field{
						Name:   "TFlag",
						Offset: 0x14,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
						},
					},
					ir.Field{
						Name:   "Align_",
						Offset: 0x15,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
						},
					},
					ir.Field{
						Name:   "FieldAlign_",
						Offset: 0x16,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
						},
					},
					ir.Field{
						Name:   "Kind_",
						Offset: 0x17,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
						},
					},
					ir.Field{
						Name:   "Equal",
						Offset: 0x18,
						Type: &ir.GoSubroutineType{
							TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
						},
					},
					ir.Field{
						Name:   "GCData",
						Offset: 0x20,
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
							Pointee:          &ir.BaseType{},
						},
					},
					ir.Field{
						Name:   "Str",
						Offset: 0x28,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
						},
					},
					ir.Field{
						Name:   "PtrToThis",
						Offset: 0x2c,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
						},
					},
				},
			},
			0x8: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
			},
			0x9: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
			},
			0xa: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
			},
			0xb: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0xc: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
			},
			0xd: &ir.GoSubroutineType{
				TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
			},
			0xe: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee:          &ir.BaseType{},
			},
			0xf: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
			},
			0x10: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
			},
			0x11: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "Bytes",
						Offset: 0x0,
						Type:   &ir.PointerType{},
					},
				},
			},
			0x12: &ir.GoSliceHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
					Fields: []ir.Field{
						ir.Field{
							Name:   "array",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "*internal/abi.Imethod", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "Name",
											Offset: 0x0,
											Type:   &ir.BaseType{},
										},
										ir.Field{
											Name:   "Typ",
											Offset: 0x4,
											Type:   &ir.BaseType{},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
						ir.Field{
							Name:   "cap",
							Offset: 0x10,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x1a, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
					Element: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
						Fields: []ir.Field{
							ir.Field{
								Name:   "Name",
								Offset: 0x0,
								Type:   &ir.BaseType{},
							},
							ir.Field{
								Name:   "Typ",
								Offset: 0x4,
								Type:   &ir.BaseType{},
							},
						},
					},
				},
			},
			0x13: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "*internal/abi.Imethod", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "Name",
							Offset: 0x0,
							Type:   &ir.BaseType{},
						},
						ir.Field{
							Name:   "Typ",
							Offset: 0x4,
							Type:   &ir.BaseType{},
						},
					},
				},
			},
			0x14: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "Name",
						Offset: 0x0,
						Type:   &ir.BaseType{},
					},
					ir.Field{
						Name:   "Typ",
						Offset: 0x4,
						Type:   &ir.BaseType{},
					},
				},
			},
			0x15: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x16: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*internal/abi.Type", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
				Pointee:          &ir.StructureType{},
			},
			0x17: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "[1]uintptr", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
				Count:            0x1,
				HasCount:         true,
				Element:          &ir.BaseType{},
			},
			0x18: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "unsafe.Pointer", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
				Pointee:          nil,
			},
			0x19: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee: &ir.GoStringDataType{
									TypeCommon: ir.TypeCommon{ID: 0x1c, Name: "string.str", ByteSize: 0x0},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type:   &ir.BaseType{},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x1c, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x1a: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x1a, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
				Element:    &ir.StructureType{},
			},
			0x1b: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x1b, Name: "*[]internal/abi.Imethod.array", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoSliceDataType{},
			},
			0x1c: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x1c, Name: "string.str", ByteSize: 0x0},
			},
			0x1d: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0x1e: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x1e, Name: "ProbeEvent", ByteSize: 0x1},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "b",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.GoInterfaceType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x0,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x1e,
	}
	main_test_interface_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4f, 0x64, 0xa4, 0x16, 0x9a, 0x49, 0x44, 0x20, 0x3, 0x3c, 0x9e, 0xa6, 0x43, 0x6b, 0x0, 0x0, 0x60, 0x83, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xbc, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1e, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_error
	main_test_error_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_error",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_error",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d8550, 0x3d8560},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "e",
							Type: &ir.GoInterfaceType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "error", ByteSize: 0x0},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x6dac0, GoKind: 0x14},
								UnderlyingStructure: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "runtime.iface", ByteSize: 0x10},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "tab",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*internal/abi.ITab", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
												Pointee: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "internal/abi.ITab", ByteSize: 0x20},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
													Fields: []ir.Field{
														ir.Field{
															Name:   "Inter",
															Offset: 0x0,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
																Pointee: &ir.StructureType{
																	TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
																	Fields: []ir.Field{
																		ir.Field{
																			Name:   "Type",
																			Offset: 0x0,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "PkgPath",
																			Offset: 0x30,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Methods",
																			Offset: 0x38,
																			Type:   nil,
																		},
																	},
																},
															},
														},
														ir.Field{
															Name:   "Type",
															Offset: 0x8,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*internal/abi.Type", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
																Pointee: &ir.StructureType{
																	TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
																	Fields: []ir.Field{
																		ir.Field{
																			Name:   "Size_",
																			Offset: 0x0,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "PtrBytes",
																			Offset: 0x8,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Hash",
																			Offset: 0x10,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "TFlag",
																			Offset: 0x14,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Align_",
																			Offset: 0x15,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "FieldAlign_",
																			Offset: 0x16,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Kind_",
																			Offset: 0x17,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Equal",
																			Offset: 0x18,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "GCData",
																			Offset: 0x20,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Str",
																			Offset: 0x28,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "PtrToThis",
																			Offset: 0x2c,
																			Type:   nil,
																		},
																	},
																},
															},
														},
														ir.Field{
															Name:   "Hash",
															Offset: 0x10,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
															},
														},
														ir.Field{
															Name:   "Fun",
															Offset: 0x18,
															Type: &ir.ArrayType{
																TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "[1]uintptr", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
																Count:            0x1,
																HasCount:         true,
																Element: &ir.BaseType{
																	TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
																},
															},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "data",
											Offset: 0x8,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "unsafe.Pointer", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
												Pointee:          nil,
											},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8550, 0x3d8560},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x1b, Name: "ProbeEvent", ByteSize: 0x1},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "e",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.GoInterfaceType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x0,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d8550},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_error",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d8550, 0x3d8560},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "e",
						Type: &ir.GoInterfaceType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "error", ByteSize: 0x0},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x6dac0, GoKind: 0x14},
							UnderlyingStructure: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "runtime.iface", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "tab",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*internal/abi.ITab", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "internal/abi.ITab", ByteSize: 0x20},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Inter",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
															Pointee: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Type",
																		Offset: 0x0,
																		Type: &ir.StructureType{
																			TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
																			Fields: []ir.Field{
																				ir.Field{
																					Name:   "Size_",
																					Offset: 0x0,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "PtrBytes",
																					Offset: 0x8,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Hash",
																					Offset: 0x10,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "TFlag",
																					Offset: 0x14,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Align_",
																					Offset: 0x15,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "FieldAlign_",
																					Offset: 0x16,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Kind_",
																					Offset: 0x17,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Equal",
																					Offset: 0x18,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "GCData",
																					Offset: 0x20,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Str",
																					Offset: 0x28,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "PtrToThis",
																					Offset: 0x2c,
																					Type:   nil,
																				},
																			},
																		},
																	},
																	ir.Field{
																		Name:   "PkgPath",
																		Offset: 0x30,
																		Type: &ir.StructureType{
																			TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
																			Fields: []ir.Field{
																				ir.Field{
																					Name:   "Bytes",
																					Offset: 0x0,
																					Type:   nil,
																				},
																			},
																		},
																	},
																	ir.Field{
																		Name:   "Methods",
																		Offset: 0x38,
																		Type: &ir.GoSliceHeaderType{
																			StructureType: nil,
																			Data:          nil,
																		},
																	},
																},
															},
														},
													},
													ir.Field{
														Name:   "Type",
														Offset: 0x8,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*internal/abi.Type", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
															Pointee: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Size_",
																		Offset: 0x0,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
																		},
																	},
																	ir.Field{
																		Name:   "PtrBytes",
																		Offset: 0x8,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
																		},
																	},
																	ir.Field{
																		Name:   "Hash",
																		Offset: 0x10,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "TFlag",
																		Offset: 0x14,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Align_",
																		Offset: 0x15,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "FieldAlign_",
																		Offset: 0x16,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Kind_",
																		Offset: 0x17,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Equal",
																		Offset: 0x18,
																		Type: &ir.GoSubroutineType{
																			TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
																		},
																	},
																	ir.Field{
																		Name:   "GCData",
																		Offset: 0x20,
																		Type: &ir.PointerType{
																			TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
																			Pointee:          nil,
																		},
																	},
																	ir.Field{
																		Name:   "Str",
																		Offset: 0x28,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
																		},
																	},
																	ir.Field{
																		Name:   "PtrToThis",
																		Offset: 0x2c,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
																		},
																	},
																},
															},
														},
													},
													ir.Field{
														Name:   "Hash",
														Offset: 0x10,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
														},
													},
													ir.Field{
														Name:   "Fun",
														Offset: 0x18,
														Type: &ir.ArrayType{
															TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "[1]uintptr", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
															Count:            0x1,
															HasCount:         true,
															Element: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
															},
														},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "data",
										Offset: 0x8,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "unsafe.Pointer", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
											Pointee:          nil,
										},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8550, 0x3d8560},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.GoInterfaceType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "error", ByteSize: 0x0},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x6dac0, GoKind: 0x14},
				UnderlyingStructure: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "runtime.iface", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "tab",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*internal/abi.ITab", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "internal/abi.ITab", ByteSize: 0x20},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "Inter",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
												Pointee: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
													Fields: []ir.Field{
														ir.Field{
															Name:   "Type",
															Offset: 0x0,
															Type: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Size_",
																		Offset: 0x0,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "PtrBytes",
																		Offset: 0x8,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "Hash",
																		Offset: 0x10,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "TFlag",
																		Offset: 0x14,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Align_",
																		Offset: 0x15,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "FieldAlign_",
																		Offset: 0x16,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Kind_",
																		Offset: 0x17,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Equal",
																		Offset: 0x18,
																		Type: &ir.GoSubroutineType{
																			TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
																		},
																	},
																	ir.Field{
																		Name:   "GCData",
																		Offset: 0x20,
																		Type: &ir.PointerType{
																			TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
																			Pointee:          nil,
																		},
																	},
																	ir.Field{
																		Name:   "Str",
																		Offset: 0x28,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
																		},
																	},
																	ir.Field{
																		Name:   "PtrToThis",
																		Offset: 0x2c,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
																		},
																	},
																},
															},
														},
														ir.Field{
															Name:   "PkgPath",
															Offset: 0x30,
															Type: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Bytes",
																		Offset: 0x0,
																		Type: &ir.PointerType{
																			TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
																			Pointee:          nil,
																		},
																	},
																},
															},
														},
														ir.Field{
															Name:   "Methods",
															Offset: 0x38,
															Type: &ir.GoSliceHeaderType{
																StructureType: &ir.StructureType{
																	TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
																	Fields: []ir.Field{
																		ir.Field{
																			Name:   "array",
																			Offset: 0x0,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "len",
																			Offset: 0x8,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "cap",
																			Offset: 0x10,
																			Type:   nil,
																		},
																	},
																},
																Data: &ir.GoSliceDataType{
																	TypeCommon: ir.TypeCommon{ID: 0x19, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
																	Element:    nil,
																},
															},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "Type",
											Offset: 0x8,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*internal/abi.Type", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
												Pointee: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
													Fields: []ir.Field{
														ir.Field{
															Name:   "Size_",
															Offset: 0x0,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
															},
														},
														ir.Field{
															Name:   "PtrBytes",
															Offset: 0x8,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
															},
														},
														ir.Field{
															Name:   "Hash",
															Offset: 0x10,
															Type:   &ir.BaseType{},
														},
														ir.Field{
															Name:   "TFlag",
															Offset: 0x14,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
															},
														},
														ir.Field{
															Name:   "Align_",
															Offset: 0x15,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
															},
														},
														ir.Field{
															Name:   "FieldAlign_",
															Offset: 0x16,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
															},
														},
														ir.Field{
															Name:   "Kind_",
															Offset: 0x17,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
															},
														},
														ir.Field{
															Name:   "Equal",
															Offset: 0x18,
															Type: &ir.GoSubroutineType{
																TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
															},
														},
														ir.Field{
															Name:   "GCData",
															Offset: 0x20,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
																Pointee:          &ir.BaseType{},
															},
														},
														ir.Field{
															Name:   "Str",
															Offset: 0x28,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
															},
														},
														ir.Field{
															Name:   "PtrToThis",
															Offset: 0x2c,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
															},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "Hash",
											Offset: 0x10,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
											},
										},
										ir.Field{
											Name:   "Fun",
											Offset: 0x18,
											Type: &ir.ArrayType{
												TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "[1]uintptr", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
												Count:            0x1,
												HasCount:         true,
												Element: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
												},
											},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "data",
							Offset: 0x8,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "unsafe.Pointer", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
								Pointee:          nil,
							},
						},
					},
				},
			},
			0x2: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "runtime.iface", ByteSize: 0x10},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "tab",
						Offset: 0x0,
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*internal/abi.ITab", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
							Pointee: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "internal/abi.ITab", ByteSize: 0x20},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "Inter",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Type",
														Offset: 0x0,
														Type: &ir.StructureType{
															TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
															Fields: []ir.Field{
																ir.Field{
																	Name:   "Size_",
																	Offset: 0x0,
																	Type:   &ir.BaseType{},
																},
																ir.Field{
																	Name:   "PtrBytes",
																	Offset: 0x8,
																	Type:   &ir.BaseType{},
																},
																ir.Field{
																	Name:   "Hash",
																	Offset: 0x10,
																	Type:   &ir.BaseType{},
																},
																ir.Field{
																	Name:   "TFlag",
																	Offset: 0x14,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
																	},
																},
																ir.Field{
																	Name:   "Align_",
																	Offset: 0x15,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
																	},
																},
																ir.Field{
																	Name:   "FieldAlign_",
																	Offset: 0x16,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
																	},
																},
																ir.Field{
																	Name:   "Kind_",
																	Offset: 0x17,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
																	},
																},
																ir.Field{
																	Name:   "Equal",
																	Offset: 0x18,
																	Type: &ir.GoSubroutineType{
																		TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
																	},
																},
																ir.Field{
																	Name:   "GCData",
																	Offset: 0x20,
																	Type: &ir.PointerType{
																		TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
																		Pointee:          &ir.BaseType{},
																	},
																},
																ir.Field{
																	Name:   "Str",
																	Offset: 0x28,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
																	},
																},
																ir.Field{
																	Name:   "PtrToThis",
																	Offset: 0x2c,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
																	},
																},
															},
														},
													},
													ir.Field{
														Name:   "PkgPath",
														Offset: 0x30,
														Type: &ir.StructureType{
															TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
															Fields: []ir.Field{
																ir.Field{
																	Name:   "Bytes",
																	Offset: 0x0,
																	Type: &ir.PointerType{
																		TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
																		Pointee:          &ir.BaseType{},
																	},
																},
															},
														},
													},
													ir.Field{
														Name:   "Methods",
														Offset: 0x38,
														Type: &ir.GoSliceHeaderType{
															StructureType: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "array",
																		Offset: 0x0,
																		Type: &ir.PointerType{
																			TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "*internal/abi.Imethod", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
																			Pointee:          nil,
																		},
																	},
																	ir.Field{
																		Name:   "len",
																		Offset: 0x8,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
																		},
																	},
																	ir.Field{
																		Name:   "cap",
																		Offset: 0x10,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
																		},
																	},
																},
															},
															Data: &ir.GoSliceDataType{
																TypeCommon: ir.TypeCommon{ID: 0x19, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
																Element: &ir.StructureType{
																	TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
																	Fields: []ir.Field{
																		ir.Field{
																			Name:   "Name",
																			Offset: 0x0,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Typ",
																			Offset: 0x4,
																			Type:   nil,
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "Type",
										Offset: 0x8,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*internal/abi.Type", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Size_",
														Offset: 0x0,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
														},
													},
													ir.Field{
														Name:   "PtrBytes",
														Offset: 0x8,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
														},
													},
													ir.Field{
														Name:   "Hash",
														Offset: 0x10,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "TFlag",
														Offset: 0x14,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Align_",
														Offset: 0x15,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "FieldAlign_",
														Offset: 0x16,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Kind_",
														Offset: 0x17,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Equal",
														Offset: 0x18,
														Type: &ir.GoSubroutineType{
															TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
														},
													},
													ir.Field{
														Name:   "GCData",
														Offset: 0x20,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
															Pointee:          &ir.BaseType{},
														},
													},
													ir.Field{
														Name:   "Str",
														Offset: 0x28,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
														},
													},
													ir.Field{
														Name:   "PtrToThis",
														Offset: 0x2c,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
														},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "Hash",
										Offset: 0x10,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
										},
									},
									ir.Field{
										Name:   "Fun",
										Offset: 0x18,
										Type: &ir.ArrayType{
											TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "[1]uintptr", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
											Count:            0x1,
											HasCount:         true,
											Element: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
											},
										},
									},
								},
							},
						},
					},
					ir.Field{
						Name:   "data",
						Offset: 0x8,
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "unsafe.Pointer", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
							Pointee:          nil,
						},
					},
				},
			},
			0x3: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "*internal/abi.ITab", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "internal/abi.ITab", ByteSize: 0x20},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "Inter",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "Type",
											Offset: 0x0,
											Type: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Size_",
														Offset: 0x0,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "PtrBytes",
														Offset: 0x8,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "Hash",
														Offset: 0x10,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "TFlag",
														Offset: 0x14,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Align_",
														Offset: 0x15,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "FieldAlign_",
														Offset: 0x16,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Kind_",
														Offset: 0x17,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Equal",
														Offset: 0x18,
														Type: &ir.GoSubroutineType{
															TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
														},
													},
													ir.Field{
														Name:   "GCData",
														Offset: 0x20,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
															Pointee:          &ir.BaseType{},
														},
													},
													ir.Field{
														Name:   "Str",
														Offset: 0x28,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
														},
													},
													ir.Field{
														Name:   "PtrToThis",
														Offset: 0x2c,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "PkgPath",
											Offset: 0x30,
											Type: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Bytes",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
															Pointee:          &ir.BaseType{},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "Methods",
											Offset: 0x38,
											Type: &ir.GoSliceHeaderType{
												StructureType: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
													Fields: []ir.Field{
														ir.Field{
															Name:   "array",
															Offset: 0x0,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "*internal/abi.Imethod", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
																Pointee: &ir.StructureType{
																	TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
																	Fields: []ir.Field{
																		ir.Field{
																			Name:   "Name",
																			Offset: 0x0,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Typ",
																			Offset: 0x4,
																			Type:   nil,
																		},
																	},
																},
															},
														},
														ir.Field{
															Name:   "len",
															Offset: 0x8,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
															},
														},
														ir.Field{
															Name:   "cap",
															Offset: 0x10,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
															},
														},
													},
												},
												Data: &ir.GoSliceDataType{
													TypeCommon: ir.TypeCommon{ID: 0x19, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
													Element: &ir.StructureType{
														TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
														Fields: []ir.Field{
															ir.Field{
																Name:   "Name",
																Offset: 0x0,
																Type:   &ir.BaseType{},
															},
															ir.Field{
																Name:   "Typ",
																Offset: 0x4,
																Type:   &ir.BaseType{},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "Type",
							Offset: 0x8,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*internal/abi.Type", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "Size_",
											Offset: 0x0,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
											},
										},
										ir.Field{
											Name:   "PtrBytes",
											Offset: 0x8,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
											},
										},
										ir.Field{
											Name:   "Hash",
											Offset: 0x10,
											Type:   &ir.BaseType{},
										},
										ir.Field{
											Name:   "TFlag",
											Offset: 0x14,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "Align_",
											Offset: 0x15,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "FieldAlign_",
											Offset: 0x16,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "Kind_",
											Offset: 0x17,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "Equal",
											Offset: 0x18,
											Type: &ir.GoSubroutineType{
												TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
											},
										},
										ir.Field{
											Name:   "GCData",
											Offset: 0x20,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
												Pointee:          &ir.BaseType{},
											},
										},
										ir.Field{
											Name:   "Str",
											Offset: 0x28,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
											},
										},
										ir.Field{
											Name:   "PtrToThis",
											Offset: 0x2c,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
											},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "Hash",
							Offset: 0x10,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
							},
						},
						ir.Field{
							Name:   "Fun",
							Offset: 0x18,
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "[1]uintptr", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
								Count:            0x1,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
								},
							},
						},
					},
				},
			},
			0x4: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "internal/abi.ITab", ByteSize: 0x20},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "Inter",
						Offset: 0x0,
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
							Pointee: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "Type",
										Offset: 0x0,
										Type: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
											Fields: []ir.Field{
												ir.Field{
													Name:   "Size_",
													Offset: 0x0,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "PtrBytes",
													Offset: 0x8,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "Hash",
													Offset: 0x10,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "TFlag",
													Offset: 0x14,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
													},
												},
												ir.Field{
													Name:   "Align_",
													Offset: 0x15,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
													},
												},
												ir.Field{
													Name:   "FieldAlign_",
													Offset: 0x16,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
													},
												},
												ir.Field{
													Name:   "Kind_",
													Offset: 0x17,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
													},
												},
												ir.Field{
													Name:   "Equal",
													Offset: 0x18,
													Type: &ir.GoSubroutineType{
														TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
													},
												},
												ir.Field{
													Name:   "GCData",
													Offset: 0x20,
													Type: &ir.PointerType{
														TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
														Pointee:          &ir.BaseType{},
													},
												},
												ir.Field{
													Name:   "Str",
													Offset: 0x28,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
													},
												},
												ir.Field{
													Name:   "PtrToThis",
													Offset: 0x2c,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "PkgPath",
										Offset: 0x30,
										Type: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
											Fields: []ir.Field{
												ir.Field{
													Name:   "Bytes",
													Offset: 0x0,
													Type: &ir.PointerType{
														TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
														Pointee:          &ir.BaseType{},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "Methods",
										Offset: 0x38,
										Type: &ir.GoSliceHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
												Fields: []ir.Field{
													ir.Field{
														Name:   "array",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "*internal/abi.Imethod", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
															Pointee: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Name",
																		Offset: 0x0,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "Typ",
																		Offset: 0x4,
																		Type:   &ir.BaseType{},
																	},
																},
															},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
														},
													},
													ir.Field{
														Name:   "cap",
														Offset: 0x10,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
														},
													},
												},
											},
											Data: &ir.GoSliceDataType{
												TypeCommon: ir.TypeCommon{ID: 0x19, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
												Element: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
													Fields: []ir.Field{
														ir.Field{
															Name:   "Name",
															Offset: 0x0,
															Type:   &ir.BaseType{},
														},
														ir.Field{
															Name:   "Typ",
															Offset: 0x4,
															Type:   &ir.BaseType{},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
					ir.Field{
						Name:   "Type",
						Offset: 0x8,
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*internal/abi.Type", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
							Pointee: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "Size_",
										Offset: 0x0,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
										},
									},
									ir.Field{
										Name:   "PtrBytes",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
										},
									},
									ir.Field{
										Name:   "Hash",
										Offset: 0x10,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "TFlag",
										Offset: 0x14,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Align_",
										Offset: 0x15,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "FieldAlign_",
										Offset: 0x16,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Kind_",
										Offset: 0x17,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Equal",
										Offset: 0x18,
										Type: &ir.GoSubroutineType{
											TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
										},
									},
									ir.Field{
										Name:   "GCData",
										Offset: 0x20,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
											Pointee:          &ir.BaseType{},
										},
									},
									ir.Field{
										Name:   "Str",
										Offset: 0x28,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
										},
									},
									ir.Field{
										Name:   "PtrToThis",
										Offset: 0x2c,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
										},
									},
								},
							},
						},
					},
					ir.Field{
						Name:   "Hash",
						Offset: 0x10,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
						},
					},
					ir.Field{
						Name:   "Fun",
						Offset: 0x18,
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "[1]uintptr", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
							Count:            0x1,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
							},
						},
					},
				},
			},
			0x5: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "Type",
							Offset: 0x0,
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "Size_",
										Offset: 0x0,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "PtrBytes",
										Offset: 0x8,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "Hash",
										Offset: 0x10,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "TFlag",
										Offset: 0x14,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Align_",
										Offset: 0x15,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "FieldAlign_",
										Offset: 0x16,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Kind_",
										Offset: 0x17,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Equal",
										Offset: 0x18,
										Type: &ir.GoSubroutineType{
											TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
										},
									},
									ir.Field{
										Name:   "GCData",
										Offset: 0x20,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
											Pointee:          &ir.BaseType{},
										},
									},
									ir.Field{
										Name:   "Str",
										Offset: 0x28,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
										},
									},
									ir.Field{
										Name:   "PtrToThis",
										Offset: 0x2c,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "PkgPath",
							Offset: 0x30,
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "Bytes",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
											Pointee:          &ir.BaseType{},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "Methods",
							Offset: 0x38,
							Type: &ir.GoSliceHeaderType{
								StructureType: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
									Fields: []ir.Field{
										ir.Field{
											Name:   "array",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "*internal/abi.Imethod", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
												Pointee: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
													Fields: []ir.Field{
														ir.Field{
															Name:   "Name",
															Offset: 0x0,
															Type:   &ir.BaseType{},
														},
														ir.Field{
															Name:   "Typ",
															Offset: 0x4,
															Type:   &ir.BaseType{},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "len",
											Offset: 0x8,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
											},
										},
										ir.Field{
											Name:   "cap",
											Offset: 0x10,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
											},
										},
									},
								},
								Data: &ir.GoSliceDataType{
									TypeCommon: ir.TypeCommon{ID: 0x19, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
									Element: &ir.StructureType{
										TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
										Fields: []ir.Field{
											ir.Field{
												Name:   "Name",
												Offset: 0x0,
												Type:   &ir.BaseType{},
											},
											ir.Field{
												Name:   "Typ",
												Offset: 0x4,
												Type:   &ir.BaseType{},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			0x6: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "Type",
						Offset: 0x0,
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
							Fields: []ir.Field{
								ir.Field{
									Name:   "Size_",
									Offset: 0x0,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
									},
								},
								ir.Field{
									Name:   "PtrBytes",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
									},
								},
								ir.Field{
									Name:   "Hash",
									Offset: 0x10,
									Type:   &ir.BaseType{},
								},
								ir.Field{
									Name:   "TFlag",
									Offset: 0x14,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
									},
								},
								ir.Field{
									Name:   "Align_",
									Offset: 0x15,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
									},
								},
								ir.Field{
									Name:   "FieldAlign_",
									Offset: 0x16,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
									},
								},
								ir.Field{
									Name:   "Kind_",
									Offset: 0x17,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
									},
								},
								ir.Field{
									Name:   "Equal",
									Offset: 0x18,
									Type: &ir.GoSubroutineType{
										TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
									},
								},
								ir.Field{
									Name:   "GCData",
									Offset: 0x20,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
										Pointee:          &ir.BaseType{},
									},
								},
								ir.Field{
									Name:   "Str",
									Offset: 0x28,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
									},
								},
								ir.Field{
									Name:   "PtrToThis",
									Offset: 0x2c,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
									},
								},
							},
						},
					},
					ir.Field{
						Name:   "PkgPath",
						Offset: 0x30,
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
							Fields: []ir.Field{
								ir.Field{
									Name:   "Bytes",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
										Pointee:          &ir.BaseType{},
									},
								},
							},
						},
					},
					ir.Field{
						Name:   "Methods",
						Offset: 0x38,
						Type: &ir.GoSliceHeaderType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
								Fields: []ir.Field{
									ir.Field{
										Name:   "array",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "*internal/abi.Imethod", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Name",
														Offset: 0x0,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "Typ",
														Offset: 0x4,
														Type:   &ir.BaseType{},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
									ir.Field{
										Name:   "cap",
										Offset: 0x10,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Data: &ir.GoSliceDataType{
								TypeCommon: ir.TypeCommon{ID: 0x19, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
								Element: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "Name",
											Offset: 0x0,
											Type:   &ir.BaseType{},
										},
										ir.Field{
											Name:   "Typ",
											Offset: 0x4,
											Type:   &ir.BaseType{},
										},
									},
								},
							},
						},
					},
				},
			},
			0x7: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "internal/abi.Type", ByteSize: 0x30},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "Size_",
						Offset: 0x0,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
						},
					},
					ir.Field{
						Name:   "PtrBytes",
						Offset: 0x8,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
						},
					},
					ir.Field{
						Name:   "Hash",
						Offset: 0x10,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
						},
					},
					ir.Field{
						Name:   "TFlag",
						Offset: 0x14,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
						},
					},
					ir.Field{
						Name:   "Align_",
						Offset: 0x15,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
						},
					},
					ir.Field{
						Name:   "FieldAlign_",
						Offset: 0x16,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
						},
					},
					ir.Field{
						Name:   "Kind_",
						Offset: 0x17,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
						},
					},
					ir.Field{
						Name:   "Equal",
						Offset: 0x18,
						Type: &ir.GoSubroutineType{
							TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
						},
					},
					ir.Field{
						Name:   "GCData",
						Offset: 0x20,
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
							Pointee:          &ir.BaseType{},
						},
					},
					ir.Field{
						Name:   "Str",
						Offset: 0x28,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
						},
					},
					ir.Field{
						Name:   "PtrToThis",
						Offset: 0x2c,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
						},
					},
				},
			},
			0x8: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "uintptr", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
			},
			0x9: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "uint32", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
			},
			0xa: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "internal/abi.TFlag", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
			},
			0xb: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0xc: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.Kind", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
			},
			0xd: &ir.GoSubroutineType{
				TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
			},
			0xe: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee:          &ir.BaseType{},
			},
			0xf: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.NameOff", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
			},
			0x10: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "internal/abi.TypeOff", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
			},
			0x11: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "internal/abi.Name", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "Bytes",
						Offset: 0x0,
						Type:   &ir.PointerType{},
					},
				},
			},
			0x12: &ir.GoSliceHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
					Fields: []ir.Field{
						ir.Field{
							Name:   "array",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "*internal/abi.Imethod", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "Name",
											Offset: 0x0,
											Type:   &ir.BaseType{},
										},
										ir.Field{
											Name:   "Typ",
											Offset: 0x4,
											Type:   &ir.BaseType{},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
						ir.Field{
							Name:   "cap",
							Offset: 0x10,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x19, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
					Element: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
						Fields: []ir.Field{
							ir.Field{
								Name:   "Name",
								Offset: 0x0,
								Type:   &ir.BaseType{},
							},
							ir.Field{
								Name:   "Typ",
								Offset: 0x4,
								Type:   &ir.BaseType{},
							},
						},
					},
				},
			},
			0x13: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "*internal/abi.Imethod", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "Name",
							Offset: 0x0,
							Type:   &ir.BaseType{},
						},
						ir.Field{
							Name:   "Typ",
							Offset: 0x4,
							Type:   &ir.BaseType{},
						},
					},
				},
			},
			0x14: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "internal/abi.Imethod", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "Name",
						Offset: 0x0,
						Type:   &ir.BaseType{},
					},
					ir.Field{
						Name:   "Typ",
						Offset: 0x4,
						Type:   &ir.BaseType{},
					},
				},
			},
			0x15: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x16: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "*internal/abi.Type", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
				Pointee:          &ir.StructureType{},
			},
			0x17: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "[1]uintptr", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
				Count:            0x1,
				HasCount:         true,
				Element:          &ir.BaseType{},
			},
			0x18: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "unsafe.Pointer", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
				Pointee:          nil,
			},
			0x19: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x19, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
				Element:    &ir.StructureType{},
			},
			0x1a: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x1a, Name: "*[]internal/abi.Imethod.array", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoSliceDataType{},
			},
			0x1b: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x1b, Name: "ProbeEvent", ByteSize: 0x1},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "e",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.GoInterfaceType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x0,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x1b,
	}
	main_test_error_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1a, 0x59, 0xe6, 0x6d, 0x45, 0x68, 0xa3, 0x60, 0x6f, 0xf5, 0x60, 0xce, 0x43, 0x6b, 0x0, 0x0, 0x50, 0x85, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xbc, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1b, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_big_struct
	main_test_big_struct_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_big_struct",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_big_struct",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d8140, 0x3d8160},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "b",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.bigStruct", ByteSize: 0x30},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "x",
										Offset: 0x0,
										Type: &ir.GoSliceHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "[]*string", ByteSize: 0x18},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x38fc0, GoKind: 0x17},
												Fields: []ir.Field{
													ir.Field{
														Name:   "array",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "**string", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{},
															Pointee: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*string", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
																Pointee: &ir.GoStringHeaderType{
																	StructureType: nil,
																	Data:          nil,
																},
															},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "int", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
														},
													},
													ir.Field{
														Name:   "cap",
														Offset: 0x10,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "int", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
														},
													},
												},
											},
											Data: &ir.GoSliceDataType{
												TypeCommon: ir.TypeCommon{ID: 0x1e, Name: "[]*string.array", ByteSize: 0x0},
												Element: &ir.PointerType{
													TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*string", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
													Pointee: &ir.GoStringHeaderType{
														StructureType: &ir.StructureType{
															TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "string", ByteSize: 0x10},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
															Fields: []ir.Field{
																ir.Field{
																	Name:   "str",
																	Offset: 0x0,
																	Type:   nil,
																},
																ir.Field{
																	Name:   "len",
																	Offset: 0x8,
																	Type:   nil,
																},
															},
														},
														Data: &ir.GoStringDataType{
															TypeCommon: ir.TypeCommon{ID: 0x20, Name: "string.str", ByteSize: 0x0},
														},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "z",
										Offset: 0x18,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
									ir.Field{
										Name:   "writer",
										Offset: 0x20,
										Type: &ir.GoInterfaceType{
											TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "io.Writer", ByteSize: 0x0},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x6d340, GoKind: 0x14},
											UnderlyingStructure: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "runtime.iface", ByteSize: 0x10},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "tab",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "*internal/abi.ITab", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
															Pointee: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.ITab", ByteSize: 0x20},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Inter",
																		Offset: 0x0,
																		Type: &ir.PointerType{
																			TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
																			Pointee:          nil,
																		},
																	},
																	ir.Field{
																		Name:   "Type",
																		Offset: 0x8,
																		Type: &ir.PointerType{
																			TypeCommon:       ir.TypeCommon{ID: 0x1b, Name: "*internal/abi.Type", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
																			Pointee:          nil,
																		},
																	},
																	ir.Field{
																		Name:   "Hash",
																		Offset: 0x10,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "uint32", ByteSize: 0x4},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
																		},
																	},
																	ir.Field{
																		Name:   "Fun",
																		Offset: 0x18,
																		Type: &ir.ArrayType{
																			TypeCommon:       ir.TypeCommon{ID: 0x1c, Name: "[1]uintptr", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
																			Count:            0x1,
																			HasCount:         true,
																			Element:          nil,
																		},
																	},
																},
															},
														},
													},
													ir.Field{
														Name:   "data",
														Offset: 0x8,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "unsafe.Pointer", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
															Pointee:          nil,
														},
													},
												},
											},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8140, 0x3d8160},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 3},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 4},
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 5},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x24, Name: "ProbeEvent", ByteSize: 0x31},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "b",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x30,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d8140},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_big_struct",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d8140, 0x3d8160},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "b",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.bigStruct", ByteSize: 0x30},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
							Fields: []ir.Field{
								ir.Field{
									Name:   "x",
									Offset: 0x0,
									Type: &ir.GoSliceHeaderType{
										StructureType: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "[]*string", ByteSize: 0x18},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x38fc0, GoKind: 0x17},
											Fields: []ir.Field{
												ir.Field{
													Name:   "array",
													Offset: 0x0,
													Type: &ir.PointerType{
														TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "**string", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{},
														Pointee: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*string", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
															Pointee: &ir.GoStringHeaderType{
																StructureType: &ir.StructureType{
																	TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "string", ByteSize: 0x10},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
																	Fields: []ir.Field{
																		ir.Field{
																			Name:   "str",
																			Offset: 0x0,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "len",
																			Offset: 0x8,
																			Type:   nil,
																		},
																	},
																},
																Data: &ir.GoStringDataType{
																	TypeCommon: ir.TypeCommon{ID: 0x20, Name: "string.str", ByteSize: 0x0},
																},
															},
														},
													},
												},
												ir.Field{
													Name:   "len",
													Offset: 0x8,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "cap",
													Offset: 0x10,
													Type:   &ir.BaseType{},
												},
											},
										},
										Data: &ir.GoSliceDataType{
											TypeCommon: ir.TypeCommon{ID: 0x1e, Name: "[]*string.array", ByteSize: 0x0},
											Element: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*string", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
												Pointee: &ir.GoStringHeaderType{
													StructureType: &ir.StructureType{
														TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "string", ByteSize: 0x10},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
														Fields: []ir.Field{
															ir.Field{
																Name:   "str",
																Offset: 0x0,
																Type: &ir.PointerType{
																	TypeCommon:       ir.TypeCommon{ID: 0x21, Name: "*string.str", ByteSize: 0x8},
																	GoTypeAttributes: ir.GoTypeAttributes{},
																	Pointee:          nil,
																},
															},
															ir.Field{
																Name:   "len",
																Offset: 0x8,
																Type:   &ir.BaseType{},
															},
														},
													},
													Data: &ir.GoStringDataType{
														TypeCommon: ir.TypeCommon{ID: 0x20, Name: "string.str", ByteSize: 0x0},
													},
												},
											},
										},
									},
								},
								ir.Field{
									Name:   "z",
									Offset: 0x18,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
								ir.Field{
									Name:   "writer",
									Offset: 0x20,
									Type: &ir.GoInterfaceType{
										TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "io.Writer", ByteSize: 0x0},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x6d340, GoKind: 0x14},
										UnderlyingStructure: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "runtime.iface", ByteSize: 0x10},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
											Fields: []ir.Field{
												ir.Field{
													Name:   "tab",
													Offset: 0x0,
													Type: &ir.PointerType{
														TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "*internal/abi.ITab", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
														Pointee: &ir.StructureType{
															TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.ITab", ByteSize: 0x20},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
															Fields: []ir.Field{
																ir.Field{
																	Name:   "Inter",
																	Offset: 0x0,
																	Type: &ir.PointerType{
																		TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
																		Pointee: &ir.StructureType{
																			TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
																			Fields: []ir.Field{
																				ir.Field{
																					Name:   "Type",
																					Offset: 0x0,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "PkgPath",
																					Offset: 0x30,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Methods",
																					Offset: 0x38,
																					Type:   nil,
																				},
																			},
																		},
																	},
																},
																ir.Field{
																	Name:   "Type",
																	Offset: 0x8,
																	Type: &ir.PointerType{
																		TypeCommon:       ir.TypeCommon{ID: 0x1b, Name: "*internal/abi.Type", ByteSize: 0x8},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
																		Pointee: &ir.StructureType{
																			TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.Type", ByteSize: 0x30},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
																			Fields: []ir.Field{
																				ir.Field{
																					Name:   "Size_",
																					Offset: 0x0,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "PtrBytes",
																					Offset: 0x8,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Hash",
																					Offset: 0x10,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "TFlag",
																					Offset: 0x14,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Align_",
																					Offset: 0x15,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "FieldAlign_",
																					Offset: 0x16,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Kind_",
																					Offset: 0x17,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Equal",
																					Offset: 0x18,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "GCData",
																					Offset: 0x20,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Str",
																					Offset: 0x28,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "PtrToThis",
																					Offset: 0x2c,
																					Type:   nil,
																				},
																			},
																		},
																	},
																},
																ir.Field{
																	Name:   "Hash",
																	Offset: 0x10,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "uint32", ByteSize: 0x4},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
																	},
																},
																ir.Field{
																	Name:   "Fun",
																	Offset: 0x18,
																	Type: &ir.ArrayType{
																		TypeCommon:       ir.TypeCommon{ID: 0x1c, Name: "[1]uintptr", ByteSize: 0x8},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
																		Count:            0x1,
																		HasCount:         true,
																		Element: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
																		},
																	},
																},
															},
														},
													},
												},
												ir.Field{
													Name:   "data",
													Offset: 0x8,
													Type: &ir.PointerType{
														TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "unsafe.Pointer", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
														Pointee:          nil,
													},
												},
											},
										},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8140, 0x3d8160},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 1},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 2},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 3},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 4},
									locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 5},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.bigStruct", ByteSize: 0x30},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "x",
						Offset: 0x0,
						Type: &ir.GoSliceHeaderType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "[]*string", ByteSize: 0x18},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x38fc0, GoKind: 0x17},
								Fields: []ir.Field{
									ir.Field{
										Name:   "array",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "**string", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*string", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
												Pointee: &ir.GoStringHeaderType{
													StructureType: &ir.StructureType{
														TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "string", ByteSize: 0x10},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
														Fields: []ir.Field{
															ir.Field{
																Name:   "str",
																Offset: 0x0,
																Type: &ir.PointerType{
																	TypeCommon:       ir.TypeCommon{ID: 0x21, Name: "*string.str", ByteSize: 0x8},
																	GoTypeAttributes: ir.GoTypeAttributes{},
																	Pointee:          nil,
																},
															},
															ir.Field{
																Name:   "len",
																Offset: 0x8,
																Type:   &ir.BaseType{},
															},
														},
													},
													Data: &ir.GoStringDataType{
														TypeCommon: ir.TypeCommon{ID: 0x20, Name: "string.str", ByteSize: 0x0},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "cap",
										Offset: 0x10,
										Type:   &ir.BaseType{},
									},
								},
							},
							Data: &ir.GoSliceDataType{
								TypeCommon: ir.TypeCommon{ID: 0x1e, Name: "[]*string.array", ByteSize: 0x0},
								Element: &ir.PointerType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*string", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
									Pointee: &ir.GoStringHeaderType{
										StructureType: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "string", ByteSize: 0x10},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
											Fields: []ir.Field{
												ir.Field{
													Name:   "str",
													Offset: 0x0,
													Type: &ir.PointerType{
														TypeCommon:       ir.TypeCommon{ID: 0x21, Name: "*string.str", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{},
														Pointee:          &ir.GoStringDataType{},
													},
												},
												ir.Field{
													Name:   "len",
													Offset: 0x8,
													Type:   &ir.BaseType{},
												},
											},
										},
										Data: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x20, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
							},
						},
					},
					ir.Field{
						Name:   "z",
						Offset: 0x18,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "int", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
						},
					},
					ir.Field{
						Name:   "writer",
						Offset: 0x20,
						Type: &ir.GoInterfaceType{
							TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "io.Writer", ByteSize: 0x0},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x6d340, GoKind: 0x14},
							UnderlyingStructure: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "runtime.iface", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "tab",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "*internal/abi.ITab", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.ITab", ByteSize: 0x20},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Inter",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
															Pointee: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Type",
																		Offset: 0x0,
																		Type: &ir.StructureType{
																			TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.Type", ByteSize: 0x30},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
																			Fields: []ir.Field{
																				ir.Field{
																					Name:   "Size_",
																					Offset: 0x0,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "PtrBytes",
																					Offset: 0x8,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Hash",
																					Offset: 0x10,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "TFlag",
																					Offset: 0x14,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Align_",
																					Offset: 0x15,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "FieldAlign_",
																					Offset: 0x16,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Kind_",
																					Offset: 0x17,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Equal",
																					Offset: 0x18,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "GCData",
																					Offset: 0x20,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "Str",
																					Offset: 0x28,
																					Type:   nil,
																				},
																				ir.Field{
																					Name:   "PtrToThis",
																					Offset: 0x2c,
																					Type:   nil,
																				},
																			},
																		},
																	},
																	ir.Field{
																		Name:   "PkgPath",
																		Offset: 0x30,
																		Type: &ir.StructureType{
																			TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "internal/abi.Name", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
																			Fields: []ir.Field{
																				ir.Field{
																					Name:   "Bytes",
																					Offset: 0x0,
																					Type:   nil,
																				},
																			},
																		},
																	},
																	ir.Field{
																		Name:   "Methods",
																		Offset: 0x38,
																		Type: &ir.GoSliceHeaderType{
																			StructureType: nil,
																			Data:          nil,
																		},
																	},
																},
															},
														},
													},
													ir.Field{
														Name:   "Type",
														Offset: 0x8,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x1b, Name: "*internal/abi.Type", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
															Pointee: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.Type", ByteSize: 0x30},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Size_",
																		Offset: 0x0,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
																		},
																	},
																	ir.Field{
																		Name:   "PtrBytes",
																		Offset: 0x8,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
																		},
																	},
																	ir.Field{
																		Name:   "Hash",
																		Offset: 0x10,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "TFlag",
																		Offset: 0x14,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "internal/abi.TFlag", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Align_",
																		Offset: 0x15,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "uint8", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "FieldAlign_",
																		Offset: 0x16,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "uint8", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Kind_",
																		Offset: 0x17,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "internal/abi.Kind", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Equal",
																		Offset: 0x18,
																		Type: &ir.GoSubroutineType{
																			TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
																		},
																	},
																	ir.Field{
																		Name:   "GCData",
																		Offset: 0x20,
																		Type: &ir.PointerType{
																			TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*uint8", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
																			Pointee:          nil,
																		},
																	},
																	ir.Field{
																		Name:   "Str",
																		Offset: 0x28,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "internal/abi.NameOff", ByteSize: 0x4},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
																		},
																	},
																	ir.Field{
																		Name:   "PtrToThis",
																		Offset: 0x2c,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "internal/abi.TypeOff", ByteSize: 0x4},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
																		},
																	},
																},
															},
														},
													},
													ir.Field{
														Name:   "Hash",
														Offset: 0x10,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "uint32", ByteSize: 0x4},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
														},
													},
													ir.Field{
														Name:   "Fun",
														Offset: 0x18,
														Type: &ir.ArrayType{
															TypeCommon:       ir.TypeCommon{ID: 0x1c, Name: "[1]uintptr", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
															Count:            0x1,
															HasCount:         true,
															Element: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
															},
														},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "data",
										Offset: 0x8,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "unsafe.Pointer", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
											Pointee:          nil,
										},
									},
								},
							},
						},
					},
				},
			},
			0x2: &ir.GoSliceHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "[]*string", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x38fc0, GoKind: 0x17},
					Fields: []ir.Field{
						ir.Field{
							Name:   "array",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "**string", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee: &ir.PointerType{
									TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*string", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
									Pointee: &ir.GoStringHeaderType{
										StructureType: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "string", ByteSize: 0x10},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
											Fields: []ir.Field{
												ir.Field{
													Name:   "str",
													Offset: 0x0,
													Type: &ir.PointerType{
														TypeCommon:       ir.TypeCommon{ID: 0x21, Name: "*string.str", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{},
														Pointee:          &ir.GoStringDataType{},
													},
												},
												ir.Field{
													Name:   "len",
													Offset: 0x8,
													Type:   &ir.BaseType{},
												},
											},
										},
										Data: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x20, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type:   &ir.BaseType{},
						},
						ir.Field{
							Name:   "cap",
							Offset: 0x10,
							Type:   &ir.BaseType{},
						},
					},
				},
				Data: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x1e, Name: "[]*string.array", ByteSize: 0x0},
					Element: &ir.PointerType{
						TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*string", ByteSize: 0x8},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
						Pointee: &ir.GoStringHeaderType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x21, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee:          &ir.GoStringDataType{},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type:   &ir.BaseType{},
									},
								},
							},
							Data: &ir.GoStringDataType{
								TypeCommon: ir.TypeCommon{ID: 0x20, Name: "string.str", ByteSize: 0x0},
							},
						},
					},
				},
			},
			0x3: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "**string", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee: &ir.PointerType{
					TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*string", ByteSize: 0x8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
					Pointee: &ir.GoStringHeaderType{
						StructureType: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "string", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
							Fields: []ir.Field{
								ir.Field{
									Name:   "str",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x21, Name: "*string.str", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee:          &ir.GoStringDataType{},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type:   &ir.BaseType{},
								},
							},
						},
						Data: &ir.GoStringDataType{
							TypeCommon: ir.TypeCommon{ID: 0x20, Name: "string.str", ByteSize: 0x0},
						},
					},
				},
			},
			0x4: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "*string", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e080, GoKind: 0x16},
				Pointee: &ir.GoStringHeaderType{
					StructureType: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "string", ByteSize: 0x10},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
						Fields: []ir.Field{
							ir.Field{
								Name:   "str",
								Offset: 0x0,
								Type: &ir.PointerType{
									TypeCommon:       ir.TypeCommon{ID: 0x21, Name: "*string.str", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{},
									Pointee:          &ir.GoStringDataType{},
								},
							},
							ir.Field{
								Name:   "len",
								Offset: 0x8,
								Type:   &ir.BaseType{},
							},
						},
					},
					Data: &ir.GoStringDataType{
						TypeCommon: ir.TypeCommon{ID: 0x20, Name: "string.str", ByteSize: 0x0},
					},
				},
			},
			0x5: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x5, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x21, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee:          &ir.GoStringDataType{},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type:   &ir.BaseType{},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x20, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x6: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "uint8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
				},
			},
			0x7: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x7, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x8: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x8, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x9: &ir.GoInterfaceType{
				TypeCommon:       ir.TypeCommon{ID: 0x9, Name: "io.Writer", ByteSize: 0x0},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x6d340, GoKind: 0x14},
				UnderlyingStructure: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "runtime.iface", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "tab",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "*internal/abi.ITab", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.ITab", ByteSize: 0x20},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "Inter",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
												Pointee: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
													Fields: []ir.Field{
														ir.Field{
															Name:   "Type",
															Offset: 0x0,
															Type: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.Type", ByteSize: 0x30},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Size_",
																		Offset: 0x0,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "PtrBytes",
																		Offset: 0x8,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "Hash",
																		Offset: 0x10,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "TFlag",
																		Offset: 0x14,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "internal/abi.TFlag", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Align_",
																		Offset: 0x15,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "FieldAlign_",
																		Offset: 0x16,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "Kind_",
																		Offset: 0x17,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "internal/abi.Kind", ByteSize: 0x1},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
																		},
																	},
																	ir.Field{
																		Name:   "Equal",
																		Offset: 0x18,
																		Type: &ir.GoSubroutineType{
																			TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
																		},
																	},
																	ir.Field{
																		Name:   "GCData",
																		Offset: 0x20,
																		Type:   &ir.PointerType{},
																	},
																	ir.Field{
																		Name:   "Str",
																		Offset: 0x28,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "internal/abi.NameOff", ByteSize: 0x4},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
																		},
																	},
																	ir.Field{
																		Name:   "PtrToThis",
																		Offset: 0x2c,
																		Type: &ir.BaseType{
																			TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "internal/abi.TypeOff", ByteSize: 0x4},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
																		},
																	},
																},
															},
														},
														ir.Field{
															Name:   "PkgPath",
															Offset: 0x30,
															Type: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "internal/abi.Name", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Bytes",
																		Offset: 0x0,
																		Type:   &ir.PointerType{},
																	},
																},
															},
														},
														ir.Field{
															Name:   "Methods",
															Offset: 0x38,
															Type: &ir.GoSliceHeaderType{
																StructureType: &ir.StructureType{
																	TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
																	Fields: []ir.Field{
																		ir.Field{
																			Name:   "array",
																			Offset: 0x0,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "len",
																			Offset: 0x8,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "cap",
																			Offset: 0x10,
																			Type:   nil,
																		},
																	},
																},
																Data: &ir.GoSliceDataType{
																	TypeCommon: ir.TypeCommon{ID: 0x22, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
																	Element:    nil,
																},
															},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "Type",
											Offset: 0x8,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x1b, Name: "*internal/abi.Type", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
												Pointee: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.Type", ByteSize: 0x30},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
													Fields: []ir.Field{
														ir.Field{
															Name:   "Size_",
															Offset: 0x0,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
															},
														},
														ir.Field{
															Name:   "PtrBytes",
															Offset: 0x8,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
															},
														},
														ir.Field{
															Name:   "Hash",
															Offset: 0x10,
															Type:   &ir.BaseType{},
														},
														ir.Field{
															Name:   "TFlag",
															Offset: 0x14,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "internal/abi.TFlag", ByteSize: 0x1},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
															},
														},
														ir.Field{
															Name:   "Align_",
															Offset: 0x15,
															Type:   &ir.BaseType{},
														},
														ir.Field{
															Name:   "FieldAlign_",
															Offset: 0x16,
															Type:   &ir.BaseType{},
														},
														ir.Field{
															Name:   "Kind_",
															Offset: 0x17,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "internal/abi.Kind", ByteSize: 0x1},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
															},
														},
														ir.Field{
															Name:   "Equal",
															Offset: 0x18,
															Type: &ir.GoSubroutineType{
																TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
															},
														},
														ir.Field{
															Name:   "GCData",
															Offset: 0x20,
															Type:   &ir.PointerType{},
														},
														ir.Field{
															Name:   "Str",
															Offset: 0x28,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "internal/abi.NameOff", ByteSize: 0x4},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
															},
														},
														ir.Field{
															Name:   "PtrToThis",
															Offset: 0x2c,
															Type: &ir.BaseType{
																TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "internal/abi.TypeOff", ByteSize: 0x4},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
															},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "Hash",
											Offset: 0x10,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "uint32", ByteSize: 0x4},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
											},
										},
										ir.Field{
											Name:   "Fun",
											Offset: 0x18,
											Type: &ir.ArrayType{
												TypeCommon:       ir.TypeCommon{ID: 0x1c, Name: "[1]uintptr", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
												Count:            0x1,
												HasCount:         true,
												Element: &ir.BaseType{
													TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
												},
											},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "data",
							Offset: 0x8,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "unsafe.Pointer", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
								Pointee:          nil,
							},
						},
					},
				},
			},
			0xa: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0xa, Name: "runtime.iface", ByteSize: 0x10},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "tab",
						Offset: 0x0,
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "*internal/abi.ITab", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
							Pointee: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.ITab", ByteSize: 0x20},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "Inter",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Type",
														Offset: 0x0,
														Type: &ir.StructureType{
															TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.Type", ByteSize: 0x30},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
															Fields: []ir.Field{
																ir.Field{
																	Name:   "Size_",
																	Offset: 0x0,
																	Type:   &ir.BaseType{},
																},
																ir.Field{
																	Name:   "PtrBytes",
																	Offset: 0x8,
																	Type:   &ir.BaseType{},
																},
																ir.Field{
																	Name:   "Hash",
																	Offset: 0x10,
																	Type:   &ir.BaseType{},
																},
																ir.Field{
																	Name:   "TFlag",
																	Offset: 0x14,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "internal/abi.TFlag", ByteSize: 0x1},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
																	},
																},
																ir.Field{
																	Name:   "Align_",
																	Offset: 0x15,
																	Type:   &ir.BaseType{},
																},
																ir.Field{
																	Name:   "FieldAlign_",
																	Offset: 0x16,
																	Type:   &ir.BaseType{},
																},
																ir.Field{
																	Name:   "Kind_",
																	Offset: 0x17,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "internal/abi.Kind", ByteSize: 0x1},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
																	},
																},
																ir.Field{
																	Name:   "Equal",
																	Offset: 0x18,
																	Type: &ir.GoSubroutineType{
																		TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
																	},
																},
																ir.Field{
																	Name:   "GCData",
																	Offset: 0x20,
																	Type:   &ir.PointerType{},
																},
																ir.Field{
																	Name:   "Str",
																	Offset: 0x28,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "internal/abi.NameOff", ByteSize: 0x4},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
																	},
																},
																ir.Field{
																	Name:   "PtrToThis",
																	Offset: 0x2c,
																	Type: &ir.BaseType{
																		TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "internal/abi.TypeOff", ByteSize: 0x4},
																		GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
																	},
																},
															},
														},
													},
													ir.Field{
														Name:   "PkgPath",
														Offset: 0x30,
														Type: &ir.StructureType{
															TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "internal/abi.Name", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
															Fields: []ir.Field{
																ir.Field{
																	Name:   "Bytes",
																	Offset: 0x0,
																	Type:   &ir.PointerType{},
																},
															},
														},
													},
													ir.Field{
														Name:   "Methods",
														Offset: 0x38,
														Type: &ir.GoSliceHeaderType{
															StructureType: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "array",
																		Offset: 0x0,
																		Type: &ir.PointerType{
																			TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "*internal/abi.Imethod", ByteSize: 0x8},
																			GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
																			Pointee:          nil,
																		},
																	},
																	ir.Field{
																		Name:   "len",
																		Offset: 0x8,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "cap",
																		Offset: 0x10,
																		Type:   &ir.BaseType{},
																	},
																},
															},
															Data: &ir.GoSliceDataType{
																TypeCommon: ir.TypeCommon{ID: 0x22, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
																Element: &ir.StructureType{
																	TypeCommon:       ir.TypeCommon{ID: 0x1a, Name: "internal/abi.Imethod", ByteSize: 0x8},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
																	Fields: []ir.Field{
																		ir.Field{
																			Name:   "Name",
																			Offset: 0x0,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Typ",
																			Offset: 0x4,
																			Type:   nil,
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "Type",
										Offset: 0x8,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x1b, Name: "*internal/abi.Type", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.Type", ByteSize: 0x30},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Size_",
														Offset: 0x0,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
														},
													},
													ir.Field{
														Name:   "PtrBytes",
														Offset: 0x8,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
														},
													},
													ir.Field{
														Name:   "Hash",
														Offset: 0x10,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "TFlag",
														Offset: 0x14,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "internal/abi.TFlag", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Align_",
														Offset: 0x15,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "FieldAlign_",
														Offset: 0x16,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "Kind_",
														Offset: 0x17,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "internal/abi.Kind", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Equal",
														Offset: 0x18,
														Type: &ir.GoSubroutineType{
															TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
														},
													},
													ir.Field{
														Name:   "GCData",
														Offset: 0x20,
														Type:   &ir.PointerType{},
													},
													ir.Field{
														Name:   "Str",
														Offset: 0x28,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "internal/abi.NameOff", ByteSize: 0x4},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
														},
													},
													ir.Field{
														Name:   "PtrToThis",
														Offset: 0x2c,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "internal/abi.TypeOff", ByteSize: 0x4},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
														},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "Hash",
										Offset: 0x10,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "uint32", ByteSize: 0x4},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
										},
									},
									ir.Field{
										Name:   "Fun",
										Offset: 0x18,
										Type: &ir.ArrayType{
											TypeCommon:       ir.TypeCommon{ID: 0x1c, Name: "[1]uintptr", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
											Count:            0x1,
											HasCount:         true,
											Element: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
											},
										},
									},
								},
							},
						},
					},
					ir.Field{
						Name:   "data",
						Offset: 0x8,
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "unsafe.Pointer", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
							Pointee:          nil,
						},
					},
				},
			},
			0xb: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0xb, Name: "*internal/abi.ITab", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30640, GoKind: 0x16},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.ITab", ByteSize: 0x20},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "Inter",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "Type",
											Offset: 0x0,
											Type: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.Type", ByteSize: 0x30},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Size_",
														Offset: 0x0,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "PtrBytes",
														Offset: 0x8,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "Hash",
														Offset: 0x10,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "TFlag",
														Offset: 0x14,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "internal/abi.TFlag", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Align_",
														Offset: 0x15,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "FieldAlign_",
														Offset: 0x16,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "Kind_",
														Offset: 0x17,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "internal/abi.Kind", ByteSize: 0x1},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
														},
													},
													ir.Field{
														Name:   "Equal",
														Offset: 0x18,
														Type: &ir.GoSubroutineType{
															TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
														},
													},
													ir.Field{
														Name:   "GCData",
														Offset: 0x20,
														Type:   &ir.PointerType{},
													},
													ir.Field{
														Name:   "Str",
														Offset: 0x28,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "internal/abi.NameOff", ByteSize: 0x4},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
														},
													},
													ir.Field{
														Name:   "PtrToThis",
														Offset: 0x2c,
														Type: &ir.BaseType{
															TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "internal/abi.TypeOff", ByteSize: 0x4},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "PkgPath",
											Offset: 0x30,
											Type: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "internal/abi.Name", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Bytes",
														Offset: 0x0,
														Type:   &ir.PointerType{},
													},
												},
											},
										},
										ir.Field{
											Name:   "Methods",
											Offset: 0x38,
											Type: &ir.GoSliceHeaderType{
												StructureType: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
													Fields: []ir.Field{
														ir.Field{
															Name:   "array",
															Offset: 0x0,
															Type: &ir.PointerType{
																TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "*internal/abi.Imethod", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
																Pointee: &ir.StructureType{
																	TypeCommon:       ir.TypeCommon{ID: 0x1a, Name: "internal/abi.Imethod", ByteSize: 0x8},
																	GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
																	Fields: []ir.Field{
																		ir.Field{
																			Name:   "Name",
																			Offset: 0x0,
																			Type:   nil,
																		},
																		ir.Field{
																			Name:   "Typ",
																			Offset: 0x4,
																			Type:   nil,
																		},
																	},
																},
															},
														},
														ir.Field{
															Name:   "len",
															Offset: 0x8,
															Type:   &ir.BaseType{},
														},
														ir.Field{
															Name:   "cap",
															Offset: 0x10,
															Type:   &ir.BaseType{},
														},
													},
												},
												Data: &ir.GoSliceDataType{
													TypeCommon: ir.TypeCommon{ID: 0x22, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
													Element: &ir.StructureType{
														TypeCommon:       ir.TypeCommon{ID: 0x1a, Name: "internal/abi.Imethod", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
														Fields: []ir.Field{
															ir.Field{
																Name:   "Name",
																Offset: 0x0,
																Type:   &ir.BaseType{},
															},
															ir.Field{
																Name:   "Typ",
																Offset: 0x4,
																Type:   &ir.BaseType{},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "Type",
							Offset: 0x8,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x1b, Name: "*internal/abi.Type", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.Type", ByteSize: 0x30},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "Size_",
											Offset: 0x0,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
											},
										},
										ir.Field{
											Name:   "PtrBytes",
											Offset: 0x8,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
											},
										},
										ir.Field{
											Name:   "Hash",
											Offset: 0x10,
											Type:   &ir.BaseType{},
										},
										ir.Field{
											Name:   "TFlag",
											Offset: 0x14,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "internal/abi.TFlag", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "Align_",
											Offset: 0x15,
											Type:   &ir.BaseType{},
										},
										ir.Field{
											Name:   "FieldAlign_",
											Offset: 0x16,
											Type:   &ir.BaseType{},
										},
										ir.Field{
											Name:   "Kind_",
											Offset: 0x17,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "internal/abi.Kind", ByteSize: 0x1},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
											},
										},
										ir.Field{
											Name:   "Equal",
											Offset: 0x18,
											Type: &ir.GoSubroutineType{
												TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
											},
										},
										ir.Field{
											Name:   "GCData",
											Offset: 0x20,
											Type:   &ir.PointerType{},
										},
										ir.Field{
											Name:   "Str",
											Offset: 0x28,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "internal/abi.NameOff", ByteSize: 0x4},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
											},
										},
										ir.Field{
											Name:   "PtrToThis",
											Offset: 0x2c,
											Type: &ir.BaseType{
												TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "internal/abi.TypeOff", ByteSize: 0x4},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
											},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "Hash",
							Offset: 0x10,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "uint32", ByteSize: 0x4},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
							},
						},
						ir.Field{
							Name:   "Fun",
							Offset: 0x18,
							Type: &ir.ArrayType{
								TypeCommon:       ir.TypeCommon{ID: 0x1c, Name: "[1]uintptr", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
								Count:            0x1,
								HasCount:         true,
								Element: &ir.BaseType{
									TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
								},
							},
						},
					},
				},
			},
			0xc: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0xc, Name: "internal/abi.ITab", ByteSize: 0x20},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xc30c0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "Inter",
						Offset: 0x0,
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
							Pointee: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "Type",
										Offset: 0x0,
										Type: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.Type", ByteSize: 0x30},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
											Fields: []ir.Field{
												ir.Field{
													Name:   "Size_",
													Offset: 0x0,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "PtrBytes",
													Offset: 0x8,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "Hash",
													Offset: 0x10,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "TFlag",
													Offset: 0x14,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "internal/abi.TFlag", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
													},
												},
												ir.Field{
													Name:   "Align_",
													Offset: 0x15,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "FieldAlign_",
													Offset: 0x16,
													Type:   &ir.BaseType{},
												},
												ir.Field{
													Name:   "Kind_",
													Offset: 0x17,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "internal/abi.Kind", ByteSize: 0x1},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
													},
												},
												ir.Field{
													Name:   "Equal",
													Offset: 0x18,
													Type: &ir.GoSubroutineType{
														TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
													},
												},
												ir.Field{
													Name:   "GCData",
													Offset: 0x20,
													Type:   &ir.PointerType{},
												},
												ir.Field{
													Name:   "Str",
													Offset: 0x28,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "internal/abi.NameOff", ByteSize: 0x4},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
													},
												},
												ir.Field{
													Name:   "PtrToThis",
													Offset: 0x2c,
													Type: &ir.BaseType{
														TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "internal/abi.TypeOff", ByteSize: 0x4},
														GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "PkgPath",
										Offset: 0x30,
										Type: &ir.StructureType{
											TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "internal/abi.Name", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
											Fields: []ir.Field{
												ir.Field{
													Name:   "Bytes",
													Offset: 0x0,
													Type:   &ir.PointerType{},
												},
											},
										},
									},
									ir.Field{
										Name:   "Methods",
										Offset: 0x38,
										Type: &ir.GoSliceHeaderType{
											StructureType: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
												Fields: []ir.Field{
													ir.Field{
														Name:   "array",
														Offset: 0x0,
														Type: &ir.PointerType{
															TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "*internal/abi.Imethod", ByteSize: 0x8},
															GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
															Pointee: &ir.StructureType{
																TypeCommon:       ir.TypeCommon{ID: 0x1a, Name: "internal/abi.Imethod", ByteSize: 0x8},
																GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
																Fields: []ir.Field{
																	ir.Field{
																		Name:   "Name",
																		Offset: 0x0,
																		Type:   &ir.BaseType{},
																	},
																	ir.Field{
																		Name:   "Typ",
																		Offset: 0x4,
																		Type:   &ir.BaseType{},
																	},
																},
															},
														},
													},
													ir.Field{
														Name:   "len",
														Offset: 0x8,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "cap",
														Offset: 0x10,
														Type:   &ir.BaseType{},
													},
												},
											},
											Data: &ir.GoSliceDataType{
												TypeCommon: ir.TypeCommon{ID: 0x22, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
												Element: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x1a, Name: "internal/abi.Imethod", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
													Fields: []ir.Field{
														ir.Field{
															Name:   "Name",
															Offset: 0x0,
															Type:   &ir.BaseType{},
														},
														ir.Field{
															Name:   "Typ",
															Offset: 0x4,
															Type:   &ir.BaseType{},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
					ir.Field{
						Name:   "Type",
						Offset: 0x8,
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x1b, Name: "*internal/abi.Type", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
							Pointee: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.Type", ByteSize: 0x30},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "Size_",
										Offset: 0x0,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
										},
									},
									ir.Field{
										Name:   "PtrBytes",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
										},
									},
									ir.Field{
										Name:   "Hash",
										Offset: 0x10,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "TFlag",
										Offset: 0x14,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "internal/abi.TFlag", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Align_",
										Offset: 0x15,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "FieldAlign_",
										Offset: 0x16,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "Kind_",
										Offset: 0x17,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "internal/abi.Kind", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Equal",
										Offset: 0x18,
										Type: &ir.GoSubroutineType{
											TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
										},
									},
									ir.Field{
										Name:   "GCData",
										Offset: 0x20,
										Type:   &ir.PointerType{},
									},
									ir.Field{
										Name:   "Str",
										Offset: 0x28,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "internal/abi.NameOff", ByteSize: 0x4},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
										},
									},
									ir.Field{
										Name:   "PtrToThis",
										Offset: 0x2c,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "internal/abi.TypeOff", ByteSize: 0x4},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
										},
									},
								},
							},
						},
					},
					ir.Field{
						Name:   "Hash",
						Offset: 0x10,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "uint32", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
						},
					},
					ir.Field{
						Name:   "Fun",
						Offset: 0x18,
						Type: &ir.ArrayType{
							TypeCommon:       ir.TypeCommon{ID: 0x1c, Name: "[1]uintptr", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
							Count:            0x1,
							HasCount:         true,
							Element: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
							},
						},
					},
				},
			},
			0xd: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0xd, Name: "*internal/abi.InterfaceType", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3940, GoKind: 0x16},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "Type",
							Offset: 0x0,
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.Type", ByteSize: 0x30},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "Size_",
										Offset: 0x0,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "PtrBytes",
										Offset: 0x8,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "Hash",
										Offset: 0x10,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "TFlag",
										Offset: 0x14,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "internal/abi.TFlag", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Align_",
										Offset: 0x15,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "FieldAlign_",
										Offset: 0x16,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "Kind_",
										Offset: 0x17,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "internal/abi.Kind", ByteSize: 0x1},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
										},
									},
									ir.Field{
										Name:   "Equal",
										Offset: 0x18,
										Type: &ir.GoSubroutineType{
											TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
										},
									},
									ir.Field{
										Name:   "GCData",
										Offset: 0x20,
										Type:   &ir.PointerType{},
									},
									ir.Field{
										Name:   "Str",
										Offset: 0x28,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "internal/abi.NameOff", ByteSize: 0x4},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
										},
									},
									ir.Field{
										Name:   "PtrToThis",
										Offset: 0x2c,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "internal/abi.TypeOff", ByteSize: 0x4},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "PkgPath",
							Offset: 0x30,
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "internal/abi.Name", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "Bytes",
										Offset: 0x0,
										Type:   &ir.PointerType{},
									},
								},
							},
						},
						ir.Field{
							Name:   "Methods",
							Offset: 0x38,
							Type: &ir.GoSliceHeaderType{
								StructureType: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
									Fields: []ir.Field{
										ir.Field{
											Name:   "array",
											Offset: 0x0,
											Type: &ir.PointerType{
												TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "*internal/abi.Imethod", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
												Pointee: &ir.StructureType{
													TypeCommon:       ir.TypeCommon{ID: 0x1a, Name: "internal/abi.Imethod", ByteSize: 0x8},
													GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
													Fields: []ir.Field{
														ir.Field{
															Name:   "Name",
															Offset: 0x0,
															Type:   &ir.BaseType{},
														},
														ir.Field{
															Name:   "Typ",
															Offset: 0x4,
															Type:   &ir.BaseType{},
														},
													},
												},
											},
										},
										ir.Field{
											Name:   "len",
											Offset: 0x8,
											Type:   &ir.BaseType{},
										},
										ir.Field{
											Name:   "cap",
											Offset: 0x10,
											Type:   &ir.BaseType{},
										},
									},
								},
								Data: &ir.GoSliceDataType{
									TypeCommon: ir.TypeCommon{ID: 0x22, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
									Element: &ir.StructureType{
										TypeCommon:       ir.TypeCommon{ID: 0x1a, Name: "internal/abi.Imethod", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
										Fields: []ir.Field{
											ir.Field{
												Name:   "Name",
												Offset: 0x0,
												Type:   &ir.BaseType{},
											},
											ir.Field{
												Name:   "Typ",
												Offset: 0x4,
												Type:   &ir.BaseType{},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			0xe: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0xe, Name: "internal/abi.InterfaceType", ByteSize: 0x50},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xb2bc0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "Type",
						Offset: 0x0,
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.Type", ByteSize: 0x30},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
							Fields: []ir.Field{
								ir.Field{
									Name:   "Size_",
									Offset: 0x0,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
									},
								},
								ir.Field{
									Name:   "PtrBytes",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
									},
								},
								ir.Field{
									Name:   "Hash",
									Offset: 0x10,
									Type:   &ir.BaseType{},
								},
								ir.Field{
									Name:   "TFlag",
									Offset: 0x14,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "internal/abi.TFlag", ByteSize: 0x1},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
									},
								},
								ir.Field{
									Name:   "Align_",
									Offset: 0x15,
									Type:   &ir.BaseType{},
								},
								ir.Field{
									Name:   "FieldAlign_",
									Offset: 0x16,
									Type:   &ir.BaseType{},
								},
								ir.Field{
									Name:   "Kind_",
									Offset: 0x17,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "internal/abi.Kind", ByteSize: 0x1},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
									},
								},
								ir.Field{
									Name:   "Equal",
									Offset: 0x18,
									Type: &ir.GoSubroutineType{
										TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
									},
								},
								ir.Field{
									Name:   "GCData",
									Offset: 0x20,
									Type:   &ir.PointerType{},
								},
								ir.Field{
									Name:   "Str",
									Offset: 0x28,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "internal/abi.NameOff", ByteSize: 0x4},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
									},
								},
								ir.Field{
									Name:   "PtrToThis",
									Offset: 0x2c,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "internal/abi.TypeOff", ByteSize: 0x4},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
									},
								},
							},
						},
					},
					ir.Field{
						Name:   "PkgPath",
						Offset: 0x30,
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "internal/abi.Name", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
							Fields: []ir.Field{
								ir.Field{
									Name:   "Bytes",
									Offset: 0x0,
									Type:   &ir.PointerType{},
								},
							},
						},
					},
					ir.Field{
						Name:   "Methods",
						Offset: 0x38,
						Type: &ir.GoSliceHeaderType{
							StructureType: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
								Fields: []ir.Field{
									ir.Field{
										Name:   "array",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "*internal/abi.Imethod", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
											Pointee: &ir.StructureType{
												TypeCommon:       ir.TypeCommon{ID: 0x1a, Name: "internal/abi.Imethod", ByteSize: 0x8},
												GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
												Fields: []ir.Field{
													ir.Field{
														Name:   "Name",
														Offset: 0x0,
														Type:   &ir.BaseType{},
													},
													ir.Field{
														Name:   "Typ",
														Offset: 0x4,
														Type:   &ir.BaseType{},
													},
												},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type:   &ir.BaseType{},
									},
									ir.Field{
										Name:   "cap",
										Offset: 0x10,
										Type:   &ir.BaseType{},
									},
								},
							},
							Data: &ir.GoSliceDataType{
								TypeCommon: ir.TypeCommon{ID: 0x22, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
								Element: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x1a, Name: "internal/abi.Imethod", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "Name",
											Offset: 0x0,
											Type:   &ir.BaseType{},
										},
										ir.Field{
											Name:   "Typ",
											Offset: 0x4,
											Type:   &ir.BaseType{},
										},
									},
								},
							},
						},
					},
				},
			},
			0xf: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0xf, Name: "internal/abi.Type", ByteSize: 0x30},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xecc80, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "Size_",
						Offset: 0x0,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
						},
					},
					ir.Field{
						Name:   "PtrBytes",
						Offset: 0x8,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
						},
					},
					ir.Field{
						Name:   "Hash",
						Offset: 0x10,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "uint32", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
						},
					},
					ir.Field{
						Name:   "TFlag",
						Offset: 0x14,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "internal/abi.TFlag", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
						},
					},
					ir.Field{
						Name:   "Align_",
						Offset: 0x15,
						Type:   &ir.BaseType{},
					},
					ir.Field{
						Name:   "FieldAlign_",
						Offset: 0x16,
						Type:   &ir.BaseType{},
					},
					ir.Field{
						Name:   "Kind_",
						Offset: 0x17,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "internal/abi.Kind", ByteSize: 0x1},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
						},
					},
					ir.Field{
						Name:   "Equal",
						Offset: 0x18,
						Type: &ir.GoSubroutineType{
							TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
						},
					},
					ir.Field{
						Name:   "GCData",
						Offset: 0x20,
						Type:   &ir.PointerType{},
					},
					ir.Field{
						Name:   "Str",
						Offset: 0x28,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "internal/abi.NameOff", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
						},
					},
					ir.Field{
						Name:   "PtrToThis",
						Offset: 0x2c,
						Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "internal/abi.TypeOff", ByteSize: 0x4},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
						},
					},
				},
			},
			0x10: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x10, Name: "uintptr", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x456c0, GoKind: 0xc},
			},
			0x11: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x11, Name: "uint32", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45580, GoKind: 0xa},
			},
			0x12: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x12, Name: "internal/abi.TFlag", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45dc0, GoKind: 0x8},
			},
			0x13: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x13, Name: "internal/abi.Kind", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5c380, GoKind: 0x8},
			},
			0x14: &ir.GoSubroutineType{
				TypeCommon:       ir.TypeCommon{ID: 0x14, Name: "func(unsafe.Pointer, unsafe.Pointer) bool", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x5ddc0, GoKind: 0x13},
			},
			0x15: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x15, Name: "internal/abi.NameOff", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e00, GoKind: 0x5},
			},
			0x16: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x16, Name: "internal/abi.TypeOff", ByteSize: 0x4},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45e40, GoKind: 0x5},
			},
			0x17: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x17, Name: "internal/abi.Name", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xdcc40, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "Bytes",
						Offset: 0x0,
						Type:   &ir.PointerType{},
					},
				},
			},
			0x18: &ir.GoSliceHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x18, Name: "[]internal/abi.Imethod", ByteSize: 0x18},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x3bb60, GoKind: 0x17},
					Fields: []ir.Field{
						ir.Field{
							Name:   "array",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "*internal/abi.Imethod", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
								Pointee: &ir.StructureType{
									TypeCommon:       ir.TypeCommon{ID: 0x1a, Name: "internal/abi.Imethod", ByteSize: 0x8},
									GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
									Fields: []ir.Field{
										ir.Field{
											Name:   "Name",
											Offset: 0x0,
											Type:   &ir.BaseType{},
										},
										ir.Field{
											Name:   "Typ",
											Offset: 0x4,
											Type:   &ir.BaseType{},
										},
									},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type:   &ir.BaseType{},
						},
						ir.Field{
							Name:   "cap",
							Offset: 0x10,
							Type:   &ir.BaseType{},
						},
					},
				},
				Data: &ir.GoSliceDataType{
					TypeCommon: ir.TypeCommon{ID: 0x22, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
					Element: &ir.StructureType{
						TypeCommon:       ir.TypeCommon{ID: 0x1a, Name: "internal/abi.Imethod", ByteSize: 0x8},
						GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
						Fields: []ir.Field{
							ir.Field{
								Name:   "Name",
								Offset: 0x0,
								Type:   &ir.BaseType{},
							},
							ir.Field{
								Name:   "Typ",
								Offset: 0x4,
								Type:   &ir.BaseType{},
							},
						},
					},
				},
			},
			0x19: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x19, Name: "*internal/abi.Imethod", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x30600, GoKind: 0x16},
				Pointee: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x1a, Name: "internal/abi.Imethod", ByteSize: 0x8},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
					Fields: []ir.Field{
						ir.Field{
							Name:   "Name",
							Offset: 0x0,
							Type:   &ir.BaseType{},
						},
						ir.Field{
							Name:   "Typ",
							Offset: 0x4,
							Type:   &ir.BaseType{},
						},
					},
				},
			},
			0x1a: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x1a, Name: "internal/abi.Imethod", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xa08a0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "Name",
						Offset: 0x0,
						Type:   &ir.BaseType{},
					},
					ir.Field{
						Name:   "Typ",
						Offset: 0x4,
						Type:   &ir.BaseType{},
					},
				},
			},
			0x1b: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x1b, Name: "*internal/abi.Type", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0xf3b00, GoKind: 0x16},
				Pointee:          &ir.StructureType{},
			},
			0x1c: &ir.ArrayType{
				TypeCommon:       ir.TypeCommon{ID: 0x1c, Name: "[1]uintptr", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x4d180, GoKind: 0x11},
				Count:            0x1,
				HasCount:         true,
				Element:          &ir.BaseType{},
			},
			0x1d: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x1d, Name: "unsafe.Pointer", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45840, GoKind: 0x0},
				Pointee:          nil,
			},
			0x1e: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x1e, Name: "[]*string.array", ByteSize: 0x0},
				Element:    &ir.PointerType{},
			},
			0x1f: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x1f, Name: "*[]*string.array", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoSliceDataType{},
			},
			0x20: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x20, Name: "string.str", ByteSize: 0x0},
			},
			0x21: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x21, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0x22: &ir.GoSliceDataType{
				TypeCommon: ir.TypeCommon{ID: 0x22, Name: "[]internal/abi.Imethod.array", ByteSize: 0x0},
				Element:    &ir.StructureType{},
			},
			0x23: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x23, Name: "*[]internal/abi.Imethod.array", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoSliceDataType{},
			},
			0x24: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x24, Name: "ProbeEvent", ByteSize: 0x31},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "b",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x30,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x24,
	}
	main_test_big_struct_bytes := []byte{0x88, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x9b, 0xa0, 0x6d, 0x76, 0x16, 0x77, 0x4c, 0x43, 0xf6, 0x41, 0x51, 0xf2, 0x43, 0x6b, 0x0, 0x0, 0x40, 0x81, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xb0, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x24, 0x0, 0x0, 0x0, 0x31, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x40, 0x87, 0x9c, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_circular_type
	main_test_circular_type_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_circular_type",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_circular_type",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d8160, 0x3d8170},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "x",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.circularReferenceType", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
								Fields: []ir.Field{
									ir.Field{
										Name:   "t",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.circularReferenceType", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x16},
											Pointee:          &ir.StructureType{},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8160, 0x3d8170},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 8, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x9},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "x",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.StructureType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x8,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d8160},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_circular_type",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d8160, 0x3d8170},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "x",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.circularReferenceType", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
							Fields: []ir.Field{
								ir.Field{
									Name:   "t",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.circularReferenceType", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x16},
										Pointee:          &ir.StructureType{},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8160, 0x3d8170},
								Pieces: []locexpr.LocationPiece{
									{Size: 8, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.StructureType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "main.circularReferenceType", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x19},
				Fields: []ir.Field{
					ir.Field{
						Name:   "t",
						Offset: 0x0,
						Type: &ir.PointerType{
							TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.circularReferenceType", ByteSize: 0x8},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x16},
							Pointee:          &ir.StructureType{},
						},
					},
				},
			},
			0x2: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*main.circularReferenceType", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x0, GoKind: 0x16},
				Pointee:          &ir.StructureType{},
			},
			0x3: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x3, Name: "ProbeEvent", ByteSize: 0x9},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "x",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.StructureType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x8,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x3,
	}
	main_test_circular_type_bytes := []byte{0x78, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x46, 0x6a, 0xce, 0xa9, 0x3d, 0xee, 0xf2, 0x63, 0x86, 0x1e, 0x40, 0x15, 0x44, 0x6b, 0x0, 0x0, 0x60, 0x81, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xb0, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x0, 0x9, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0xfd, 0x18, 0x0, 0x40, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x8, 0x0, 0x0, 0x0, 0x0, 0xfd, 0x18, 0x0, 0x40, 0x0, 0x0, 0x0, 0x0, 0xfd, 0x18, 0x0, 0x40, 0x0, 0x0, 0x0}

	// IR for main_stack_A
	main_stack_A_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "stack_A",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.stack_A",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9a70, 0x3d9ab0},
					},
					InlinePCRanges: nil,
					Variables:      nil,
					Lines:          nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x1, Name: "ProbeEvent", ByteSize: 0x0},
							PresenseBitsetSize: 0x0,
							Expressions:        nil,
						},
						InjectionPCs: []uint64{0x3d9a7c},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.stack_A",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9a70, 0x3d9ab0},
				},
				InlinePCRanges: nil,
				Variables:      nil,
				Lines:          nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x1, Name: "ProbeEvent", ByteSize: 0x0},
				PresenseBitsetSize: 0x0,
				Expressions:        nil,
			},
		},
		MaxTypeID: 0x1,
	}
	main_stack_A_bytes := []byte{0x50, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x44, 0xa1, 0xb4, 0x5e, 0x81, 0x83, 0x58, 0x8b, 0xac, 0x8a, 0x42, 0x3a, 0x44, 0x6b, 0x0, 0x0, 0x70, 0x9a, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa8, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_stack_B
	main_stack_B_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "stack_B",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.stack_B",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9ab0, 0x3d9af0},
					},
					InlinePCRanges: nil,
					Variables:      nil,
					Lines:          nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x1, Name: "ProbeEvent", ByteSize: 0x0},
							PresenseBitsetSize: 0x0,
							Expressions:        nil,
						},
						InjectionPCs: []uint64{0x3d9abc},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.stack_B",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9ab0, 0x3d9af0},
				},
				InlinePCRanges: nil,
				Variables:      nil,
				Lines:          nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x1, Name: "ProbeEvent", ByteSize: 0x0},
				PresenseBitsetSize: 0x0,
				Expressions:        nil,
			},
		},
		MaxTypeID: 0x1,
	}
	main_stack_B_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x28, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7f, 0xd8, 0x9c, 0xbb, 0xf2, 0xb6, 0x7f, 0x92, 0x13, 0x85, 0x63, 0x5a, 0x44, 0x6b, 0x0, 0x0, 0xb0, 0x9a, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2c, 0x9c, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa8, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_stack_C
	main_stack_C_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "stack_C",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.stack_C",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d9af0, 0x3d9b60},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "~r0",
							Type: &ir.StructureType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "string", ByteSize: 0x10},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
								Fields: []ir.Field{
									ir.Field{
										Name:   "str",
										Offset: 0x0,
										Type: &ir.PointerType{
											TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{},
											Pointee: &ir.GoStringDataType{
												TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
											},
										},
									},
									ir.Field{
										Name:   "len",
										Offset: 0x8,
										Type: &ir.BaseType{
											TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
											GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
										},
									},
								},
							},
							Locations: []ir.Location{
								ir.Location{
									Range:  ir.PCRange{0x3d9af0, 0x3d9b60},
									Pieces: []locexpr.LocationPiece{},
								},
							},
							IsParameter: true,
							IsReturn:    true,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x7, Name: "ProbeEvent", ByteSize: 0x0},
							PresenseBitsetSize: 0x0,
							Expressions:        nil,
						},
						InjectionPCs: []uint64{0x3d9afc},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.stack_C",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d9af0, 0x3d9b60},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "~r0",
						Type: &ir.StructureType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "string", ByteSize: 0x10},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
							Fields: []ir.Field{
								ir.Field{
									Name:   "str",
									Offset: 0x0,
									Type: &ir.PointerType{
										TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{},
										Pointee: &ir.GoStringDataType{
											TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
										},
									},
								},
								ir.Field{
									Name:   "len",
									Offset: 0x8,
									Type: &ir.BaseType{
										TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
										GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
									},
								},
							},
						},
						Locations: []ir.Location{
							{
								Range:  ir.PCRange{0x3d9af0, 0x3d9b60},
								Pieces: []locexpr.LocationPiece{},
							},
						},
						IsParameter: true,
						IsReturn:    true,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.GoStringHeaderType{
				StructureType: &ir.StructureType{
					TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "string", ByteSize: 0x10},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45400, GoKind: 0x18},
					Fields: []ir.Field{
						ir.Field{
							Name:   "str",
							Offset: 0x0,
							Type: &ir.PointerType{
								TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{},
								Pointee: &ir.GoStringDataType{
									TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
								},
							},
						},
						ir.Field{
							Name:   "len",
							Offset: 0x8,
							Type: &ir.BaseType{
								TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
							},
						},
					},
				},
				Data: &ir.GoStringDataType{
					TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
				},
			},
			0x2: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x2, Name: "*uint8", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x2e100, GoKind: 0x16},
				Pointee: &ir.BaseType{
					TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint8", ByteSize: 0x1},
					GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
				},
			},
			0x3: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x3, Name: "uint8", ByteSize: 0x1},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45480, GoKind: 0x8},
			},
			0x4: &ir.BaseType{
				TypeCommon:       ir.TypeCommon{ID: 0x4, Name: "int", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x45640, GoKind: 0x2},
			},
			0x5: &ir.GoStringDataType{
				TypeCommon: ir.TypeCommon{ID: 0x5, Name: "string.str", ByteSize: 0x0},
			},
			0x6: &ir.PointerType{
				TypeCommon:       ir.TypeCommon{ID: 0x6, Name: "*string.str", ByteSize: 0x8},
				GoTypeAttributes: ir.GoTypeAttributes{},
				Pointee:          &ir.GoStringDataType{},
			},
			0x7: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x7, Name: "ProbeEvent", ByteSize: 0x0},
				PresenseBitsetSize: 0x0,
				Expressions:        nil,
			},
		},
		MaxTypeID: 0x7,
	}
	main_stack_C_bytes := []byte{0x60, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x30, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x36, 0x43, 0x8b, 0x3c, 0x74, 0x9c, 0x7f, 0x59, 0xc3, 0xf4, 0xfe, 0x7a, 0x44, 0x6b, 0x0, 0x0, 0xf0, 0x9a, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x8c, 0x9a, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2c, 0x9c, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa8, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// IR for main_test_channel
	main_test_channel_ir := &ir.Program{
		ID: 0x1,
		Probes: []*ir.Probe{
			&ir.Probe{
				ID:      "test_channel",
				Kind:    0x1,
				Version: 0,
				Tags:    nil,
				Subprogram: &ir.Subprogram{
					ID:   0x1,
					Name: "main.test_channel",
					OutOfLinePCRanges: []ir.PCRange{
						ir.PCRange{0x3d8cf0, 0x3d8d00},
					},
					InlinePCRanges: nil,
					Variables: []*ir.Variable{
						&ir.Variable{
							Name: "c",
							Type: &ir.GoChannelType{
								TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "chan bool", ByteSize: 0x0},
								GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x47080, GoKind: 0x12},
							},
							Locations: []ir.Location{
								ir.Location{
									Range: ir.PCRange{0x3d8cf0, 0x3d8d00},
									Pieces: []locexpr.LocationPiece{
										locexpr.LocationPiece{Size: 0, InReg: true, StackOffset: 0, Register: 0},
									},
								},
							},
							IsParameter: true,
							IsReturn:    false,
						},
					},
					Lines: nil,
				},
				Events: []*ir.Event{
					&ir.Event{
						ID: 0x1,
						Type: &ir.EventRootType{
							TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x1},
							PresenseBitsetSize: 0x1,
							Expressions: []*ir.RootExpression{
								&ir.RootExpression{
									Name:   "c",
									Offset: 0x1,
									Expression: ir.Expression{
										Type: &ir.GoChannelType{},
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: &ir.Variable{},
												Offset:   0x0,
												ByteSize: 0x0,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{0x3d8cf0},
						Condition:    (*ir.Expression)(nil),
					},
				},
				Snapshot: false,
			},
		},
		Subprograms: []*ir.Subprogram{
			&ir.Subprogram{
				ID:   0x1,
				Name: "main.test_channel",
				OutOfLinePCRanges: []ir.PCRange{
					ir.PCRange{0x3d8cf0, 0x3d8d00},
				},
				InlinePCRanges: nil,
				Variables: []*ir.Variable{
					&ir.Variable{
						Name: "c",
						Type: &ir.GoChannelType{
							TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "chan bool", ByteSize: 0x0},
							GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x47080, GoKind: 0x12},
						},
						Locations: []ir.Location{
							{
								Range: ir.PCRange{0x3d8cf0, 0x3d8d00},
								Pieces: []locexpr.LocationPiece{
									{Size: 0, InReg: true, StackOffset: 0, Register: 0},
								},
							},
						},
						IsParameter: true,
						IsReturn:    false,
					},
				},
				Lines: nil,
			},
		},
		Types: map[ir.TypeID]ir.Type{
			0x1: &ir.GoChannelType{
				TypeCommon:       ir.TypeCommon{ID: 0x1, Name: "chan bool", ByteSize: 0x0},
				GoTypeAttributes: ir.GoTypeAttributes{GoRuntimeType: 0x47080, GoKind: 0x12},
			},
			0x2: &ir.EventRootType{
				TypeCommon:         ir.TypeCommon{ID: 0x2, Name: "ProbeEvent", ByteSize: 0x1},
				PresenseBitsetSize: 0x1,
				Expressions: []*ir.RootExpression{
					&ir.RootExpression{
						Name:   "c",
						Offset: 0x1,
						Expression: ir.Expression{
							Type: &ir.GoChannelType{},
							Operations: []ir.Op{
								&ir.LocationOp{
									Variable: &ir.Variable{},
									Offset:   0x0,
									ByteSize: 0x0,
								},
							},
						},
					},
				},
			},
		},
		MaxTypeID: 0x2,
	}
	main_test_channel_bytes := []byte{0x58, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xe7, 0x85, 0x82, 0x4a, 0xc8, 0xcb, 0x2f, 0x44, 0x9d, 0xe2, 0x2d, 0x9b, 0x44, 0x6b, 0x0, 0x0, 0xf0, 0x8c, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0x8c, 0x94, 0x3d, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd4, 0xed, 0x4, 0x0, 0x0, 0x0, 0x0, 0x0, 0x74, 0x96, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}

	// Use variables to avoid "declared and not used" errors
	_ = main_test_single_byte_ir
	_ = main_test_single_rune_ir
	_ = main_test_single_bool_ir
	_ = main_test_single_int_ir
	_ = main_test_single_int8_ir
	_ = main_test_single_int16_ir
	_ = main_test_single_int32_ir
	_ = main_test_single_int64_ir
	_ = main_test_single_uint_ir
	_ = main_test_single_uint8_ir
	_ = main_test_single_uint16_ir
	_ = main_test_single_uint32_ir
	_ = main_test_single_uint64_ir
	_ = main_test_single_float32_ir
	_ = main_test_single_float64_ir
	_ = main_test_type_alias_ir
	_ = main_test_single_string_ir
	_ = main_test_three_strings_ir
	_ = main_test_three_strings_in_struct_ir
	_ = main_test_three_strings_in_struct_pointer_ir
	_ = main_test_one_string_in_struct_pointer_ir
	_ = main_test_byte_array_ir
	_ = main_test_rune_array_ir
	_ = main_test_string_array_ir
	_ = main_test_bool_array_ir
	_ = main_test_int_array_ir
	_ = main_test_int8_array_ir
	_ = main_test_int16_array_ir
	_ = main_test_int32_array_ir
	_ = main_test_int64_array_ir
	_ = main_test_uint_array_ir
	_ = main_test_uint8_array_ir
	_ = main_test_uint16_array_ir
	_ = main_test_uint32_array_ir
	_ = main_test_uint64_array_ir
	_ = main_test_array_of_arrays_ir
	_ = main_test_array_of_strings_ir
	_ = main_test_array_of_arrays_of_arrays_ir
	_ = main_test_array_of_structs_ir
	_ = main_test_uint_slice_ir
	_ = main_test_empty_slice_ir
	_ = main_test_slice_of_slices_ir
	_ = main_test_struct_slice_ir
	_ = main_test_empty_slice_of_structs_ir
	_ = main_test_nil_slice_of_structs_ir
	_ = main_test_string_slice_ir
	_ = main_test_nil_slice_with_other_params_ir
	_ = main_test_nil_slice_ir
	_ = main_test_struct_ir
	_ = main_test_empty_struct_ir
	_ = main_test_uint_pointer_ir
	_ = main_test_string_pointer_ir
	_ = main_test_nil_pointer_ir
	_ = main_test_combined_byte_ir
	_ = main_test_multiple_simple_params_ir
	_ = main_test_map_string_to_int_ir
	_ = main_test_interface_ir
	_ = main_test_error_ir
	_ = main_test_big_struct_ir
	_ = main_test_circular_type_ir
	_ = main_stack_A_ir
	_ = main_stack_B_ir
	_ = main_stack_C_ir
	_ = main_test_channel_ir

	_ = main_test_single_byte_bytes
	_ = main_test_single_rune_bytes
	_ = main_test_single_bool_bytes
	_ = main_test_single_int_bytes
	_ = main_test_single_int8_bytes
	_ = main_test_single_int16_bytes
	_ = main_test_single_int32_bytes
	_ = main_test_single_int64_bytes
	_ = main_test_single_uint_bytes
	_ = main_test_single_uint8_bytes
	_ = main_test_single_uint16_bytes
	_ = main_test_single_uint32_bytes
	_ = main_test_single_uint64_bytes
	_ = main_test_single_float32_bytes
	_ = main_test_single_float64_bytes
	_ = main_test_type_alias_bytes
	_ = main_test_single_string_bytes
	_ = main_test_three_strings_bytes
	_ = main_test_three_strings_in_struct_bytes
	_ = main_test_three_strings_in_struct_pointer_bytes
	_ = main_test_one_string_in_struct_pointer_bytes
	_ = main_test_byte_array_bytes
	_ = main_test_rune_array_bytes
	_ = main_test_string_array_bytes
	_ = main_test_bool_array_bytes
	_ = main_test_int_array_bytes
	_ = main_test_int8_array_bytes
	_ = main_test_int16_array_bytes
	_ = main_test_int32_array_bytes
	_ = main_test_int64_array_bytes
	_ = main_test_uint_array_bytes
	_ = main_test_uint8_array_bytes
	_ = main_test_uint16_array_bytes
	_ = main_test_uint32_array_bytes
	_ = main_test_uint64_array_bytes
	_ = main_test_array_of_arrays_bytes
	_ = main_test_array_of_strings_bytes
	_ = main_test_array_of_arrays_of_arrays_bytes
	_ = main_test_array_of_structs_bytes
	_ = main_test_uint_slice_bytes
	_ = main_test_empty_slice_bytes
	_ = main_test_slice_of_slices_bytes
	_ = main_test_struct_slice_bytes
	_ = main_test_empty_slice_of_structs_bytes
	_ = main_test_nil_slice_of_structs_bytes
	_ = main_test_string_slice_bytes
	_ = main_test_nil_slice_with_other_params_bytes
	_ = main_test_nil_slice_bytes
	_ = main_test_struct_bytes
	_ = main_test_empty_struct_bytes
	_ = main_test_uint_pointer_bytes
	_ = main_test_string_pointer_bytes
	_ = main_test_nil_pointer_bytes
	_ = main_test_combined_byte_bytes
	_ = main_test_multiple_simple_params_bytes
	_ = main_test_map_string_to_int_bytes
	_ = main_test_interface_bytes
	_ = main_test_error_bytes
	_ = main_test_big_struct_bytes
	_ = main_test_circular_type_bytes
	_ = main_stack_A_bytes
	_ = main_stack_B_bytes
	_ = main_stack_C_bytes
	_ = main_test_channel_bytes

}
