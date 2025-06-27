const uint8_t stack_machine_code[] = {
		SM_OP_ILLEGAL, 

	// 0x1: ChasePointers
		SM_OP_CHASE_POINTERS, 
		SM_OP_RETURN, 

	// 0x3: ProcessExpression[Probe[main.intArg]@0x4a806a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x19: ProcessEvent[Probe[main.intArg]@4a806a]
		SM_OP_PREPARE_EVENT_ROOT, 0x11, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x03, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.intArg]@0x4a806a.expr[0]]
		SM_OP_RETURN, 

	// 0x28: ProcessType[string]
		SM_OP_PROCESS_STRING, 0x0b, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x2e: ProcessExpression[Probe[main.stringArg]@0x4a80ea.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x03, 0x08, 0x08, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x28, 0x00, 0x00, 0x00, // ProcessType[string]
		SM_OP_RETURN, 

	// 0x50: ProcessEvent[Probe[main.stringArg]@4a80ea]
		SM_OP_PREPARE_EVENT_ROOT, 0x12, 0x00, 0x00, 0x00, 0x11, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x2e, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.stringArg]@0x4a80ea.expr[0]]
		SM_OP_RETURN, 

	// 0x5f: ProcessType[[]int]
		SM_OP_PROCESS_SLICE, 0x0d, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x69: ProcessExpression[Probe[main.intSliceArg]@0x4a816a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x03, 0x08, 0x08, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x02, 0x08, 0x10, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x5f, 0x00, 0x00, 0x00, // ProcessType[[]int]
		SM_OP_RETURN, 

	// 0x92: ProcessEvent[Probe[main.intSliceArg]@4a816a]
		SM_OP_PREPARE_EVENT_ROOT, 0x13, 0x00, 0x00, 0x00, 0x19, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x69, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.intSliceArg]@0x4a816a.expr[0]]
		SM_OP_RETURN, 

	// 0xa1: ProcessExpression[Probe[main.intArrayArg]@0x4a81ea.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_DEREFERENCE_CFA, 0x00, 0x00, 0x00, 0x00, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0xbd: ProcessEvent[Probe[main.intArrayArg]@4a81ea]
		SM_OP_PREPARE_EVENT_ROOT, 0x14, 0x00, 0x00, 0x00, 0x19, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0xa1, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.intArrayArg]@0x4a81ea.expr[0]]
		SM_OP_RETURN, 

	// 0xcc: ProcessType[[]string]
		SM_OP_PROCESS_SLICE, 0x0f, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0xd6: ProcessExpression[Probe[main.stringSliceArg]@0x4a826a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x03, 0x08, 0x08, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x02, 0x08, 0x10, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0xcc, 0x00, 0x00, 0x00, // ProcessType[[]string]
		SM_OP_RETURN, 

	// 0xff: ProcessEvent[Probe[main.stringSliceArg]@4a826a]
		SM_OP_PREPARE_EVENT_ROOT, 0x15, 0x00, 0x00, 0x00, 0x19, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0xd6, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.stringSliceArg]@0x4a826a.expr[0]]
		SM_OP_RETURN, 

	// 0x10e: ProcessType[[3]string]
		SM_OP_PROCESS_ARRAY_DATA_PREP, 0x30, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x28, 0x00, 0x00, 0x00, // ProcessType[string]
		SM_OP_PROCESS_SLICE_DATA_REPEAT, 0x10, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x11e: ProcessExpression[Probe[main.stringArrayArg]@0x4a82ea.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_DEREFERENCE_CFA, 0x00, 0x00, 0x00, 0x00, 0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x0e, 0x01, 0x00, 0x00, // ProcessType[[3]string]
		SM_OP_RETURN, 

	// 0x13f: ProcessEvent[Probe[main.stringArrayArg]@4a82ea]
		SM_OP_PREPARE_EVENT_ROOT, 0x16, 0x00, 0x00, 0x00, 0x31, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x1e, 0x01, 0x00, 0x00, // ProcessExpression[Probe[main.stringArrayArg]@0x4a82ea.expr[0]]
		SM_OP_RETURN, 

	// 0x14e: ProcessExpression[Probe[main.stringArrayArgFrameless]@0x4a8360.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_DEREFERENCE_CFA, 0x00, 0x00, 0x00, 0x00, 0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x0e, 0x01, 0x00, 0x00, // ProcessType[[3]string]
		SM_OP_RETURN, 

	// 0x16f: ProcessEvent[Probe[main.stringArrayArgFrameless]@4a8360]
		SM_OP_PREPARE_EVENT_ROOT, 0x17, 0x00, 0x00, 0x00, 0x31, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x4e, 0x01, 0x00, 0x00, // ProcessExpression[Probe[main.stringArrayArgFrameless]@0x4a8360.expr[0]]
		SM_OP_RETURN, 

	// 0x17e: ProcessExpression[Probe[main.inlined]@0x4a838a.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x194: ProcessEvent[Probe[main.inlined]@4a838a]
		SM_OP_PREPARE_EVENT_ROOT, 0x18, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x7e, 0x01, 0x00, 0x00, // ProcessExpression[Probe[main.inlined]@0x4a838a.expr[0]]
		SM_OP_RETURN, 

	// 0x1a3: ProcessExpression[Probe[main.inlined]@0x4a7dce.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_RETURN, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x1b3: ProcessEvent[Probe[main.inlined]@4a7dce]
		SM_OP_PREPARE_EVENT_ROOT, 0x18, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0xa3, 0x01, 0x00, 0x00, // ProcessExpression[Probe[main.inlined]@0x4a7dce.expr[0]]
		SM_OP_RETURN, 

	// 0x1c2: ProcessType[[]string.array]
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
const uint64_t stack_machine_code_len = 475;
const uint32_t stack_machine_code_max_op = 13;
const uint32_t chase_pointers_entrypoint = 0x1;

const uint32_t prog_id = 1;

const probe_params_t probe_params[] = {
	{.throttler_idx = 0, .stack_machine_pc = 0x19, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 1, .stack_machine_pc = 0x50, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 2, .stack_machine_pc = 0x92, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 3, .stack_machine_pc = 0xbd, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 4, .stack_machine_pc = 0xff, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 5, .stack_machine_pc = 0x13f, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 6, .stack_machine_pc = 0x16f, .pointer_chasing_limit = 4294967295, .frameless = true},
	{.throttler_idx = 7, .stack_machine_pc = 0x194, .pointer_chasing_limit = 4294967295, .frameless = false},
	{.throttler_idx = 7, .stack_machine_pc = 0x1b3, .pointer_chasing_limit = 4294967295, .frameless = true},
};
const uint32_t num_probe_params = 9;
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
	TYPE_11 = 11, // string.str
	TYPE_12 = 12, // *string.str
	TYPE_13 = 13, // []int.array
	TYPE_14 = 14, // *[]int.array
	TYPE_15 = 15, // []string.array
	TYPE_16 = 16, // *[]string.array
	TYPE_17 = 17, // Probe[main.intArg]
	TYPE_18 = 18, // Probe[main.stringArg]
	TYPE_19 = 19, // Probe[main.intSliceArg]
	TYPE_20 = 20, // Probe[main.intArrayArg]
	TYPE_21 = 21, // Probe[main.stringSliceArg]
	TYPE_22 = 22, // Probe[main.stringArrayArg]
	TYPE_23 = 23, // Probe[main.stringArrayArgFrameless]
	TYPE_24 = 24, // Probe[main.inlined]
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
	/* 11: string.str	*/{.byte_len = 512, .enqueue_pc = 0x0},
	/* 12: *string.str	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 13: []int.array	*/{.byte_len = 512, .enqueue_pc = 0x0},
	/* 14: *[]int.array	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 15: []string.array	*/{.byte_len = 512, .enqueue_pc = 0x1c2},
	/* 16: *[]string.array	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 17: Probe[main.intArg]	*/{.byte_len = 9, .enqueue_pc = 0x0},
	/* 18: Probe[main.stringArg]	*/{.byte_len = 17, .enqueue_pc = 0x0},
	/* 19: Probe[main.intSliceArg]	*/{.byte_len = 25, .enqueue_pc = 0x0},
	/* 20: Probe[main.intArrayArg]	*/{.byte_len = 25, .enqueue_pc = 0x0},
	/* 21: Probe[main.stringSliceArg]	*/{.byte_len = 25, .enqueue_pc = 0x0},
	/* 22: Probe[main.stringArrayArg]	*/{.byte_len = 49, .enqueue_pc = 0x0},
	/* 23: Probe[main.stringArrayArgFrameless]	*/{.byte_len = 49, .enqueue_pc = 0x0},
	/* 24: Probe[main.inlined]	*/{.byte_len = 9, .enqueue_pc = 0x0},
};

const uint32_t type_ids[] = {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, };

const uint32_t num_types = 24;

const throttler_params_t throttler_params[] = {
	{.period_ns = 1000000000, .budget = 1},
	{.period_ns = 1000000000, .budget = 1},
	{.period_ns = 1000000000, .budget = 1},
	{.period_ns = 1000000000, .budget = 1},
	{.period_ns = 1000000000, .budget = 1},
	{.period_ns = 1000000000, .budget = 1},
	{.period_ns = 1000000000, .budget = 1},
	{.period_ns = 1000000000, .budget = 2},
};
#define NUM_THROTTLERS 8
