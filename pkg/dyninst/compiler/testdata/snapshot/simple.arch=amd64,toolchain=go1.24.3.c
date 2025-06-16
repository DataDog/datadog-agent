const uint8_t stack_machine_code[] = {
		SM_OP_ILLEGAL, 

	// 0x1: ChasePointers
		SM_OP_CHASE_POINTERS, 
		SM_OP_RETURN, 

	// 0x3: ProcessExpression[Probe[main.intArg]@0x4a804a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x19: ProcessEvent[Probe[main.intArg]@4a804a]
		SM_OP_PREPARE_EVENT_ROOT, 0x35, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x03, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.intArg]@0x4a804a.expr[0]]
		SM_OP_RETURN, 

	// 0x28: ProcessType[string]
		SM_OP_PROCESS_STRING, 0x2b, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x2e: ProcessExpression[Probe[main.stringArg]@0x4a80ca.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x03, 0x08, 0x08, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x28, 0x00, 0x00, 0x00, // ProcessType[string]
		SM_OP_RETURN, 

	// 0x50: ProcessEvent[Probe[main.stringArg]@4a80ca]
		SM_OP_PREPARE_EVENT_ROOT, 0x36, 0x00, 0x00, 0x00, 0x11, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x2e, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.stringArg]@0x4a80ca.expr[0]]
		SM_OP_RETURN, 

	// 0x5f: ProcessType[[]int]
		SM_OP_PROCESS_SLICE, 0x2d, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x69: ProcessExpression[Probe[main.intSliceArg]@0x4a814a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x03, 0x08, 0x08, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x02, 0x08, 0x10, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x5f, 0x00, 0x00, 0x00, // ProcessType[[]int]
		SM_OP_RETURN, 

	// 0x92: ProcessEvent[Probe[main.intSliceArg]@4a814a]
		SM_OP_PREPARE_EVENT_ROOT, 0x37, 0x00, 0x00, 0x00, 0x19, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x69, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.intSliceArg]@0x4a814a.expr[0]]
		SM_OP_RETURN, 

	// 0xa1: ProcessExpression[Probe[main.intArrayArg]@0x4a81ca.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_DEREFERENCE_CFA, 0x00, 0x00, 0x00, 0x00, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0xbd: ProcessEvent[Probe[main.intArrayArg]@4a81ca]
		SM_OP_PREPARE_EVENT_ROOT, 0x38, 0x00, 0x00, 0x00, 0x19, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0xa1, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.intArrayArg]@0x4a81ca.expr[0]]
		SM_OP_RETURN, 

	// 0xcc: ProcessType[[]string]
		SM_OP_PROCESS_SLICE, 0x2f, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0xd6: ProcessExpression[Probe[main.stringSliceArg]@0x4a824a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x03, 0x08, 0x08, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x02, 0x08, 0x10, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0xcc, 0x00, 0x00, 0x00, // ProcessType[[]string]
		SM_OP_RETURN, 

	// 0xff: ProcessEvent[Probe[main.stringSliceArg]@4a824a]
		SM_OP_PREPARE_EVENT_ROOT, 0x39, 0x00, 0x00, 0x00, 0x19, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0xd6, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.stringSliceArg]@0x4a824a.expr[0]]
		SM_OP_RETURN, 

	// 0x10e: ProcessType[[3]string]
		SM_OP_PROCESS_ARRAY_DATA_PREP, 0x30, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x28, 0x00, 0x00, 0x00, // ProcessType[string]
		SM_OP_PROCESS_SLICE_DATA_REPEAT, 0x10, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x11e: ProcessExpression[Probe[main.stringArrayArg]@0x4a82ca.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_DEREFERENCE_CFA, 0x00, 0x00, 0x00, 0x00, 0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x0e, 0x01, 0x00, 0x00, // ProcessType[[3]string]
		SM_OP_RETURN, 

	// 0x13f: ProcessEvent[Probe[main.stringArrayArg]@4a82ca]
		SM_OP_PREPARE_EVENT_ROOT, 0x3a, 0x00, 0x00, 0x00, 0x31, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x1e, 0x01, 0x00, 0x00, // ProcessExpression[Probe[main.stringArrayArg]@0x4a82ca.expr[0]]
		SM_OP_RETURN, 

	// 0x14e: ProcessExpression[Probe[main.mapArg]@0x4a840a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x15d: ProcessEvent[Probe[main.mapArg]@4a840a]
		SM_OP_PREPARE_EVENT_ROOT, 0x3b, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x4e, 0x01, 0x00, 0x00, // ProcessExpression[Probe[main.mapArg]@0x4a840a.expr[0]]
		SM_OP_RETURN, 

	// 0x16c: ProcessExpression[Probe[main.bigMapArg]@0x4a8473.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x17b: ProcessEvent[Probe[main.bigMapArg]@4a8473]
		SM_OP_PREPARE_EVENT_ROOT, 0x3c, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x6c, 0x01, 0x00, 0x00, // ProcessExpression[Probe[main.bigMapArg]@0x4a8473.expr[0]]
		SM_OP_RETURN, 

	// 0x18a: ProcessExpression[Probe[main.inlined]@0x4a834a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x1a0: ProcessEvent[Probe[main.inlined]@4a834a]
		SM_OP_PREPARE_EVENT_ROOT, 0x3d, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x8a, 0x01, 0x00, 0x00, // ProcessExpression[Probe[main.inlined]@0x4a834a.expr[0]]
		SM_OP_RETURN, 

	// 0x1af: ProcessExpression[Probe[main.inlined]@0x4a7da6.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_RETURN, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x1bf: ProcessEvent[Probe[main.inlined]@4a7da6]
		SM_OP_PREPARE_EVENT_ROOT, 0x3d, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0xaf, 0x01, 0x00, 0x00, // ProcessExpression[Probe[main.inlined]@0x4a7da6.expr[0]]
		SM_OP_RETURN, 

	// 0x1ce: ProcessType[*****int]
		SM_OP_PROCESS_POINTER, 0x03, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x1d4: ProcessExpression[Probe[main.PointerChainArg]@0x4a7fe6.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0xce, 0x01, 0x00, 0x00, // ProcessType[*****int]
		SM_OP_RETURN, 

	// 0x1ef: ProcessEvent[Probe[main.PointerChainArg]@4a7fe6]
		SM_OP_PREPARE_EVENT_ROOT, 0x3e, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0xd4, 0x01, 0x00, 0x00, // ProcessExpression[Probe[main.PointerChainArg]@0x4a7fe6.expr[0]]
		SM_OP_RETURN, 

	// 0x1fe: ProcessType[[]string.array]
		SM_OP_PROCESS_SLICE_DATA_PREP, 
		SM_OP_CALL, 0x28, 0x00, 0x00, 0x00, // ProcessType[string]
		SM_OP_PROCESS_SLICE_DATA_REPEAT, 0x10, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x20a: ProcessType[****int]
		SM_OP_PROCESS_POINTER, 0x04, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x210: ProcessType[***int]
		SM_OP_PROCESS_POINTER, 0x05, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x216: ProcessType[**int]
		SM_OP_PROCESS_POINTER, 0x06, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x21c: ProcessType[*int]
		SM_OP_PROCESS_POINTER, 0x01, 0x00, 0x00, 0x00, 
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
const uint64_t stack_machine_code_len = 559;
const uint32_t stack_machine_code_max_op = 13;
const uint32_t chase_pointers_entrypoint = 0x1;

const probe_params_t probe_params[] = {
	{.throttler_idx = 0, .stack_machine_pc = 0x19, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 1, .stack_machine_pc = 0x50, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 2, .stack_machine_pc = 0x92, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 3, .stack_machine_pc = 0xbd, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 4, .stack_machine_pc = 0xff, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 5, .stack_machine_pc = 0x13f, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 6, .stack_machine_pc = 0x15d, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 7, .stack_machine_pc = 0x17b, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 8, .stack_machine_pc = 0x1a0, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 8, .stack_machine_pc = 0x1bf, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 9, .stack_machine_pc = 0x1ef, .pointer_chasing_limit = 3, .frameless = false},
};
const uint32_t num_probe_params = 11;
typedef enum type {
	TYPE_NONE = 0,
	TYPE_1 = 1, // int
	TYPE_2 = 2, // *****int
	TYPE_3 = 3, // ****int
	TYPE_4 = 4, // ***int
	TYPE_5 = 5, // **int
	TYPE_6 = 6, // *int
	TYPE_7 = 7, // string
	TYPE_8 = 8, // *uint8
	TYPE_9 = 9, // uint8
	TYPE_10 = 10, // []int
	TYPE_11 = 11, // [3]int
	TYPE_12 = 12, // []string
	TYPE_13 = 13, // *string
	TYPE_14 = 14, // [3]string
	TYPE_15 = 15, // map[string]int
	TYPE_16 = 16, // *map<string,int>
	TYPE_17 = 17, // map<string,int>
	TYPE_18 = 18, // uint64
	TYPE_19 = 19, // uintptr
	TYPE_20 = 20, // **table<string,int>
	TYPE_21 = 21, // *table<string,int>
	TYPE_22 = 22, // table<string,int>
	TYPE_23 = 23, // uint16
	TYPE_24 = 24, // groupReference<string,int>
	TYPE_25 = 25, // *noalg.map.group[string]int
	TYPE_26 = 26, // noalg.map.group[string]int
	TYPE_27 = 27, // noalg.[8]struct { key string; elem int }
	TYPE_28 = 28, // noalg.struct { key string; elem int }
	TYPE_29 = 29, // map[string]main.bigStruct
	TYPE_30 = 30, // *map<string,main.bigStruct>
	TYPE_31 = 31, // map<string,main.bigStruct>
	TYPE_32 = 32, // **table<string,main.bigStruct>
	TYPE_33 = 33, // *table<string,main.bigStruct>
	TYPE_34 = 34, // table<string,main.bigStruct>
	TYPE_35 = 35, // groupReference<string,main.bigStruct>
	TYPE_36 = 36, // *noalg.map.group[string]main.bigStruct
	TYPE_37 = 37, // noalg.map.group[string]main.bigStruct
	TYPE_38 = 38, // noalg.[8]struct { key string; elem *main.bigStruct }
	TYPE_39 = 39, // noalg.struct { key string; elem *main.bigStruct }
	TYPE_40 = 40, // *main.bigStruct
	TYPE_41 = 41, // main.bigStruct
	TYPE_42 = 42, // [128]uint8
	TYPE_43 = 43, // string.str
	TYPE_44 = 44, // *string.str
	TYPE_45 = 45, // []int.array
	TYPE_46 = 46, // *[]int.array
	TYPE_47 = 47, // []string.array
	TYPE_48 = 48, // *[]string.array
	TYPE_49 = 49, // []*table<string,int>.array
	TYPE_50 = 50, // []noalg.map.group[string]int.array
	TYPE_51 = 51, // []*table<string,main.bigStruct>.array
	TYPE_52 = 52, // []noalg.map.group[string]main.bigStruct.array
	TYPE_53 = 53, // Probe[main.intArg]
	TYPE_54 = 54, // Probe[main.stringArg]
	TYPE_55 = 55, // Probe[main.intSliceArg]
	TYPE_56 = 56, // Probe[main.intArrayArg]
	TYPE_57 = 57, // Probe[main.stringSliceArg]
	TYPE_58 = 58, // Probe[main.stringArrayArg]
	TYPE_59 = 59, // Probe[main.mapArg]
	TYPE_60 = 60, // Probe[main.bigMapArg]
	TYPE_61 = 61, // Probe[main.inlined]
	TYPE_62 = 62, // Probe[main.PointerChainArg]
} type_t;

const type_info_t type_info[] = {
	/* 1: int	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 2: *****int	*/{.byte_len = 8, .enqueue_pc = 0x1ce},
	/* 3: ****int	*/{.byte_len = 8, .enqueue_pc = 0x20a},
	/* 4: ***int	*/{.byte_len = 8, .enqueue_pc = 0x210},
	/* 5: **int	*/{.byte_len = 8, .enqueue_pc = 0x216},
	/* 6: *int	*/{.byte_len = 8, .enqueue_pc = 0x21c},
	/* 7: string	*/{.byte_len = 16, .enqueue_pc = 0x28},
	/* 8: *uint8	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 9: uint8	*/{.byte_len = 1, .enqueue_pc = 0x0},
	/* 10: []int	*/{.byte_len = 24, .enqueue_pc = 0x5f},
	/* 11: [3]int	*/{.byte_len = 24, .enqueue_pc = 0x0},
	/* 12: []string	*/{.byte_len = 24, .enqueue_pc = 0xcc},
	/* 13: *string	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 14: [3]string	*/{.byte_len = 48, .enqueue_pc = 0x10e},
	/* 15: map[string]int	*/{.byte_len = 0, .enqueue_pc = 0x0},
	/* 16: *map<string,int>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 17: map<string,int>	*/{.byte_len = 48, .enqueue_pc = 0x0},
	/* 18: uint64	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 19: uintptr	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 20: **table<string,int>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 21: *table<string,int>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 22: table<string,int>	*/{.byte_len = 32, .enqueue_pc = 0x0},
	/* 23: uint16	*/{.byte_len = 2, .enqueue_pc = 0x0},
	/* 24: groupReference<string,int>	*/{.byte_len = 16, .enqueue_pc = 0x0},
	/* 25: *noalg.map.group[string]int	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 26: noalg.map.group[string]int	*/{.byte_len = 200, .enqueue_pc = 0x0},
	/* 27: noalg.[8]struct { key string; elem int }	*/{.byte_len = 192, .enqueue_pc = 0x0},
	/* 28: noalg.struct { key string; elem int }	*/{.byte_len = 24, .enqueue_pc = 0x0},
	/* 29: map[string]main.bigStruct	*/{.byte_len = 0, .enqueue_pc = 0x0},
	/* 30: *map<string,main.bigStruct>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 31: map<string,main.bigStruct>	*/{.byte_len = 48, .enqueue_pc = 0x0},
	/* 32: **table<string,main.bigStruct>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 33: *table<string,main.bigStruct>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 34: table<string,main.bigStruct>	*/{.byte_len = 32, .enqueue_pc = 0x0},
	/* 35: groupReference<string,main.bigStruct>	*/{.byte_len = 16, .enqueue_pc = 0x0},
	/* 36: *noalg.map.group[string]main.bigStruct	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 37: noalg.map.group[string]main.bigStruct	*/{.byte_len = 200, .enqueue_pc = 0x0},
	/* 38: noalg.[8]struct { key string; elem *main.bigStruct }	*/{.byte_len = 192, .enqueue_pc = 0x0},
	/* 39: noalg.struct { key string; elem *main.bigStruct }	*/{.byte_len = 24, .enqueue_pc = 0x0},
	/* 40: *main.bigStruct	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 41: main.bigStruct	*/{.byte_len = 184, .enqueue_pc = 0x0},
	/* 42: [128]uint8	*/{.byte_len = 128, .enqueue_pc = 0x0},
	/* 43: string.str	*/{.byte_len = 512, .enqueue_pc = 0x0},
	/* 44: *string.str	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 45: []int.array	*/{.byte_len = 512, .enqueue_pc = 0x0},
	/* 46: *[]int.array	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 47: []string.array	*/{.byte_len = 512, .enqueue_pc = 0x1fe},
	/* 48: *[]string.array	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 49: []*table<string,int>.array	*/{.byte_len = 2048, .enqueue_pc = 0x0},
	/* 50: []noalg.map.group[string]int.array	*/{.byte_len = 512, .enqueue_pc = 0x0},
	/* 51: []*table<string,main.bigStruct>.array	*/{.byte_len = 2048, .enqueue_pc = 0x0},
	/* 52: []noalg.map.group[string]main.bigStruct.array	*/{.byte_len = 512, .enqueue_pc = 0x0},
	/* 53: Probe[main.intArg]	*/{.byte_len = 9, .enqueue_pc = 0x0},
	/* 54: Probe[main.stringArg]	*/{.byte_len = 17, .enqueue_pc = 0x0},
	/* 55: Probe[main.intSliceArg]	*/{.byte_len = 25, .enqueue_pc = 0x0},
	/* 56: Probe[main.intArrayArg]	*/{.byte_len = 25, .enqueue_pc = 0x0},
	/* 57: Probe[main.stringSliceArg]	*/{.byte_len = 25, .enqueue_pc = 0x0},
	/* 58: Probe[main.stringArrayArg]	*/{.byte_len = 49, .enqueue_pc = 0x0},
	/* 59: Probe[main.mapArg]	*/{.byte_len = 1, .enqueue_pc = 0x0},
	/* 60: Probe[main.bigMapArg]	*/{.byte_len = 1, .enqueue_pc = 0x0},
	/* 61: Probe[main.inlined]	*/{.byte_len = 9, .enqueue_pc = 0x0},
	/* 62: Probe[main.PointerChainArg]	*/{.byte_len = 9, .enqueue_pc = 0x0},
};

const uint32_t type_ids[] = {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, };

const uint32_t num_types = 62;

const throttler_params_t throttler_params[] = {
	{.period_ns = 100000000, .budget = 500},
	{.period_ns = 100000000, .budget = 500},
	{.period_ns = 100000000, .budget = 500},
	{.period_ns = 100000000, .budget = 500},
	{.period_ns = 100000000, .budget = 500},
	{.period_ns = 100000000, .budget = 500},
	{.period_ns = 100000000, .budget = 500},
	{.period_ns = 100000000, .budget = 500},
	{.period_ns = 100000000, .budget = 500},
	{.period_ns = 100000000, .budget = 500},
};
#define NUM_THROTTLERS 10
