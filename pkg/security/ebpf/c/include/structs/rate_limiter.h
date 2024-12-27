#ifndef _STRUCTS_RATE_LIMITER_H_
#define _STRUCTS_RATE_LIMITER_H_

struct rate_limiter_ctx {
    u64 current_period;
    u32 counter;
    u32 padding;
};


#endif /* _STRUCTS_RATE_LIMITER_H_ */
