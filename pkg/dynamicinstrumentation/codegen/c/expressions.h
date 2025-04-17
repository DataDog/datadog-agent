#ifndef DI_EXPRESSIONS_H
#define DI_EXPRESSIONS_H

// read_register reads `element_size` bytes from register `reg` into a u64 which is then pushed to
// the top of the BPF parameter stack.
static __always_inline int read_register(expression_context_t *context, __u64 reg, __u32 element_size)
{
    long err;
    __u64 valueHolder = 0;
    err = bpf_probe_read_kernel(&valueHolder,  element_size, &context->ctx->DWARF_REGISTER(reg));
    if (err != 0) {
        log_debug("error when reading data from register: %ld", err);
    }
    bpf_map_push_elem(context->param_stack, &valueHolder, 0);
    context->stack_counter += 1;
    return err;
}

// read_stack reads `element_size` bytes from the traced program's stack at offset `stack_offset`
// into a u64 which is then pushed to the top of the BPF parameter stack.
static __always_inline int read_stack(expression_context_t *context, size_t stack_offset, __u32 element_size)
{
    long err;
    __u64 valueHolder = 0;
    err = bpf_probe_read_kernel(&valueHolder, element_size, &context->ctx->DWARF_STACK_REGISTER+stack_offset);
    if (err != 0) {
        log_debug("error when reading data from stack: %ld", err);
    }
    bpf_map_push_elem(context->param_stack, &valueHolder, 0);
    context->stack_counter += 1;
    return err;
}

// read_register_value_to_output reads `element_size` bytes from register `reg` into a u64 which is then written to
// the output buffer.
static __always_inline int read_register_value_to_output(expression_context_t *context, __u64 reg, __u32 element_size)
{
    long err;
    err = bpf_probe_read_kernel(&context->event->output[(context->output_offset)], element_size, &context->ctx->DWARF_REGISTER(reg));
    if (err != 0) {
        log_debug("error when reading data while reading register value to output: %ld", err);
    }
    context->output_offset += element_size;
    return err;
}

// read_stack_to_output reads `element_size` bytes from the traced program's stack at offset `stack_offset`
// into a u64 which is then written to the output buffer.
static __always_inline int read_stack_value_to_output(expression_context_t *context, __u64 stack_offset, __u32 element_size)
{
    long err;
    err = bpf_probe_read_kernel(&context->event->output[(context->output_offset)], element_size, &context->ctx->DWARF_STACK_REGISTER+stack_offset);
    if (err != 0) {
        log_debug("error when reading data while reading stack value to output: %ld", err);
    }
    context->output_offset += element_size;
    return err;
}

// pop writes to output `num_elements` elements, each of size `element_size, from the top of the stack.
static __always_inline int pop(expression_context_t *context, __u64 num_elements, __u32 element_size)
{
    long return_err;
    long err;
    __u64 valueHolder;
    int i;
    __u8 num_elements_byte = (__u8)num_elements;
    for(i = 0; i < num_elements_byte; i++) {
        bpf_map_pop_elem(context->param_stack, &valueHolder);
        context->stack_counter -= 1;
        log_debug("Popping to output: %llu", valueHolder);
        err = bpf_probe_read_kernel(&context->event->output[(context->output_offset)+i], element_size, &valueHolder);
        if (err != 0) {
            log_debug("error when reading data while popping from bpf stack: %ld", err);
            return_err = err;
        }
        context->output_offset += element_size;
    }
    return return_err;
}

// dereference pops the 8-byte address from the top of the BPF parameter stack and dereferences
// it, reading a value of size `element_size` from it, and pushes that value (encoded as a u64)
// back to the BPF parameter stack.
// It should only be used for types of 8 bytes or less (see `dereference_large`).
static __always_inline int dereference(expression_context_t *context, __u32 element_size)
{
    long err;
    __u64 addressHolder = 0;
    err = bpf_map_pop_elem(context->param_stack, &addressHolder);
    if (err != 0) {
        log_debug("Error popping: %ld", err);
    } else {
        context->stack_counter -= 1;
    }
    log_debug("Going to dereference 0x%llx", addressHolder);

    __u64 valueHolder = 0;
    err = bpf_probe_read_user(&valueHolder, element_size, (void*)addressHolder);
    if (err != 0) {
        log_debug("error when reading data while dereferencing: %ld", err);
    }
    // a mask is used to zero out bytes not used by a smaller type encoded into a __u64
    __u64 mask = (element_size == 8) ? ~0ULL : (1ULL << (8 * element_size)) - 1;
    __u64 encodedValueHolder = valueHolder & mask;

    bpf_map_push_elem(context->param_stack, &encodedValueHolder, 0);
    context->stack_counter += 1;
    return err;
}

// dereference_to_output pops the 8-byte address from the top of the BPF parameter stack and
// dereferences it, reading a value of size `element_size` from it, and writes that value
// directly to the output buffer.
// It should only be used for types of 8 bytes or less (see `dereference_large_to_output`).
static __always_inline int dereference_to_output(expression_context_t *context, __u32 element_size)
{
    long return_err;
    long err;
    __u64 addressHolder = 0;
    bpf_map_pop_elem(context->param_stack, &addressHolder);
    context->stack_counter -= 1;

    __u64 valueHolder = 0;

    log_debug("Going to deref to output: 0x%llx", addressHolder);
    err = bpf_probe_read_user(&valueHolder, element_size, (void*)addressHolder);
    if (err != 0) {
        return_err = err;
        log_debug("error when reading data while dereferencing to output: %ld", err);
    }
    // a mask is used to zero out bytes not used by a smaller type encoded into a __u64
    __u64 mask = (element_size == 8) ? ~0ULL : (1ULL << (8 * element_size)) - 1;
    __u64 encodedValueHolder = valueHolder & mask;

    log_debug("Writing %llu to output (dereferenced)", encodedValueHolder);
    if (element_size > 8) {
        element_size = 8;
    }
    err = bpf_probe_read_kernel(&context->event->output[(context->output_offset)], element_size, &encodedValueHolder);
    if (err != 0) {
        return_err = err;
        log_debug("error when reading data while dereferencing into output: %ld", err);
    }
    context->output_offset += element_size;
    return return_err;
}

// dereference_large pops the 8-byte address from the top of the BPF parameter stack and dereferences
// it, reading a value of size `element_size` from it, and pushes that value, encoded in 8-byte chunks
// to the BPF parameter stack. This is safe to use for types larger than 8-bytes.
// back to the BPF parameter stack.
static __always_inline int dereference_large(expression_context_t *context, __u32 element_size, __u8 num_chunks)
{
    long return_err;
    long err;
    __u64 addressHolder = 0;
    bpf_map_pop_elem(context->param_stack, &addressHolder);
    context->stack_counter -= 1;

    int i;
    __u32 chunk_size;
    for (i = 0; i < num_chunks; i++) {
        chunk_size = (i == num_chunks - 1 && element_size % 8 != 0) ? (element_size % 8) : 8;
        err = bpf_probe_read_user(&context->temp_storage[i], element_size, (void*)(addressHolder + (i * 8)));
        if (err != 0) {
            return_err = err;
            log_debug("error when reading data dereferencing large: %ld", err);
        }
    }

    // Mask the last chunk if element_size is not a multiple of 8
    if (element_size % 8 != 0) {
        __u64 mask = (1ULL << (8 * (element_size % 8))) - 1;
        context->temp_storage[num_chunks - 1] &= mask;
    }

    for (int i = 0; i < num_chunks; i++) {
        bpf_map_push_elem(context->param_stack, &context->temp_storage[i], 0);
        context->stack_counter += 1;
    }

    // zero out shared array
    err = bpf_probe_read_kernel(context->temp_storage, element_size*num_chunks, context->zero_string);
    if (err != 0) {
        return_err = err;
        log_debug("error when reading data zeroing out shared memory while dereferencing large: %ld", err);
    }
    return return_err;
}

// dereference_large pops the 8-byte address from the top of the BPF parameter stack and dereferences
// it, reading a value of size `element_size` from it, and writes that value to the output buffer.
// This is safe to use for types larger than 8-bytes.
static __always_inline int dereference_large_to_output(expression_context_t *context, __u32 element_size)
{
    long err;
    __u64 addressHolder = 0;
    bpf_map_pop_elem(context->param_stack, &addressHolder);
    context->stack_counter -= 1;
    err = bpf_probe_read_user(&context->event->output[(context->output_offset)], element_size, (void*)(addressHolder));
    if (err != 0) {
        log_debug("error when reading data: %ld", err);
    }
    context->output_offset += element_size;
    return err;
}

// apply_offset adds `offset` to the 8-byte address on the top of the BPF parameter stack.
static __always_inline int apply_offset(expression_context_t *context, size_t offset)
{
    __u64 addressHolder = 0;
    bpf_map_pop_elem(context->param_stack, &addressHolder);
    context->stack_counter -= 1;

    addressHolder += offset;
    bpf_map_push_elem(context->param_stack, &addressHolder, 0);
    context->stack_counter += 1;
    return 0;
}

// dereference_dynamic_to_output reads an 8-byte length from the top of the BPF parameter stack, followed by
// an 8-byte address. It applies the maximum `bytes_limit` to the length, then dereferences the address to
// the output buffer.
static __always_inline int dereference_dynamic_to_output(expression_context_t *context, __u16 bytes_limit)
{
    long err = 0;
    __u64 lengthToRead = 0;
    bpf_map_pop_elem(context->param_stack, &lengthToRead);
    context->stack_counter -= 1;

    __u64 addressHolder = 0;
    bpf_map_pop_elem(context->param_stack, &addressHolder);
    context->stack_counter -= 1;

    __u32 collection_size;
    collection_size = (__u16)lengthToRead;
    if (collection_size > bytes_limit) {
        collection_size = bytes_limit;
    }
    err = bpf_probe_read_user(&context->event->output[(context->output_offset)], collection_size, (void*)addressHolder);
    if (err != 0) {
        log_debug("error when doing dynamic dereference: %ld", err);
    }

    if (collection_size > bytes_limit) {
        collection_size = bytes_limit;
    }
    context->output_offset += collection_size;
    return err;
}

// set_limit_entry is used to set a limit for a specific collection (such as a slice). It reads the true length of the
// collection from the top of the BPF parameter stack, applies the passed `limit` to it, and updates the `collection_limit`
// map entry associated with `collection_identifier` to this limit. The `collection_identifier` is a user space generated
// and track identifier for each collection which can be referenced in BPF code.
static __always_inline int set_limit_entry(expression_context_t *context, __u16 limit, char collection_identifier[6])
{
    // Read the 2 byte length from top of the stack, then set collectionLimit to the minimum of the two
    __u64 length;
    bpf_map_pop_elem(context->param_stack, &length);
    context->stack_counter -= 1;

    __u16 lengthShort = (__u16)length;
    if (lengthShort > limit) {
        lengthShort = limit;
    }

    long err = 0;
    err = bpf_map_update_elem(&collection_limits, collection_identifier, &lengthShort, BPF_ANY);
    if (err != 0) {
        log_debug("error updating collection limit for %s to %hu: %ld", collection_identifier, lengthShort, err);
    }

    log_debug("set limit entry for %s to %hu", collection_identifier, lengthShort);
    return 0;
}

// copy duplicates the u64 element on the top of the BPF parameter stack.
static __always_inline int copy(expression_context_t *context)
{
    __u64 holder;
    bpf_map_peek_elem(context->param_stack, &holder);
    bpf_map_push_elem(context->param_stack, &holder, 0);
    context->stack_counter += 1;
    return 0;
}

// read_str_to_output reads a Go string to the output buffer, limited in length by `limit`.
// In Go, strings are internally implemented as structs with two fields. The fields are length, 
// and a pointer to a character array. This expression expects the address of the string struct
// itself to be on the top of the stack.
static __always_inline int read_str_to_output(expression_context_t *context, __u16 limit)
{
    long err;
    __u64 stringStructAddressHolder = 0;
    err = bpf_map_pop_elem(context->param_stack, &stringStructAddressHolder);
    if (err != 0) {
        log_debug("error popping string struct addr: %ld", err);
        return err;
    }
    context->stack_counter -= 1;

    if (stringStructAddressHolder == 0) {
        log_debug("invalid string struct address: 0");
        return -1;
    }

    char* characterPointer;
    err = bpf_probe_read_user(&characterPointer, sizeof(characterPointer), (void*)(stringStructAddressHolder));
    if (err != 0) {
        log_debug("error reading character pointer: %ld", err);
        return err;
    }

    log_debug("Reading string from 0x%p", characterPointer);

    // Use temporary length variable to satisfy verifier
    // It's not entirely clear why, but it appears the verifier can't trace the origin
    // of length if it isn't via a register assignment.
    __u32 length;
    __u32 temp_length;
    err = bpf_probe_read_user(&temp_length, sizeof(temp_length), (void*)(stringStructAddressHolder+8));
    if (err != 0) {
        log_debug("error reading string length: %ld", err);
        return err;
    }
    length = temp_length;
    if (length > limit) {
        length = limit;
    }
    err = bpf_probe_read_user(&context->event->output[(context->output_offset)], length, (char*)characterPointer);
    if (err != 0) {
        log_debug("error reading string: %ld", err);
    }
    context->output_offset += length;
    log_debug("Read %u bytes (limit = %hu)", length, limit);

    return err;
}
#endif
