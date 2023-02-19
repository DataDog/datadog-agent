//#include "kconfig.h"
//
//#include "bpf_tracing.h"
//#include "tracer.h"
//#include "bpf_helpers.h"
//#include "bpf_telemetry.h"
//#include "bpf_builtins.h"
//#include "ip.h"
//#include "port_range.h"
//
//#include "protocols/kafka-maps.h"
//#include "protocols/kafka-types.h"
//#include "protocols/kafka-buffer.h"
//#include "protocols/kafka.h"
//
//// This entry point is needed to bypass a memory limit on socket filters
//// See: https://datadoghq.atlassian.net/wiki/spaces/NET/pages/2326855913/HTTP#Known-issues
//SEC("socket/kafka_filter_entry")
//int socket__kafka_filter_entry(struct __sk_buff *skb) {
//    bpf_tail_call_compat(skb, &kafka_progs, KAFKA_PROG);
//    return 0;
//}
//
////SEC("socket/kafka_filter")
////int socket__kafka_filter(struct __sk_buff* skb) {
////    skb_info_t skb_info;
////    u32 zero = 0;
////    kafka_transaction_t *kafka = bpf_map_lookup_elem(&kafka_heap, &zero);
////    if (kafka == NULL) {
////        log_debug("socket__kafka_filter: kafka_transaction state is NULL\n");
////        return 0;
////    }
////    bpf_memset(kafka, 0, sizeof(kafka_transaction_t));
////
////    if (!read_conn_tuple_skb(skb, &skb_info, &kafka->base.tup)) {
////        return 0;
////    }
////
////    if (!kafka_allow_packet(kafka, skb, &skb_info)) {
////        return 0;
////    }
////
////    normalize_tuple(&kafka->base.tup);
////
////    read_into_buffer_skb((char *)kafka->request_fragment, skb, &skb_info);
////    kafka_process(kafka);
////    return 0;
////}
//
//SEC("tracepoint/net/netif_receive_skb")
//int tracepoint__net__netif_receive_skb(struct pt_regs* ctx) {
//    // flush batch to userspace
//    // because perf events can't be sent from socket filter programs
//    kafka_flush_batch(ctx);
//    return 0;
//}
//
//// This number will be interpreted by elf-loader to set the current running kernel version
//__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)
//
//char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
