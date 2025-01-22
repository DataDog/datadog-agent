#ifndef _STRUCTS_RATE_LIMITER_H_
#define _STRUCTS_RATE_LIMITER_H_

struct rate_limiter_ctx {
    u32 current_period;
    u32 counter;
};


#endif /* _STRUCTS_RATE_LIMITER_H_ */
