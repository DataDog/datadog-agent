const uint8_t stack_machine_code[] = {
		SM_OP_ILLEGAL, 

	// 0x1: ChasePointers
		SM_OP_CHASE_POINTERS, 
		SM_OP_RETURN, 

	// 0x3: ProcessExpression[Probe[main.intArg]@0x4a7f0a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x19: ProcessEvent[Probe[main.intArg]@4a7f0a]
		SM_OP_PREPARE_EVENT_ROOT, 0x31, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x03, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.intArg]@0x4a7f0a.expr[0]]
		SM_OP_RETURN, 

	// 0x28: ProcessType[string]
		SM_OP_PROCESS_STRING, 0x27, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x2e: ProcessExpression[Probe[main.stringArg]@0x4a7f8a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x03, 0x08, 0x08, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x28, 0x00, 0x00, 0x00, // ProcessType[string]
		SM_OP_RETURN, 

	// 0x50: ProcessEvent[Probe[main.stringArg]@4a7f8a]
		SM_OP_PREPARE_EVENT_ROOT, 0x32, 0x00, 0x00, 0x00, 0x11, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x2e, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.stringArg]@0x4a7f8a.expr[0]]
		SM_OP_RETURN, 

	// 0x5f: ProcessType[[]int]
		SM_OP_PROCESS_SLICE, 0x29, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x69: ProcessExpression[Probe[main.intSliceArg]@0x4a800a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x03, 0x08, 0x08, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x02, 0x08, 0x10, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x5f, 0x00, 0x00, 0x00, // ProcessType[[]int]
		SM_OP_RETURN, 

	// 0x92: ProcessEvent[Probe[main.intSliceArg]@4a800a]
		SM_OP_PREPARE_EVENT_ROOT, 0x33, 0x00, 0x00, 0x00, 0x19, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x69, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.intSliceArg]@0x4a800a.expr[0]]
		SM_OP_RETURN, 

	// 0xa1: ProcessExpression[Probe[main.intArrayArg]@0x4a808a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_DEREFERENCE_CFA, 0x00, 0x00, 0x00, 0x00, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0xbd: ProcessEvent[Probe[main.intArrayArg]@4a808a]
		SM_OP_PREPARE_EVENT_ROOT, 0x34, 0x00, 0x00, 0x00, 0x19, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0xa1, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.intArrayArg]@0x4a808a.expr[0]]
		SM_OP_RETURN, 

	// 0xcc: ProcessType[[]string]
		SM_OP_PROCESS_SLICE, 0x2b, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0xd6: ProcessExpression[Probe[main.stringSliceArg]@0x4a810a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x03, 0x08, 0x08, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x02, 0x08, 0x10, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0xcc, 0x00, 0x00, 0x00, // ProcessType[[]string]
		SM_OP_RETURN, 

	// 0xff: ProcessEvent[Probe[main.stringSliceArg]@4a810a]
		SM_OP_PREPARE_EVENT_ROOT, 0x35, 0x00, 0x00, 0x00, 0x19, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0xd6, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.stringSliceArg]@0x4a810a.expr[0]]
		SM_OP_RETURN, 

	// 0x10e: ProcessType[[3]string]
		SM_OP_PROCESS_ARRAY_DATA_PREP, 0x30, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x28, 0x00, 0x00, 0x00, // ProcessType[string]
		SM_OP_PROCESS_SLICE_DATA_REPEAT, 0x10, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x11e: ProcessExpression[Probe[main.stringArrayArg]@0x4a818a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_DEREFERENCE_CFA, 0x00, 0x00, 0x00, 0x00, 0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x0e, 0x01, 0x00, 0x00, // ProcessType[[3]string]
		SM_OP_RETURN, 

	// 0x13f: ProcessEvent[Probe[main.stringArrayArg]@4a818a]
		SM_OP_PREPARE_EVENT_ROOT, 0x36, 0x00, 0x00, 0x00, 0x31, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x1e, 0x01, 0x00, 0x00, // ProcessExpression[Probe[main.stringArrayArg]@0x4a818a.expr[0]]
		SM_OP_RETURN, 

	// 0x14e: ProcessExpression[Probe[main.mapArg]@0x4a82ca.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x15d: ProcessEvent[Probe[main.mapArg]@4a82ca]
		SM_OP_PREPARE_EVENT_ROOT, 0x37, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x4e, 0x01, 0x00, 0x00, // ProcessExpression[Probe[main.mapArg]@0x4a82ca.expr[0]]
		SM_OP_RETURN, 

	// 0x16c: ProcessExpression[Probe[main.bigMapArg]@0x4a8333.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x17b: ProcessEvent[Probe[main.bigMapArg]@4a8333]
		SM_OP_PREPARE_EVENT_ROOT, 0x38, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x6c, 0x01, 0x00, 0x00, // ProcessExpression[Probe[main.bigMapArg]@0x4a8333.expr[0]]
		SM_OP_RETURN, 

	// 0x18a: ProcessExpression[Probe[main.inlined]@0x4a820a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x1a0: ProcessEvent[Probe[main.inlined]@4a820a]
		SM_OP_PREPARE_EVENT_ROOT, 0x39, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x8a, 0x01, 0x00, 0x00, // ProcessExpression[Probe[main.inlined]@0x4a820a.expr[0]]
		SM_OP_RETURN, 

	// 0x1af: ProcessExpression[Probe[main.inlined]@0x4a7da6.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_RETURN, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x1bf: ProcessEvent[Probe[main.inlined]@4a7da6]
		SM_OP_PREPARE_EVENT_ROOT, 0x39, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0xaf, 0x01, 0x00, 0x00, // ProcessExpression[Probe[main.inlined]@0x4a7da6.expr[0]]
		SM_OP_RETURN, 

	// 0x1ce: ProcessType[[]string.array]
		SM_OP_PROCESS_SLICE_DATA_PREP, 
		SM_OP_CALL, 0x28, 0x00, 0x00, 0x00, // ProcessType[string]
		SM_OP_PROCESS_SLICE_DATA_REPEAT, 0x10, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// Extra illegal ops to simplify code bound checks
		SM_OP_ILLEGAL, 
		SM_OP_ILLEGAL, 
		SM_OP_ILLEGAL, 
		SM_OP_ILLEGAL, 
		SM_OP_ILLEGAL, 
		SM_OP_ILLEGAL, 
		SM_OP_ILLEGAL, 
		SM_OP_ILLEGAL, 
		SM_OP_ILLEGAL, 
		SM_OP_ILLEGAL, 
		SM_OP_ILLEGAL, 
		SM_OP_ILLEGAL, 
		SM_OP_ILLEGAL, 
};
const uint64_t stack_machine_code_len = 487;
const uint32_t stack_machine_code_max_op = 13;
const uint32_t chase_pointers_entrypoint = 0x1;

const probe_params_t probe_params[] = {
	{.stack_machine_pc = 0x19, .stream_id = 0, .frameless = false},
	{.stack_machine_pc = 0x50, .stream_id = 0, .frameless = false},
	{.stack_machine_pc = 0x92, .stream_id = 0, .frameless = false},
	{.stack_machine_pc = 0xbd, .stream_id = 0, .frameless = false},
	{.stack_machine_pc = 0xff, .stream_id = 0, .frameless = false},
	{.stack_machine_pc = 0x13f, .stream_id = 0, .frameless = false},
	{.stack_machine_pc = 0x15d, .stream_id = 0, .frameless = false},
	{.stack_machine_pc = 0x17b, .stream_id = 0, .frameless = false},
	{.stack_machine_pc = 0x1a0, .stream_id = 0, .frameless = false},
	{.stack_machine_pc = 0x1bf, .stream_id = 0, .frameless = false},
};
const uint32_t num_probe_params = 10;
typedef enum type {
	TYPE_NONE = 0,
	TYPE_1 = 1, // int
	TYPE_2 = 2, // string
	TYPE_3 = 3, // *uint8
	TYPE_4 = 4, // uint8
	TYPE_5 = 5, // []int
	TYPE_6 = 6, // *int
	TYPE_7 = 7, // [3]int
	TYPE_8 = 8, // []string
	TYPE_9 = 9, // *string
	TYPE_10 = 10, // [3]string
	TYPE_11 = 11, // map[string]int
	TYPE_12 = 12, // *map<string,int>
	TYPE_13 = 13, // map<string,int>
	TYPE_14 = 14, // uint64
	TYPE_15 = 15, // uintptr
	TYPE_16 = 16, // **table<string,int>
	TYPE_17 = 17, // *table<string,int>
	TYPE_18 = 18, // table<string,int>
	TYPE_19 = 19, // uint16
	TYPE_20 = 20, // groupReference<string,int>
	TYPE_21 = 21, // *noalg.map.group[string]int
	TYPE_22 = 22, // noalg.map.group[string]int
	TYPE_23 = 23, // noalg.[8]struct { key string; elem int }
	TYPE_24 = 24, // noalg.struct { key string; elem int }
	TYPE_25 = 25, // map[string]main.bigStruct
	TYPE_26 = 26, // *map<string,main.bigStruct>
	TYPE_27 = 27, // map<string,main.bigStruct>
	TYPE_28 = 28, // **table<string,main.bigStruct>
	TYPE_29 = 29, // *table<string,main.bigStruct>
	TYPE_30 = 30, // table<string,main.bigStruct>
	TYPE_31 = 31, // groupReference<string,main.bigStruct>
	TYPE_32 = 32, // *noalg.map.group[string]main.bigStruct
	TYPE_33 = 33, // noalg.map.group[string]main.bigStruct
	TYPE_34 = 34, // noalg.[8]struct { key string; elem *main.bigStruct }
	TYPE_35 = 35, // noalg.struct { key string; elem *main.bigStruct }
	TYPE_36 = 36, // *main.bigStruct
	TYPE_37 = 37, // main.bigStruct
	TYPE_38 = 38, // [128]uint8
	TYPE_39 = 39, // string.str
	TYPE_40 = 40, // *string.str
	TYPE_41 = 41, // []int.array
	TYPE_42 = 42, // *[]int.array
	TYPE_43 = 43, // []string.array
	TYPE_44 = 44, // *[]string.array
	TYPE_45 = 45, // []*table<string,int>.array
	TYPE_46 = 46, // []noalg.map.group[string]int.array
	TYPE_47 = 47, // []*table<string,main.bigStruct>.array
	TYPE_48 = 48, // []noalg.map.group[string]main.bigStruct.array
	TYPE_49 = 49, // Probe[main.intArg]
	TYPE_50 = 50, // Probe[main.stringArg]
	TYPE_51 = 51, // Probe[main.intSliceArg]
	TYPE_52 = 52, // Probe[main.intArrayArg]
	TYPE_53 = 53, // Probe[main.stringSliceArg]
	TYPE_54 = 54, // Probe[main.stringArrayArg]
	TYPE_55 = 55, // Probe[main.mapArg]
	TYPE_56 = 56, // Probe[main.bigMapArg]
	TYPE_57 = 57, // Probe[main.inlined]
} type_t;

const type_info_t type_info[] = {
	/* 1: int	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 2: string	*/{.byte_len = 16, .enqueue_pc = 0x28},
	/* 3: *uint8	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 4: uint8	*/{.byte_len = 1, .enqueue_pc = 0x0},
	/* 5: []int	*/{.byte_len = 24, .enqueue_pc = 0x5f},
	/* 6: *int	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 7: [3]int	*/{.byte_len = 24, .enqueue_pc = 0x0},
	/* 8: []string	*/{.byte_len = 24, .enqueue_pc = 0xcc},
	/* 9: *string	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 10: [3]string	*/{.byte_len = 48, .enqueue_pc = 0x10e},
	/* 11: map[string]int	*/{.byte_len = 0, .enqueue_pc = 0x0},
	/* 12: *map<string,int>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 13: map<string,int>	*/{.byte_len = 48, .enqueue_pc = 0x0},
	/* 14: uint64	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 15: uintptr	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 16: **table<string,int>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 17: *table<string,int>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 18: table<string,int>	*/{.byte_len = 32, .enqueue_pc = 0x0},
	/* 19: uint16	*/{.byte_len = 2, .enqueue_pc = 0x0},
	/* 20: groupReference<string,int>	*/{.byte_len = 16, .enqueue_pc = 0x0},
	/* 21: *noalg.map.group[string]int	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 22: noalg.map.group[string]int	*/{.byte_len = 200, .enqueue_pc = 0x0},
	/* 23: noalg.[8]struct { key string; elem int }	*/{.byte_len = 192, .enqueue_pc = 0x0},
	/* 24: noalg.struct { key string; elem int }	*/{.byte_len = 24, .enqueue_pc = 0x0},
	/* 25: map[string]main.bigStruct	*/{.byte_len = 0, .enqueue_pc = 0x0},
	/* 26: *map<string,main.bigStruct>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 27: map<string,main.bigStruct>	*/{.byte_len = 48, .enqueue_pc = 0x0},
	/* 28: **table<string,main.bigStruct>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 29: *table<string,main.bigStruct>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 30: table<string,main.bigStruct>	*/{.byte_len = 32, .enqueue_pc = 0x0},
	/* 31: groupReference<string,main.bigStruct>	*/{.byte_len = 16, .enqueue_pc = 0x0},
	/* 32: *noalg.map.group[string]main.bigStruct	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 33: noalg.map.group[string]main.bigStruct	*/{.byte_len = 200, .enqueue_pc = 0x0},
	/* 34: noalg.[8]struct { key string; elem *main.bigStruct }	*/{.byte_len = 192, .enqueue_pc = 0x0},
	/* 35: noalg.struct { key string; elem *main.bigStruct }	*/{.byte_len = 24, .enqueue_pc = 0x0},
	/* 36: *main.bigStruct	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 37: main.bigStruct	*/{.byte_len = 184, .enqueue_pc = 0x0},
	/* 38: [128]uint8	*/{.byte_len = 128, .enqueue_pc = 0x0},
	/* 39: string.str	*/{.byte_len = 512, .enqueue_pc = 0x0},
	/* 40: *string.str	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 41: []int.array	*/{.byte_len = 512, .enqueue_pc = 0x0},
	/* 42: *[]int.array	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 43: []string.array	*/{.byte_len = 512, .enqueue_pc = 0x1ce},
	/* 44: *[]string.array	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 45: []*table<string,int>.array	*/{.byte_len = 2048, .enqueue_pc = 0x0},
	/* 46: []noalg.map.group[string]int.array	*/{.byte_len = 512, .enqueue_pc = 0x0},
	/* 47: []*table<string,main.bigStruct>.array	*/{.byte_len = 2048, .enqueue_pc = 0x0},
	/* 48: []noalg.map.group[string]main.bigStruct.array	*/{.byte_len = 512, .enqueue_pc = 0x0},
	/* 49: Probe[main.intArg]	*/{.byte_len = 9, .enqueue_pc = 0x0},
	/* 50: Probe[main.stringArg]	*/{.byte_len = 17, .enqueue_pc = 0x0},
	/* 51: Probe[main.intSliceArg]	*/{.byte_len = 25, .enqueue_pc = 0x0},
	/* 52: Probe[main.intArrayArg]	*/{.byte_len = 25, .enqueue_pc = 0x0},
	/* 53: Probe[main.stringSliceArg]	*/{.byte_len = 25, .enqueue_pc = 0x0},
	/* 54: Probe[main.stringArrayArg]	*/{.byte_len = 49, .enqueue_pc = 0x0},
	/* 55: Probe[main.mapArg]	*/{.byte_len = 1, .enqueue_pc = 0x0},
	/* 56: Probe[main.bigMapArg]	*/{.byte_len = 1, .enqueue_pc = 0x0},
	/* 57: Probe[main.inlined]	*/{.byte_len = 9, .enqueue_pc = 0x0},
};

const uint32_t type_ids[] = {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, };

const uint32_t num_types = 57;

