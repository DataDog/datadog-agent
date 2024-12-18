// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#ifndef DI_EXPRESIONS_H
#define DI_EXPRESIONS_H

static __always_inline int read_register(struct expression_context context, __u64 reg, __u32 element_size)
{
    long err;
    __u64 valueHolder = 0;
    err = bpf_probe_read(&valueHolder,  element_size, &context.ctx->DWARF_REGISTER(reg));
    if (err != 0) {
        log_debug("error when reading data from register: %ld", err);
    }
    bpf_map_push_elem(&param_stack, &valueHolder, 0);
    return err;
}

static __always_inline int read_stack(struct expression_context context, size_t stack_offset, __u32 element_size)
{
    long err;
    __u64 valueHolder = 0;
    err = bpf_probe_read(&valueHolder, element_size, &context.ctx->DWARF_STACK_REGISTER+stack_offset);
    if (err != 0) {
        log_debug("error when reading data from stack: %ld", err);
    }
    bpf_map_push_elem(&param_stack, &valueHolder, 0);
    return err;
}

static __always_inline int read_register_value_to_output(struct expression_context context, __u64 reg, __u32 element_size)
{
    long err;
    err = bpf_probe_read(&context.event->output[*(context.output_offset)], element_size, &context.ctx->DWARF_REGISTER(reg));
    if (err != 0) {
        log_debug("error when reading data while reading register value to output: %ld", err);
    }
    *context.output_offset += element_size;
    return err;
}

static __always_inline int read_stack_value_to_output(struct expression_context context, __u64 stack_offset, __u32 element_size)
{
    long err;
    err = bpf_probe_read(&context.event->output[*(context.output_offset)], element_size, &context.ctx->DWARF_STACK_REGISTER+stack_offset);
    if (err != 0) {
        log_debug("error when reading data while reading stack value to output: %ld", err);
    }
    *context.output_offset += element_size;
    return err;
}

static __always_inline int pop(struct expression_context context, __u64 num_elements, __u32 element_size)
{
    long return_err;
    long err;
    __u64 valueHolder;
    int i;
    for(i = 0; i < num_elements; i++) {
        bpf_map_pop_elem(&param_stack, &valueHolder);
        log_debug("Popping to output: %llu", valueHolder);
        err = bpf_probe_read(&context.event->output[*(context.output_offset)+i], element_size, &valueHolder);
        if (err != 0) {
            log_debug("error when reading data while popping from bpf stack: %ld", err);
            return_err = err;
        }
        *context.output_offset += element_size;
    }
    return return_err;
}

static __always_inline int dereference(struct expression_context context, __u32 element_size)
{
    long err;
    __u64 addressHolder = 0;
    err = bpf_map_pop_elem(&param_stack, &addressHolder);
    if (err != 0) {
        log_debug("Error popping: %ld", err);
    }
    log_debug("Going to dereference 0x%llx", addressHolder);

    __u64 valueHolder = 0;
    err = bpf_probe_read_user(&valueHolder, element_size, (void*)addressHolder);
    if (err != 0) {
        log_debug("error when reading data while dereferencing: %ld", err);
    }
    __u64 mask = (element_size == 8) ? ~0ULL : (1ULL << (8 * element_size)) - 1;
    __u64 encodedValueHolder = valueHolder & mask;

    bpf_map_push_elem(&param_stack, &encodedValueHolder, 0);
    return err;
}

static __always_inline int dereference_to_output(struct expression_context context, __u32 element_size)
{
    long return_err;
    long err;
    __u64 addressHolder = 0;
    bpf_map_pop_elem(&param_stack, &addressHolder);

    __u64 valueHolder = 0;

    log_debug("Going to deref to output: 0x%llx", addressHolder);
    err = bpf_probe_read(&valueHolder, element_size, (void*)addressHolder);
    if (err != 0) {
        return_err = err;
        log_debug("error when reading data while dereferencing to output: %ld", err);
    }
    __u64 mask = (element_size == 8) ? ~0ULL : (1ULL << (8 * element_size)) - 1;
    __u64 encodedValueHolder = valueHolder & mask;

    log_debug("Writing %llu to output (dereferenced)", encodedValueHolder);
    err = bpf_probe_read(&context.event->output[*(context.output_offset)], element_size, &encodedValueHolder);
    if (err != 0) {
        return_err = err;
        log_debug("error when reading data while dereferencing into output: %ld", err);
    }
    *context.output_offset += element_size;
    return return_err;
}

static __always_inline int dereference_large(struct expression_context context, __u32 element_size, __u8 num_chunks)
{
    long return_err;
    long err;
    __u64 addressHolder = 0;
    bpf_map_pop_elem(&param_stack, &addressHolder);

    int i;
    __u32 chunk_size;
    for (i = 0; i < num_chunks; i++) {
        chunk_size = (i == num_chunks - 1 && element_size % 8 != 0) ? (element_size % 8) : 8;
        err = bpf_probe_read(&context.temp_storage[i], element_size, (void*)(addressHolder + (i * 8)));
        if (err != 0) {
            return_err = err;
            log_debug("error when reading data dereferencing large: %ld", err);
        }
    }

    // Mask the last chunk if element_size is not a multiple of 8
    if (element_size % 8 != 0) {
        __u64 mask = (1ULL << (8 * (element_size % 8))) - 1;
        context.temp_storage[num_chunks - 1] &= mask;
    }

    for (int i = 0; i < num_chunks; i++) {
        bpf_map_push_elem(&param_stack, &context.temp_storage[i], 0);
    }

    // zero out shared array
    err = bpf_probe_read(context.temp_storage, element_size*num_chunks, context.zero_string);
    if (err != 0) {
        return_err = err;
        log_debug("error when reading data zeroing out shared memory while dereferencing large: %ld", err);
    }
    return return_err;
}

static __always_inline int dereference_large_to_output(struct expression_context context, __u32 element_size)
{
    long err;
    __u64 addressHolder = 0;
    bpf_map_pop_elem(&param_stack, &addressHolder);
    err = bpf_probe_read(&context.event->output[*(context.output_offset)], element_size, (void*)(addressHolder));
    if (err != 0) {
        log_debug("error when reading data: %ld", err);
    }
    *context.output_offset += element_size;
    return err;
}

static __always_inline int apply_offset(struct expression_context context, size_t offset)
{
    __u64 addressHolder = 0;
    bpf_map_pop_elem(&param_stack, &addressHolder);
    addressHolder += offset;
    bpf_map_push_elem(&param_stack, &addressHolder, 0);
    return 0;
}

// Expects the stack to set up such that first pop is length, second is address
static __always_inline int dereference_dynamic(struct expression_context context, __u32 bytes_limit, __u8 num_chunks, __u32 element_size)
{
    long return_err;
    long err;
    __u64 lengthToRead = 0;
    bpf_map_pop_elem(&param_stack, &lengthToRead);

    __u64 addressHolder = 0;
    bpf_map_pop_elem(&param_stack, &addressHolder);

    int i;
    __u32 chunk_size;
    for (i = 0; i < num_chunks; i++) {
        chunk_size = (i == num_chunks - 1 && bytes_limit % 8 != 0) ? (bytes_limit % 8) : 8;
        err = bpf_probe_read(&context.temp_storage[i], chunk_size, (void*)(addressHolder + (i * 8)));
        if (err != 0) {
            return_err = err;
            log_debug("error when reading data dereferencing dynamically into shared memory: %ld", err);
        }
    }

    for (i = 0; i < num_chunks; i++) {
        err = bpf_probe_read(&context.event->output[*(context.output_offset)], 8, &context.temp_storage[i]);
        if (err != 0) {
            return_err = err;
            log_debug("error when reading data dereferencing dynamically: %ld", err);
        }
        *context.output_offset += 8;
    }
    return return_err;
}

// Expects the stack to set up such that first pop is length, second is address
static __always_inline int dereference_dynamic_to_output(struct expression_context context, __u16 bytes_limit)
{
    long err = 0;
    __u64 lengthToRead = 0;
    bpf_map_pop_elem(&param_stack, &lengthToRead);

    __u64 addressHolder = 0;
    bpf_map_pop_elem(&param_stack, &addressHolder);

    __u32 collection_size;
    collection_size = (__u16)lengthToRead;
    if (collection_size > bytes_limit) {
        collection_size = bytes_limit;
    }
    err = bpf_probe_read(&context.event->output[*(context.output_offset)], collection_size, (void*)addressHolder);
    if (err != 0) {
        log_debug("error when doing dynamic dereference: %ld", err);
    }

    if (collection_size > bytes_limit) {
        collection_size = bytes_limit;
    }
    *context.output_offset += collection_size;
    return err;
}

static __always_inline int set_limit_entry(struct expression_context context, __u16 limit, char collection_identifier[6])
{
    // Read the 2 byte length from top of the stack, then set collectionLimit to the minimum of the two
    __u64 length;
    bpf_map_pop_elem(&param_stack, &length);

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

static __always_inline int copy(struct expression_context context)
{
    __u64 holder;
    bpf_map_peek_elem(&param_stack, &holder);
    bpf_map_push_elem(&param_stack, &holder, 0);
    return 0;
}

static __always_inline int read_str_to_output(struct expression_context context, __u16 limit)
{
    // Expect the address of the string struct on the stack
    long err;

    __u64 stringStructAddressHolder = 0;
    err = bpf_map_pop_elem(&param_stack, &stringStructAddressHolder);
    if (err != 0) {
        log_debug("error popping string struct addr: %ld", err);
        return err;
    }

    char* characterPointer = 0;
    err = bpf_probe_read(&characterPointer, sizeof(characterPointer), (void*)(stringStructAddressHolder));
    log_debug("Reading from 0x%p", characterPointer);

    __u32 length;
    err = bpf_probe_read(&length, sizeof(length), (void*)(stringStructAddressHolder+8));
    if (err != 0) {
        log_debug("error reading string length: %ld", err);
        return err;
    }
    if (length > limit) {
        length = limit;
    }
    err = bpf_probe_read(&context.event->output[*(context.output_offset)], length, (char*)characterPointer);
    if (err != 0) {
        log_debug("error reading string: %ld", err);
    }
    if (length > limit) {
        length = limit;
    }
    *context.output_offset += length;
    log_debug("Read %u bytes (limit = %hu)", length, limit);

    return err;
}
#endif
