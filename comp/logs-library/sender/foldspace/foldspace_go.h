/*
 * C declarations for the stateful logs client, for cgo.
 *
 * This header is a Go-specific artifact rather than a packaging of the ABI:
 * cgo needs a declaration to compile against, while bindings/java binds by
 * symbol name and ships no header at all.
 *
 * The library owns admission, dispatch, durability, and stream lifecycle. The
 * caller owns endpoints, sockets, threads, timers, and scheduling, and drives
 * the library by executing the effects it hands back.
 *
 * Check foldspace_abi_version() against FOLDSPACE_ABI_VERSION at startup: this
 * header and the shared library are built separately, so a mismatch is
 * otherwise silent until a field is read at the wrong offset.
 */

#ifndef FOLDSPACE_GO_H
#define FOLDSPACE_GO_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

#define FOLDSPACE_ABI_VERSION 1

/* Status codes. The whole invocation error surface, and none of it carries a
 * value, which is why these are codes rather than error objects. */
enum foldspace_status {
  FOLDSPACE_OK = 0,
  FOLDSPACE_ERR_NULL_POINTER = 1,
  FOLDSPACE_ERR_INVALID_SENDER = 2,
  FOLDSPACE_ERR_NO_SENDERS = 10,
  FOLDSPACE_ERR_SENDER_CLASS_COUNT_MISMATCH = 11,
  FOLDSPACE_ERR_NO_RELIABLE_SENDERS = 12,
  FOLDSPACE_ERR_BATCH_CAPACITY_ZERO = 13,
  FOLDSPACE_ERR_MAX_INFLIGHT_PAYLOADS_ZERO = 14,
  FOLDSPACE_ERR_MAX_PAYLOAD_BYTES_ZERO = 15,
  FOLDSPACE_ERR_RECONNECT_BACKOFF_BASE_ZERO = 16,
  FOLDSPACE_ERR_RECONNECT_BACKOFF_FACTOR_ZERO = 17,
  FOLDSPACE_ERR_RECONNECT_BACKOFF_CAP_BELOW_BASE = 18
};

/* Delivery class of one sender, passed as an ordered array at construction.
 * SenderId(i) names endpoint i for the life of the client.
 *
 * A reliable set holds one promise between its members, so the first member to
 * acknowledge a payload discharges the rest of them. A reliable sender holds
 * its own promise instead: its acknowledgement is required for durability and
 * it never discards a payload, whatever the other endpoints do. */
enum foldspace_sender_class {
  FOLDSPACE_SENDER_RELIABLE_SET = 0,
  FOLDSPACE_SENDER_UNRELIABLE = 1,
  FOLDSPACE_SENDER_RELIABLE = 2
};

/* Encoding applied to every batch body. */
enum foldspace_encoding {
  FOLDSPACE_ENCODING_IDENTITY = 0,
  FOLDSPACE_ENCODING_ZSTD = 1
};

/* What the library did with an offered record, or with a flush. */
enum foldspace_admission {
  FOLDSPACE_ADMISSION_ACCEPTED = 1,
  FOLDSPACE_ADMISSION_REFUSED = 2,
  FOLDSPACE_ADMISSION_TOO_LARGE = 3,
  FOLDSPACE_ADMISSION_SHUTTING_DOWN = 4
};

/* Wire status of an acknowledged batch. The protocol's values, repeated here
 * so a caller reading a status off a response need not guess. */
enum foldspace_ack_status {
  FOLDSPACE_ACK_UNKNOWN = 0,
  FOLDSPACE_ACK_OK = 1
};

/* What the caller must do. */
enum foldspace_effect_kind {
  FOLDSPACE_EFFECT_OPEN_STREAM = 1,
  FOLDSPACE_EFFECT_SEND_BATCH = 2,
  FOLDSPACE_EFFECT_CLOSE_STREAM = 3,
  FOLDSPACE_EFFECT_SCHEDULE_TIMER = 4,
  FOLDSPACE_EFFECT_REPORT_ERROR = 5
};

enum foldspace_timer_kind {
  FOLDSPACE_TIMER_ROTATE_STREAM = 1,
  FOLDSPACE_TIMER_DRAIN_EXPIRED = 2
};

/* The one error type with payloads. It never returns from a call; it travels
 * out inside a FOLDSPACE_EFFECT_REPORT_ERROR effect. */
enum foldspace_core_error_kind {
  FOLDSPACE_CORE_ERROR_ACK_MISMATCH = 1,
  FOLDSPACE_CORE_ERROR_ACK_WITHOUT_OUTSTANDING_BATCH = 2,
  FOLDSPACE_CORE_ERROR_BATCH_REJECTED = 3,
  FOLDSPACE_CORE_ERROR_STREAM_FAILED = 4
};

enum foldspace_notification_kind {
  FOLDSPACE_NOTIFICATION_PAYLOAD_DURABLE = 1,
  FOLDSPACE_NOTIFICATION_PAYLOAD_DROPPED = 2
};

/* A borrowed byte string. A null ptr means absent, which is distinct from
 * present and empty. Text that is not valid UTF-8 is replaced rather than
 * rejected, so there is no encoding status. */
typedef struct {
  const uint8_t *ptr;
  size_t len;
} foldspace_str;

/* Bounds one of the client's local tables. Ignored unless enabled is non-zero.
 *
 * stale_after_nanos drops an item nothing has touched for that long, whatever
 * the watermarks say; zero leaves the watermarks as the only limit. */
typedef struct {
  uint8_t enabled;
  uint64_t max_item_count;
  int64_t max_memory_bytes;
  double high_watermark;
  double low_watermark;
  double age_decay_factor;
  uint64_t grace_period_nanos;
  uint64_t stale_after_nanos;
} foldspace_eviction;

/* Core configuration. Endpoint addresses, credentials, TLS, flow control,
 * connect bounds, and window depth are the caller's and appear nowhere. */
typedef struct {
  uint64_t senders;
  uint64_t max_inflight_payloads;
  uint64_t batch_capacity;
  uint64_t max_payload_bytes;
  uint64_t coalesce_threshold_bytes;
  int content_encoding;
  int zstd_level;
  uint64_t reconnect_backoff_base_nanos;
  uint32_t reconnect_backoff_factor;
  uint64_t reconnect_backoff_cap_nanos;
  uint64_t drain_timeout_nanos;
  uint64_t stream_lifetime_nanos;
  uint32_t first_payload_batch_id;
  uint32_t snapshot_batch_id;
  foldspace_eviction dictionary_eviction;
  foldspace_eviction pattern_eviction;
} foldspace_config;

/* One log offered to the library. Every pointer is borrowed for the duration
 * of the call. */
typedef struct {
  foldspace_str body;
  int64_t timestamp_millis;
  foldspace_str service;
  foldspace_str status;
  foldspace_str source;
  foldspace_str hostname;
  foldspace_str uuid;
  const foldspace_str *tags;
  size_t tags_len;
  const foldspace_str *processing_tags;
  size_t processing_tags_len;
} foldspace_log_record;

/* What a state-changing call left behind. Every field is a level rather than
 * an edge, so a caller acting on one has not had to observe every prior call.
 *
 * wake is a caller-owned array: set wake and wake_capacity before the call and
 * read wake_len after. A capacity of foldspace_sender_count() can never
 * truncate, since a sender appears at most once; wake_total says what the call
 * produced either way. Pass a null progress to ignore all of it. */
typedef struct {
  uint64_t *wake;
  size_t wake_capacity;
  size_t wake_len;
  size_t wake_total;
  uint8_t has_capacity;
  uint8_t notifications_ready;
} foldspace_progress;

/* A protocol error reported against one sender. Which fields are meaningful
 * follows from kind; message points into the effect and dies with it. */
typedef struct {
  int kind;
  uint32_t expected_batch_id;
  uint32_t actual_batch_id;
  uint32_t batch_id;
  int32_t batch_status;
  foldspace_str message;
} foldspace_core_error;

typedef struct foldspace_client foldspace_client;
typedef struct foldspace_effects foldspace_effects;
typedef struct foldspace_effect foldspace_effect;
typedef struct foldspace_lease foldspace_lease;
typedef struct foldspace_notifications foldspace_notifications;
typedef struct foldspace_notification foldspace_notification;

uint32_t foldspace_abi_version(void);
const char *foldspace_error_message(int code);

/* classes names every sender's class in order, or is NULL to make every sender
 * a member of one reliable set. */
int foldspace_client_new(const foldspace_config *config, const uint8_t *classes,
                         size_t classes_len, foldspace_client **out);
void foldspace_client_free(foldspace_client *client);
int foldspace_sender_count(const foldspace_client *client, uint64_t *out);

/* Requests one stream per sender. Without it no sender ever dials. */
int foldspace_start(foldspace_client *client, foldspace_progress *progress);
int foldspace_has_capacity(const foldspace_client *client, uint8_t *out);

/* now_nanos is a reading from any monotonic source whose origin is fixed for
 * the life of the client; only differences between readings are interpreted.
 * metadata_id is opaque to the library and comes back in notifications. */
int foldspace_push_log(foldspace_client *client, const foldspace_log_record *record,
                       uint64_t now_nanos, uint64_t metadata_id, int *admission,
                       foldspace_progress *progress);
int foldspace_flush(foldspace_client *client, int *admission, foldspace_progress *progress);

int foldspace_begin_shutdown(foldspace_client *client, int *admission,
                             foldspace_progress *progress);
int foldspace_is_drained(const foldspace_client *client, uint8_t *out);
int foldspace_force_shutdown(foldspace_client *client, foldspace_progress *progress);

/* Effects are produced only on seal and surface only here. */
int foldspace_poll_sender(foldspace_client *client, uint64_t sender_id,
                          foldspace_effects **out);
uint64_t foldspace_effects_len(const foldspace_effects *effects);
/* Takes the next effect, or NULL when exhausted. The effect outlives the
 * list and is freed separately. */
foldspace_effect *foldspace_effects_next(foldspace_effects *effects);
void foldspace_effects_free(foldspace_effects *effects);
void foldspace_effect_free(foldspace_effect *effect);

int foldspace_effect_kind(const foldspace_effect *effect);
uint64_t foldspace_effect_sender(const foldspace_effect *effect);
uint64_t foldspace_effect_stream(const foldspace_effect *effect);
uint64_t foldspace_effect_after_nanos(const foldspace_effect *effect);
int foldspace_effect_timer_kind(const foldspace_effect *effect);
uint32_t foldspace_effect_batch_id(const foldspace_effect *effect);
/* A lease is independent of the effect that carried it: freeing the effect
 * does not free the bytes, so a sender writing asynchronously outlives it and
 * N senders can hold one payload at once. */
foldspace_lease *foldspace_effect_lease(const foldspace_effect *effect);
int foldspace_effect_error(const foldspace_effect *effect, foldspace_core_error *out);

const uint8_t *foldspace_lease_bytes(const foldspace_lease *lease);
uint64_t foldspace_lease_len(const foldspace_lease *lease);
void foldspace_lease_free(foldspace_lease *lease);

/* Feedback is stream-qualified, so an event from a generation the library has
 * already replaced is discarded rather than applied to its replacement. */
int foldspace_handle_stream_opened(foldspace_client *client, uint64_t sender_id,
                                   uint64_t stream_id, foldspace_progress *progress);
int foldspace_handle_ack(foldspace_client *client, uint64_t sender_id, uint64_t stream_id,
                         uint32_t batch_id, int32_t status, foldspace_progress *progress);
int foldspace_handle_stream_error(foldspace_client *client, uint64_t sender_id,
                                  uint64_t stream_id, foldspace_str message,
                                  foldspace_progress *progress);
int foldspace_handle_timer(foldspace_client *client, uint64_t sender_id, uint64_t stream_id,
                           int timer_kind, foldspace_progress *progress);

int foldspace_has_notifications(const foldspace_client *client, uint8_t *out);
int foldspace_poll_notifications(foldspace_client *client, foldspace_notifications **out);
uint64_t foldspace_notifications_len(const foldspace_notifications *notifications);
foldspace_notification *foldspace_notifications_next(foldspace_notifications *notifications);
void foldspace_notifications_free(foldspace_notifications *notifications);
void foldspace_notification_free(foldspace_notification *notification);

int foldspace_notification_kind(const foldspace_notification *notification);
uint8_t foldspace_notification_abandoned(const foldspace_notification *notification);
/* Returns 1 and writes out when a sender is named, 0 otherwise. */
uint8_t foldspace_notification_sender(const foldspace_notification *notification, uint64_t *out);
/* Valid until the notification is freed. */
const uint64_t *foldspace_notification_metadata_ids(const foldspace_notification *notification,
                                                    size_t *len);

int foldspace_changelog_byte_size(const foldspace_client *client, uint64_t *out);
int foldspace_changelog_entry_count(const foldspace_client *client, uint64_t *out);
/* Zero advancement across a window in which records were admitted is a caller
 * passing one cached timestamp forever, which freezes eviction and rotation. */
int foldspace_take_clock_advance_nanos(foldspace_client *client, uint64_t *out);

#ifdef __cplusplus
}
#endif

#endif /* FOLDSPACE_GO_H */
