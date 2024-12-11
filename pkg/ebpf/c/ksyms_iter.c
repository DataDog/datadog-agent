// SPDX-License-Identifier: GPL-2.0
/* Copyright (c) 2020 Facebook */

#include "ktypes.h"
#include "bpf_helpers.h"
#include "bpf_metadata.h"
static volatile unsigned long last_sym_value = 0;

static __always_inline char to_lower(char c)
{
	if (c >= 'A' && c <= 'Z')
		c += ('a' - 'A');
	return c;
}

static __always_inline char to_upper(char c)
{
	if (c >= 'a' && c <= 'z')
		c -= ('a' - 'A');
	return c;
}

struct bpf_iter__ksym {
	struct bpf_iter_meta *meta;
	struct kallsym_iter *ksym;
};

/* Dump symbols with max size; the latter is calculated by caching symbol N value
 * and when iterating on symbol N+1, we can print max size of symbol N via
 * address of N+1 - address of N.
 */
SEC("iter/ksym")
int bpf_iter__dump_ksyms(struct bpf_iter__ksym *ctx)
{
	struct seq_file *seq = ctx->meta->seq;
	struct kallsym_iter *iter = ctx->ksym;
	unsigned long value;
	char type;

	if (!iter)
		return 0;

	if (last_sym_value)
		BPF_SEQ_PRINTF(seq, "0x%x\n", iter->value - last_sym_value);
	else
		BPF_SEQ_PRINTF(seq, "\n");

	value = iter->value;

	last_sym_value = value;

	type = iter->type;

	if (iter->module_name[0]) {
		type = iter->exported ? to_upper(type) : to_lower(type);
		BPF_SEQ_PRINTF(seq, "%llx %c %s [ %s ] ",
			       value, type, iter->name, iter->module_name);
	} else {
		BPF_SEQ_PRINTF(seq, "%llx %c %s ", value, type, iter->name);
	}
	if (!iter->pos_mod_end || iter->pos_mod_end > iter->pos)
		BPF_SEQ_PRINTF(seq, "MOD ");
	else if (!iter->pos_ftrace_mod_end || iter->pos_ftrace_mod_end > iter->pos)
		BPF_SEQ_PRINTF(seq, "FTRACE_MOD ");
	else if (!iter->pos_bpf_end || iter->pos_bpf_end > iter->pos)
		BPF_SEQ_PRINTF(seq, "BPF ");
	else
		BPF_SEQ_PRINTF(seq, "KPROBE ");
	return 0;
}

char _license[] SEC("license") = "GPL";
