#ifndef _CONSTANTS_OFFSETS_SPLICE_H_
#define _CONSTANTS_OFFSETS_SPLICE_H_

#include "constants/macros.h"

u64 __attribute__((always_inline)) get_pipe_inode_info_bufs_offset(void) {
    u64 pipe_inode_info_bufs_offset;
    LOAD_CONSTANT("pipe_inode_info_bufs_offset", pipe_inode_info_bufs_offset);
    return pipe_inode_info_bufs_offset;
}

int __attribute__((always_inline)) get_pipe_last_buffer_flags(struct pipe_inode_info *pipe, struct pipe_buffer *bufs) {
    u64 kernel_has_legacy_pipe_inode_info;
    LOAD_CONSTANT("kernel_has_legacy_pipe_inode_info", kernel_has_legacy_pipe_inode_info);

    struct pipe_buffer *pipe_last_buffer = NULL;

    if (kernel_has_legacy_pipe_inode_info) { // kernels < 5.5
        u64 pipe_inode_info_nrbufs_offset;
        LOAD_CONSTANT("pipe_inode_info_nrbufs_offset", pipe_inode_info_nrbufs_offset);

        u64 pipe_inode_info_curbuf_offset;
        LOAD_CONSTANT("pipe_inode_info_curbuf_offset", pipe_inode_info_curbuf_offset);

        u64 pipe_inode_info_buffers_offset;
        LOAD_CONSTANT("pipe_inode_info_buffers_offset", pipe_inode_info_buffers_offset);

        unsigned int nrbufs, curbuf, buffers;
        bpf_probe_read(&nrbufs, sizeof(nrbufs), (void *)pipe + pipe_inode_info_nrbufs_offset);
        bpf_probe_read(&curbuf, sizeof(curbuf), (void *)pipe + pipe_inode_info_curbuf_offset);
        bpf_probe_read(&buffers, sizeof(buffers), (void *)pipe + pipe_inode_info_buffers_offset);

        unsigned int last_buffer_index = nrbufs > 0 ? nrbufs - 1 : 0;
        pipe_last_buffer = bufs + ((curbuf + last_buffer_index) & (buffers - 1));
    } else {
        u64 pipe_inode_info_head_offset;
        LOAD_CONSTANT("pipe_inode_info_head_offset", pipe_inode_info_head_offset);

        u64 pipe_inode_info_ring_size_offset;
        LOAD_CONSTANT("pipe_inode_info_ring_size_offset", pipe_inode_info_ring_size_offset);

        unsigned int head, ring_size;
        bpf_probe_read(&head, sizeof(head), (void *)pipe + pipe_inode_info_head_offset);
        bpf_probe_read(&ring_size, sizeof(ring_size), (void *)pipe + pipe_inode_info_ring_size_offset);

        unsigned int last_buffer_index = head > 0 ? head - 1 : 0;
        pipe_last_buffer = bufs + (last_buffer_index & (ring_size - 1));
    }

    if (!pipe_last_buffer) {
        return 0;
    }

    int pipe_last_buffer_flags;
    bpf_probe_read(&pipe_last_buffer_flags, sizeof(pipe_last_buffer_flags), &pipe_last_buffer->flags);
    return pipe_last_buffer_flags;
}

#endif
