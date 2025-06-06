const uint8_t stack_machine_code[] = {
		SM_OP_ILLEGAL, 

	// 0x1: ChasePointers
		SM_OP_CHASE_POINTERS, 
		SM_OP_RETURN, 

	// 0x3: ProcessExpression[Probe[main.mapArg]@0x9ff2c.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x12: ProcessEvent[Probe[main.mapArg]@9ff2c]
		SM_OP_PREPARE_EVENT_ROOT, 0x27, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x03, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.mapArg]@0x9ff2c.expr[0]]
		SM_OP_RETURN, 

	// 0x21: ProcessExpression[Probe[main.bigMapArg]@0x9ffa0.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x30: ProcessEvent[Probe[main.bigMapArg]@9ffa0]
		SM_OP_PREPARE_EVENT_ROOT, 0x28, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x21, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.bigMapArg]@0x9ffa0.expr[0]]
		SM_OP_RETURN, 

	// 0x3f: ProcessType[string]
		SM_OP_PROCESS_STRING, 0x23, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x45: ProcessExpression[Probe[main.stringArg]@0xa00dc.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_READ_REGISTER, 0x01, 0x08, 0x08, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x3f, 0x00, 0x00, 0x00, // ProcessType[string]
		SM_OP_RETURN, 

	// 0x67: ProcessEvent[Probe[main.stringArg]@a00dc]
		SM_OP_PREPARE_EVENT_ROOT, 0x29, 0x00, 0x00, 0x00, 0x11, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x45, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.stringArg]@0xa00dc.expr[0]]
		SM_OP_RETURN, 

	// 0x76: ProcessExpression[Probe[main.inlined]@0xa01dc.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_EXPR_READ_REGISTER, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0x8c: ProcessEvent[Probe[main.inlined]@a01dc]
		SM_OP_PREPARE_EVENT_ROOT, 0x2a, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x76, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.inlined]@0xa01dc.expr[0]]
		SM_OP_RETURN, 

	// 0x9b: ProcessExpression[Probe[main.inlined]@0x9fec0.expr[0]]
		SM_OP_EXPR_PREPARE, 
		SM_OP_RETURN, 
		SM_OP_EXPR_SAVE, 0x01, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 
		SM_OP_RETURN, 

	// 0xab: ProcessEvent[Probe[main.inlined]@9fec0]
		SM_OP_PREPARE_EVENT_ROOT, 0x2a, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 
		SM_OP_CALL, 0x9b, 0x00, 0x00, 0x00, // ProcessExpression[Probe[main.inlined]@0x9fec0.expr[0]]
		SM_OP_RETURN, 
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
const uint64_t stack_machine_code_len = 199;
const uint32_t stack_machine_code_max_op = 13;
const uint32_t chase_pointers_entrypoint = 0x1;

const probe_params_t probe_params[] = {
	{.stack_machine_pc = 0x12, .stream_id = 0, .frameless = false},
	{.stack_machine_pc = 0x30, .stream_id = 0, .frameless = false},
	{.stack_machine_pc = 0x67, .stream_id = 0, .frameless = false},
	{.stack_machine_pc = 0x8c, .stream_id = 0, .frameless = false},
	{.stack_machine_pc = 0xab, .stream_id = 0, .frameless = false},
};
const uint32_t num_probe_params = 5;
typedef enum type {
	TYPE_NONE = 0,
	TYPE_1 = 1, // int
	TYPE_2 = 2, // map[string]int
	TYPE_3 = 3, // *map<string,int>
	TYPE_4 = 4, // map<string,int>
	TYPE_5 = 5, // uint64
	TYPE_6 = 6, // uintptr
	TYPE_7 = 7, // **table<string,int>
	TYPE_8 = 8, // *table<string,int>
	TYPE_9 = 9, // table<string,int>
	TYPE_10 = 10, // uint16
	TYPE_11 = 11, // uint8
	TYPE_12 = 12, // groupReference<string,int>
	TYPE_13 = 13, // *noalg.map.group[string]int
	TYPE_14 = 14, // noalg.map.group[string]int
	TYPE_15 = 15, // noalg.[8]struct { key string; elem int }
	TYPE_16 = 16, // noalg.struct { key string; elem int }
	TYPE_17 = 17, // string
	TYPE_18 = 18, // *uint8
	TYPE_19 = 19, // map[string]main.bigStruct
	TYPE_20 = 20, // *map<string,main.bigStruct>
	TYPE_21 = 21, // map<string,main.bigStruct>
	TYPE_22 = 22, // **table<string,main.bigStruct>
	TYPE_23 = 23, // *table<string,main.bigStruct>
	TYPE_24 = 24, // table<string,main.bigStruct>
	TYPE_25 = 25, // groupReference<string,main.bigStruct>
	TYPE_26 = 26, // *noalg.map.group[string]main.bigStruct
	TYPE_27 = 27, // noalg.map.group[string]main.bigStruct
	TYPE_28 = 28, // noalg.[8]struct { key string; elem *main.bigStruct }
	TYPE_29 = 29, // noalg.struct { key string; elem *main.bigStruct }
	TYPE_30 = 30, // *main.bigStruct
	TYPE_31 = 31, // main.bigStruct
	TYPE_32 = 32, // [128]uint8
	TYPE_33 = 33, // []*table<string,int>.array
	TYPE_34 = 34, // []noalg.map.group[string]int.array
	TYPE_35 = 35, // string.str
	TYPE_36 = 36, // *string.str
	TYPE_37 = 37, // []*table<string,main.bigStruct>.array
	TYPE_38 = 38, // []noalg.map.group[string]main.bigStruct.array
	TYPE_39 = 39, // Probe[main.mapArg]
	TYPE_40 = 40, // Probe[main.bigMapArg]
	TYPE_41 = 41, // Probe[main.stringArg]
	TYPE_42 = 42, // Probe[main.inlined]
} type_t;

const type_info_t type_info[] = {
	/* 1: int	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 2: map[string]int	*/{.byte_len = 0, .enqueue_pc = 0x0},
	/* 3: *map<string,int>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 4: map<string,int>	*/{.byte_len = 48, .enqueue_pc = 0x0},
	/* 5: uint64	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 6: uintptr	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 7: **table<string,int>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 8: *table<string,int>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 9: table<string,int>	*/{.byte_len = 32, .enqueue_pc = 0x0},
	/* 10: uint16	*/{.byte_len = 2, .enqueue_pc = 0x0},
	/* 11: uint8	*/{.byte_len = 1, .enqueue_pc = 0x0},
	/* 12: groupReference<string,int>	*/{.byte_len = 16, .enqueue_pc = 0x0},
	/* 13: *noalg.map.group[string]int	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 14: noalg.map.group[string]int	*/{.byte_len = 200, .enqueue_pc = 0x0},
	/* 15: noalg.[8]struct { key string; elem int }	*/{.byte_len = 192, .enqueue_pc = 0x0},
	/* 16: noalg.struct { key string; elem int }	*/{.byte_len = 24, .enqueue_pc = 0x0},
	/* 17: string	*/{.byte_len = 16, .enqueue_pc = 0x3f},
	/* 18: *uint8	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 19: map[string]main.bigStruct	*/{.byte_len = 0, .enqueue_pc = 0x0},
	/* 20: *map<string,main.bigStruct>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 21: map<string,main.bigStruct>	*/{.byte_len = 48, .enqueue_pc = 0x0},
	/* 22: **table<string,main.bigStruct>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 23: *table<string,main.bigStruct>	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 24: table<string,main.bigStruct>	*/{.byte_len = 32, .enqueue_pc = 0x0},
	/* 25: groupReference<string,main.bigStruct>	*/{.byte_len = 16, .enqueue_pc = 0x0},
	/* 26: *noalg.map.group[string]main.bigStruct	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 27: noalg.map.group[string]main.bigStruct	*/{.byte_len = 200, .enqueue_pc = 0x0},
	/* 28: noalg.[8]struct { key string; elem *main.bigStruct }	*/{.byte_len = 192, .enqueue_pc = 0x0},
	/* 29: noalg.struct { key string; elem *main.bigStruct }	*/{.byte_len = 24, .enqueue_pc = 0x0},
	/* 30: *main.bigStruct	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 31: main.bigStruct	*/{.byte_len = 184, .enqueue_pc = 0x0},
	/* 32: [128]uint8	*/{.byte_len = 128, .enqueue_pc = 0x0},
	/* 33: []*table<string,int>.array	*/{.byte_len = 2048, .enqueue_pc = 0x0},
	/* 34: []noalg.map.group[string]int.array	*/{.byte_len = 512, .enqueue_pc = 0x0},
	/* 35: string.str	*/{.byte_len = 512, .enqueue_pc = 0x0},
	/* 36: *string.str	*/{.byte_len = 8, .enqueue_pc = 0x0},
	/* 37: []*table<string,main.bigStruct>.array	*/{.byte_len = 2048, .enqueue_pc = 0x0},
	/* 38: []noalg.map.group[string]main.bigStruct.array	*/{.byte_len = 512, .enqueue_pc = 0x0},
	/* 39: Probe[main.mapArg]	*/{.byte_len = 1, .enqueue_pc = 0x0},
	/* 40: Probe[main.bigMapArg]	*/{.byte_len = 1, .enqueue_pc = 0x0},
	/* 41: Probe[main.stringArg]	*/{.byte_len = 17, .enqueue_pc = 0x0},
	/* 42: Probe[main.inlined]	*/{.byte_len = 9, .enqueue_pc = 0x0},
};

const uint32_t type_ids[] = {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, };

const uint32_t num_types = 42;

