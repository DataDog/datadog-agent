#ifndef __KAFKA_USM_EVENTS
#define __KAFKA_USM_EVENTS

#include "protocols/kafka/types.h"
#include "protocols/direct_consumer.h"
#include "protocols/events.h"

// This controls the number of Kafka transactions read from userspace at a time
#define KAFKA_BATCH_SIZE (MAX_BATCH_SIZE(kafka_event_t))

USM_EVENTS_INIT(kafka, kafka_event_t, KAFKA_BATCH_SIZE);

// The DirectConsumer emit path is inlined into kafka_batch_enqueue_wrapper, which the Kafka v12
// fetch-response parser inlines at every enqueue site (~6x). The extra instructions push the
// program past the 4096-instruction verifier limit (BPF_MAXINSNS) on kernels < 5.2 - the dead
// branch counts toward program size even though LOAD_CONSTANT prunes it at verify time.
// DirectConsumer also requires kernel >= 5.8 (perf/ringbuf output from socket filters), so compile
// it out where it cannot be used: prebuilt objects load on every kernel (including < 5.2), and
// runtime-compiled objects built for kernels < 5.8. CO-RE objects only load where BTF is present
// (>= 5.x, well above the 4096-insn limit) so they keep the path.
#if defined(COMPILE_PREBUILT) || (defined(COMPILE_RUNTIME) && LINUX_VERSION_CODE < KERNEL_VERSION(5, 8, 0))
#define KAFKA_DIRECT_CONSUMER_ENABLED 0
#else
#define KAFKA_DIRECT_CONSUMER_ENABLED 1
#endif

#if KAFKA_DIRECT_CONSUMER_ENABLED
// Initialize DirectConsumer utilities for Kafka protocol
USM_DIRECT_CONSUMER_INIT(kafka, kafka_event_t, kafka_batch_events)
#endif

#endif
