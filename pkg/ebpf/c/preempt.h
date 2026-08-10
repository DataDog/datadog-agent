#ifndef __PREEMPT_H__
#define __PREEMPT_H__

#include <asm-generic/errno-base.h>

#define PREEMPT_BITS	8
#define SOFTIRQ_BITS	8
#define HARDIRQ_BITS	4
#define NMI_BITS	4

#define PREEMPT_SHIFT	0
#define SOFTIRQ_SHIFT	(PREEMPT_SHIFT + PREEMPT_BITS)
#define HARDIRQ_SHIFT	(SOFTIRQ_SHIFT + SOFTIRQ_BITS)
#define NMI_SHIFT	(HARDIRQ_SHIFT + HARDIRQ_BITS)

#define __IRQ_MASK(x)	((1UL << (x))-1)

#define SOFTIRQ_MASK	(__IRQ_MASK(SOFTIRQ_BITS) << SOFTIRQ_SHIFT)
#define HARDIRQ_MASK	(__IRQ_MASK(HARDIRQ_BITS) << HARDIRQ_SHIFT)
#define NMI_MASK	(__IRQ_MASK(NMI_BITS)     << NMI_SHIFT)

#define SOFTIRQ_OFFSET	(1UL << SOFTIRQ_SHIFT)

#ifdef bpf_target_x86
extern const int __preempt_count __ksym __weak;
volatile const u64 use_preempt_count = 0;

struct pcpu_hot___local {
	int preempt_count;
} __attribute__((preserve_access_index));

extern struct pcpu_hot___local pcpu_hot __ksym __weak;
#endif

#define bpf_ksym_exists(sym) ({									\
	_Static_assert(!__builtin_constant_p(!!sym), #sym " should be marked as __weak");	\
	!!sym;											\
})

static __always_inline int get_preempt_count(void)
{
#if defined(bpf_target_x86)
	/* By default, read the per-CPU __preempt_count. */
    if (use_preempt_count)
		return *(int *) bpf_this_cpu_ptr(&__preempt_count);

	/*
	 * If __preempt_count does not exist, try to read preempt_count under
	 * struct pcpu_hot. Between v6.1 and v6.14 -- more specifically,
	 * [64701838bf057, 46e8fff6d45fe), preempt_count had been managed
	 * under struct pcpu_hot.
	 */
	if (bpf_core_field_exists(pcpu_hot.preempt_count))
		return ((struct pcpu_hot___local *)
			bpf_this_cpu_ptr(&pcpu_hot))->preempt_count;
#elif defined(bpf_target_arm64)
	return bpf_get_current_task_btf()->thread_info.preempt.count;
#endif
    return 0;
}

/* Description
 *	Report whether it is in interrupt context. Only works on the following archs:
 *	* x86
 *	* arm64
 *
 *  Does not support CONFIG_PREEMPT_RT
 */
static __always_inline int bpf_in_interrupt(int pcnt)
{
	return pcnt & (NMI_MASK | HARDIRQ_MASK | SOFTIRQ_MASK);
}

/* Description
 *	Report whether it is in NMI context. Only works on the following archs:
 *	* x86
 *	* arm64
 */
static __always_inline int bpf_in_nmi(int pcnt)
{
	return pcnt & NMI_MASK;
}

/* Description
 *	Report whether it is in hard IRQ context. Only works on the following archs:
 *	* x86
 *	* arm64
 *
 *  Does not support CONFIG_PREEMPT_RT
 */
static __always_inline int bpf_in_hardirq(int pcnt)
{
	return pcnt & HARDIRQ_MASK;
}

/* Description
 *	Report whether it is in softirq context. Only works on the following archs:
 *	* x86
 *	* arm64
 *
 *  Does not support CONFIG_PREEMPT_RT
 */
static __always_inline int bpf_in_serving_softirq(int pcnt)
{
	return (pcnt & SOFTIRQ_MASK) & SOFTIRQ_OFFSET;
}

/* Description
 *	Report whether it is in task context. Only works on the following archs:
 *	* x86
 *	* arm64
 *
 *  Does not support CONFIG_PREEMPT_RT
 */
static __always_inline int bpf_in_task(int pcnt)
{
	return !(pcnt & (NMI_MASK | HARDIRQ_MASK | SOFTIRQ_OFFSET));
}


#define TASK_DEPTH      0 
#define SOFTIRQ_DEPTH   1
#define HARDIRQ_DEPTH   2
#define NMI_DEPTH       3
static __always_inline int get_nesting_depth(void) 
{
    int pcnt = get_preempt_count();

    if (bpf_in_nmi(pcnt))
        return NMI_DEPTH;

    if (bpf_in_hardirq(pcnt))
        return HARDIRQ_DEPTH;

    if (bpf_in_serving_softirq(pcnt))
        return SOFTIRQ_DEPTH;

    if (bpf_in_task(pcnt))
        return TASK_DEPTH;

    return -1;
}

#endif // __PREEMPT_H__
