/*
 * C declarations for the stateful logs client, for cgo. Generated from
 * src/lib.rs by cbindgen; regenerate it with `make regenerate-go-header`
 * from the repository root.
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

/*
 Version of this ABI. It is 1. The library is unreleased, so a change to a
 declaration ships with the header and does not raise this number.

 The header is generated from the library's source, but the two are built
 separately, so a caller that does not check this can link a header
 describing a different ABI and find out at the first miscast field.
 */
#define FOLDSPACE_ABI_VERSION 1

/*
 The call succeeded.
 */
#define FOLDSPACE_OK 0

/*
 A required pointer was null.
 */
#define FOLDSPACE_ERR_NULL_POINTER 1

/*
 The sender id names no configured sender.
 */
#define FOLDSPACE_ERR_INVALID_SENDER 2

/*
 The content-encoding value names no encoding the library has.
 */
#define FOLDSPACE_ERR_UNKNOWN_ENCODING 3

/*
 A sender-class value names no delivery class the library has.
 */
#define FOLDSPACE_ERR_UNKNOWN_SENDER_CLASS 4

/*
 The timer-kind value names no timer the library schedules.
 */
#define FOLDSPACE_ERR_UNKNOWN_TIMER_KIND 5

/*
 A core needs at least one destination sender.
 */
#define FOLDSPACE_ERR_NO_SENDERS 10

/*
 At least one sender must be reliable, or nothing can go durable.
 */
#define FOLDSPACE_ERR_NO_RELIABLE_SENDERS 11

/*
 `batch_capacity` was zero, so a batch could hold no datum.
 */
#define FOLDSPACE_ERR_BATCH_CAPACITY_ZERO 12

/*
 `max_inflight_payloads` was zero, so no payload could be retained.
 */
#define FOLDSPACE_ERR_MAX_INFLIGHT_PAYLOADS_ZERO 13

/*
 `max_payload_bytes` was zero, so a payload could carry no datum.
 */
#define FOLDSPACE_ERR_MAX_PAYLOAD_BYTES_ZERO 14

/*
 Reconnect backoff needs a non-zero base interval.
 */
#define FOLDSPACE_ERR_RECONNECT_BACKOFF_BASE_ZERO 15

/*
 Reconnect backoff needs a factor of at least one.
 */
#define FOLDSPACE_ERR_RECONNECT_BACKOFF_FACTOR_ZERO 16

/*
 Reconnect backoff cap must be at least the base interval.
 */
#define FOLDSPACE_ERR_RECONNECT_BACKOFF_CAP_BELOW_BASE 17

/*
 More senders than a route mask can name, so some endpoint would be
 unaddressable.
 */
#define FOLDSPACE_ERR_TOO_MANY_SENDERS 18

/*
 `max_open_buffers` was zero, so no route could accumulate.
 */
#define FOLDSPACE_ERR_MAX_OPEN_BUFFERS_ZERO 19

/*
 `max_open_bytes` was zero, so no buffer could hold a datum.
 */
#define FOLDSPACE_ERR_MAX_OPEN_BYTES_ZERO 20

/*
 `max_open_bytes` is below what one buffer at its own ceiling holds.
 */
#define FOLDSPACE_ERR_MAX_OPEN_BYTES_BELOW_ONE_BUFFER 21

/*
 A config count does not fit this platform's `size_t`.
 */
#define FOLDSPACE_ERR_CONFIG_VALUE_OUT_OF_RANGE 24

/*
 `stream_lifetime` was zero, so every stream would rotate as it opened.
 */
#define FOLDSPACE_ERR_STREAM_LIFETIME_ZERO 25

/*
 `drain_timeout` was zero, so a rotating stream could settle nothing.
 */
#define FOLDSPACE_ERR_DRAIN_TIMEOUT_ZERO 26

/*
 `max_payload_bytes` exceeds the protocol's batch cap, so a full payload
 would be refused on every replay.
 */
#define FOLDSPACE_ERR_MAX_PAYLOAD_BYTES_ABOVE_PROTOCOL 27

/*
 Delivery class of one sender: one member of the reliable set, which the
 first member's acknowledgement discharges.
 */
#define FOLDSPACE_SENDER_RELIABLE_SET 0

/*
 Delivery class of one sender: best-effort, promising nothing.
 */
#define FOLDSPACE_SENDER_UNRELIABLE 1

/*
 Delivery class of one sender: its own promise, which its own
 acknowledgement is the only thing that keeps.
 */
#define FOLDSPACE_SENDER_RELIABLE 2

/*
 Body encoding a caller has not chosen: the library's own default applies.

 Zero is this rather than a named encoding so that a zeroed config, or one
 built field by field with this left out, cannot silently pick an encoding
 the library would not have picked.
 */
#define FOLDSPACE_ENCODING_DEFAULT 0

/*
 Body encoding applied to every batch: none, bodies travel unencoded.
 */
#define FOLDSPACE_ENCODING_IDENTITY 1

/*
 Body encoding applied to every batch: zstd at `zstd_level`.
 */
#define FOLDSPACE_ENCODING_ZSTD 2

/*
 Body encoding applied to every batch: the JSON split, each part its own
 zstd frame. `zstd_level` does not apply.
 */
#define FOLDSPACE_ENCODING_JSPLIT 3

/*
 The record and its metadata id belong to the library.
 */
#define FOLDSPACE_ADMISSION_ACCEPTED 1

/*
 The retained set is at its bound; offer the same record again later.
 */
#define FOLDSPACE_ADMISSION_REFUSED 2

/*
 No representation of the record fits a payload; it was not taken.
 */
#define FOLDSPACE_ADMISSION_TOO_LARGE 3

/*
 Admission is closed and the retained set is draining.
 */
#define FOLDSPACE_ADMISSION_SHUTTING_DOWN 4

/*
 The record has an empty body, which the wire cannot express; it was not
 taken.
 */
#define FOLDSPACE_ADMISSION_EMPTY 5

/*
 The record's route names an endpoint this client has no sender for; it was
 not taken, and offering it again unchanged cannot succeed.
 */
#define FOLDSPACE_ADMISSION_UNROUTABLE 6

/*
 The record's route names no endpoint at all, so there is nowhere to carry
 it; it was not taken, and offering it again unchanged cannot succeed.
 */
#define FOLDSPACE_ADMISSION_UNROUTED 7

/*
 The wire status of an acknowledged batch: the server accepted it.

 This is the protocol's value rather than this ABI's, and it is here so a
 caller reading a status off a response does not have to guess which
 number means accepted.
 */
#define FOLDSPACE_ACK_OK 1

/*
 The wire status of an acknowledged batch: anything else, which recycles
 the stream and, for a best-effort sender, sheds its backlog.
 */
#define FOLDSPACE_ACK_UNKNOWN 0

/*
 Open a stream for this sender after the effect's delay.
 */
#define FOLDSPACE_EFFECT_OPEN_STREAM 1

/*
 Write this batch to the stream it names.
 */
#define FOLDSPACE_EFFECT_SEND_BATCH 2

/*
 Close this sender's stream.
 */
#define FOLDSPACE_EFFECT_CLOSE_STREAM 3

/*
 Schedule a timer and feed it back.
 */
#define FOLDSPACE_EFFECT_SCHEDULE_TIMER 4

/*
 A protocol error against one sender.
 */
#define FOLDSPACE_EFFECT_REPORT_ERROR 5

/*
 Rotate an active stream.
 */
#define FOLDSPACE_TIMER_ROTATE_STREAM 1

/*
 Finish draining a stream.
 */
#define FOLDSPACE_TIMER_DRAIN_EXPIRED 2

/*
 The server acknowledged a batch id that was not expected.
 */
#define FOLDSPACE_CORE_ERROR_ACK_MISMATCH 1

/*
 The server acknowledged a batch when none was outstanding.
 */
#define FOLDSPACE_CORE_ERROR_ACK_WITHOUT_OUTSTANDING_BATCH 2

/*
 The server returned a non-OK status for a batch.
 */
#define FOLDSPACE_CORE_ERROR_BATCH_REJECTED 3

/*
 The caller reported a stream error.
 */
#define FOLDSPACE_CORE_ERROR_STREAM_FAILED 4

/*
 Resolution: every record named here belongs to a durable payload.
 */
#define FOLDSPACE_NOTIFICATION_PAYLOAD_DURABLE 1

/*
 Resolution: every record named here is gone, and the caller owns it again.
 */
#define FOLDSPACE_NOTIFICATION_PAYLOAD_ABANDONED 2

/*
 An opaque client handle.
 */
typedef struct foldspace_client foldspace_client;

/*
 What a `foldspace_effects` points into: the effects as read, and the
 effects they were read from, which own the error messages.
 */
typedef struct foldspace_effects_owner foldspace_effects_owner;

/*
 Bytes of one sealed batch, owned independently of the effect that named
 them. The caller reads them through the effect's `batch`; the lease is what
 keeps them alive.
 */
typedef struct foldspace_lease foldspace_lease;

/*
 What a `foldspace_notifications` points into: the notifications as read,
 and the notifications they were read from, which own the metadata ids.
 */
typedef struct foldspace_notifications_owner foldspace_notifications_owner;

/*
 An opaque tokenizer handle: the pattern extractor's compiled automaton.

 It holds no per-log state, so any number of clients can share one. Each
 client keeps its own reference, which is what lets the handle be freed as
 soon as the last client is built.
 */
typedef struct foldspace_tokenizer foldspace_tokenizer;

/*
 Bounds one of the client's local tables. Ignored unless `enabled` is
 non-zero.
 */
typedef struct {
  uint8_t enabled;
  int64_t max_memory_bytes;
  double high_watermark;
  double low_watermark;
  double age_decay_factor;
  uint64_t grace_period_nanos;
  /*
   Drops an item nothing has touched for this long, whatever the
   watermarks say. Zero leaves the watermarks as the only limit.
   */
  uint64_t stale_after_nanos;
} foldspace_eviction;

/*
 Core configuration. Endpoints, credentials, TLS, connect bounds, and window
 depth are the caller's and appear nowhere here.
 */
typedef struct {
  uint64_t max_inflight_payloads;
  uint64_t batch_capacity;
  uint64_t max_payload_bytes;
  /*
   One of the `FOLDSPACE_ENCODING_*` values. Any other is rejected.
   */
  int content_encoding;
  /*
   Applies when `content_encoding` is zstd. Zero asks zstd for its own
   default level.
   */
  int zstd_level;
  uint64_t reconnect_backoff_base_nanos;
  uint32_t reconnect_backoff_factor;
  uint64_t reconnect_backoff_cap_nanos;
  uint64_t drain_timeout_nanos;
  uint64_t stream_lifetime_nanos;
  foldspace_eviction dictionary_eviction;
  foldspace_eviction pattern_eviction;
  /*
   Sightings a new pattern shape spends in its stateless form before the
   next one admits it to the pattern table. Zero admits every shape on its
   first sighting.
   */
  uint32_t pattern_sightings_before_admission;
  /*
   The longest body the pattern extractor tokenizes; a longer one is sent
   raw. Zero asks for the library's default, so a zero-filled config
   patterns as the default does.
   */
  uint64_t max_patterned_body_bytes;
} foldspace_config;

/*
 A borrowed byte string. A null `ptr` means absent, which is distinct from
 present and empty.
 */
typedef struct {
  const uint8_t *ptr;
  size_t len;
} foldspace_str;

/*
 One log offered to the library. Every pointer is borrowed for the duration
 of the call and nothing here is retained.

 Text fields that are not valid UTF-8 are replaced character by character
 rather than rejected, so there is no encoding error in the status set.
 */
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

/*
 What a state-changing call left behind.

 `wake` names every sender the call gave work to, and may name senders that
 already held some, so a caller that polls each sender a call names reaches
 all of it. `has_capacity` and `notifications_ready` are levels, so a caller
 acting on one has not had to observe every prior call.

 `wake` is a caller-owned array: set `wake` and `wake_capacity` before the
 call and read `wake_len` after. An array of `foldspace_sender_count`
 entries can never truncate, because a sender appears in a wake set at most
 once; `wake_total` says what the call produced either way. A null progress
 ignores all of it.
 */
typedef struct {
  uint64_t *wake;
  size_t wake_capacity;
  size_t wake_len;
  size_t wake_total;
  uint8_t has_capacity;
  uint8_t notifications_ready;
} foldspace_progress;

/*
 What the library's buffer pool and rule store have been doing.

 An eviction seals a buffer early rather than full, which raises no error
 and refuses no record, so these counters are the only account of a client
 whose routes are displacing each other. The two eviction counters split by
 policy because the remedies differ: byte evictions mean `max_open_bytes`
 is too small for the workload's records, mask evictions mean more routes
 are live than `max_open_buffers` allows. Both are library-chosen through
 this ABI, so the counters read as a diagnosis of the workload rather than
 as a knob the caller turns.

 The rule counters account for the rules the library holds and what it has
 told each destination. `store_entries` is how many rules the store holds,
 counting patterns and dictionary entries together rather than either
 alone. `sent_entries` sums across destinations, so it is bounded by
 `store_entries` times the endpoint count. A `retired_entries` that never
 falls is a reference never given back, and `derive_fallbacks` counts
 widens sent whole because the destination could not resolve the base they
 diff against.

 The table figures report the pattern table and the dictionary apart.
 `*_entries` and `*_bytes` are the live table, with bytes measured the way
 the table's `max_memory_bytes` budget measures it. `*_candidates` and
 `*_candidate_bytes` are the values the admission gate is holding back, which
 sit outside the table and outside that budget; the bytes are estimated from
 the set's capacity, because a set emptied at its cap keeps its allocation.
 `*_candidate_bytes` also count the admission contest's frequency sketch and
 victim queue, and for the pattern table its churn meter, so a table with no
 gate reports zero candidates and still reports candidate bytes once its
 sketch is in use.
 */
typedef struct {
  uint64_t byte_evictions;
  uint64_t mask_evictions;
  uint64_t open_buffers;
  uint64_t open_bytes;
  uint64_t oldest_open_buffer_idle_pushes;
  uint64_t store_entries;
  uint64_t retired_entries;
  uint64_t sent_entries;
  uint64_t derive_fallbacks;
  uint64_t pattern_entries;
  uint64_t pattern_bytes;
  uint64_t pattern_candidates;
  uint64_t pattern_candidate_bytes;
  uint64_t dictionary_entries;
  uint64_t dictionary_bytes;
  uint64_t dictionary_candidates;
  uint64_t dictionary_candidate_bytes;
} foldspace_core_stats;

/*
 What one sender has delivered and given up on, counted from construction.

 Under fan-out the same records are counted once for every sender that
 carried them, so these measure the wire rather than the source. `sent_*`
 counts payloads the sender had acknowledged, whatever their durability, and
 `dropped_*` payloads it gave up on, which a peer may still have delivered.
 Bytes are encoded payload sizes.
 */
typedef struct {
  uint64_t sent_records;
  uint64_t sent_bytes;
  uint64_t dropped_records;
  uint64_t dropped_bytes;
} foldspace_sender_counts;

/*
 A protocol error the library reported against one sender.

 Which fields are meaningful follows from `kind`. `message` points into the
 effect list and is valid until the list is freed.
 */
typedef struct {
  int kind;
  uint32_t expected_batch_id;
  uint32_t actual_batch_id;
  uint32_t batch_id;
  int32_t batch_status;
  foldspace_str message;
} foldspace_core_error;

/*
 One effect, read in place from the list that carries it.

 Which fields are meaningful follows from `kind`, and the rest are zero.
 `lease` and `batch` are set only for a send: the lease is the caller's to
 free whether or not it writes the batch, and `batch` is the bytes it holds,
 valid until it is freed. `error` is set only for a reported error, and its
 message points into the list.
 */
typedef struct {
  int kind;
  uint64_t sender;
  /*
   The stream generation the effect names. Zero for a reported error,
   whose stream, when it has one, is inside its error.
   */
  uint64_t stream;
  /*
   How long to wait before acting: a dial's backoff, or a timer's delay.
   */
  uint64_t after_nanos;
  int timer_kind;
  /*
   The batch id to put on the wire with `batch`.
   */
  uint32_t batch_id;
  foldspace_lease *lease;
  foldspace_str batch;
  foldspace_core_error error;
} foldspace_effect;

/*
 A drained run of one sender's effects. `items` and `len` are the caller's
 to read; the whole struct goes back to `foldspace_effects_free`.
 */
typedef struct {
  const foldspace_effect *items;
  size_t len;
  foldspace_effects_owner *owner;
} foldspace_effects;

/*
 One resolution, read in place from the list that carries it. It names
 records and ends their life; `metadata_ids` points into the list.
 */
typedef struct {
  int kind;
  const uint64_t *metadata_ids;
  size_t metadata_ids_len;
} foldspace_notification;

/*
 A drained run of notifications. `items` and `len` are the caller's to
 read; the whole struct goes back to `foldspace_notifications_free`.
 */
typedef struct {
  const foldspace_notification *items;
  size_t len;
  foldspace_notifications_owner *owner;
} foldspace_notifications;

#ifdef __cplusplus
extern "C" {
#endif // __cplusplus

/*
 The version of this ABI the linked library implements.
 */
uint32_t foldspace_abi_version(void);

/*
 A static description of a status code. Never null, never freed.
 */
const char *foldspace_error_message(int code);

/*
 Writes the library's default configuration.

 Every field is derived from the library's own default, so a caller can
 take this, change the one field it cares about, and pass it back without
 having to know what the rest should be. A binding that restated these
 values instead would drift from them silently.

 Sender classes are not here: they describe the caller's endpoints, and the
 library has no default for how many of those exist.
 */
int foldspace_default_config(foldspace_config *out);

/*
 Builds a tokenizer to hand to `foldspace_client_new`.

 The automaton is large and slow to compile, so a caller building several
 clients builds one of these and passes it to each.
 */
int foldspace_tokenizer_new(foldspace_tokenizer **out);

/*
 Releases the caller's reference to a tokenizer. Clients built from it keep
 theirs; a null pointer is ignored.
 */
void foldspace_tokenizer_free(foldspace_tokenizer *tokenizer);

/*
 Creates a client. `classes` names every sender's delivery class in order,
 and `classes_len` is how many senders there are, so the two cannot
 disagree. It is required: a client with no destination has nothing to do.
 Sender id `i` names the endpoint at `classes[i]` for the life of the client.

 `tokenizer` is required too. The client takes its own reference, so the
 caller may free the handle once construction returns.
 */
int foldspace_client_new(const foldspace_config *config,
                         const foldspace_tokenizer *tokenizer,
                         const uint8_t *classes,
                         size_t classes_len,
                         foldspace_client **out);

/*
 Destroys a client. The caller must have stopped calling in on every region
 first; a null pointer is ignored.
 */
void foldspace_client_free(foldspace_client *client);

/*
 Number of senders this client was configured with.
 */
int foldspace_sender_count(const foldspace_client *client, uint64_t *out);

/*
 The token to stamp on every stream's `dd-content-encoding` header.

 The caller owns the transport and so sets the header, but the library owns
 the encoding, so only it knows the value. Reading it back is the only way
 to learn what a config that asked for the default resolved to.

 The string is NUL-terminated and lives as long as `client`; the caller does
 not free it. An absent header means the protocol's default encoding rather
 than no encoding, so there is always a token and it is always worth
 stamping.
 */
int foldspace_content_encoding(const foldspace_client *client, const char **out);

/*
 Whether a record can be offered without crossing the retained bound.
 */
int foldspace_has_capacity(const foldspace_client *client, uint8_t *out);

/*
 Offers one record to the endpoints `route` names. `now_nanos` is a reading
 from any monotonic source whose origin is fixed for the life of the
 client; only differences between readings are interpreted.
 `metadata_id` is opaque to the library and comes back in notifications.

 `route` is a bitset over sender ids. A bit at or above the sender count is
 a disagreement between the caller's routing policy and its endpoint
 configuration, and the record is refused as unroutable. No value means
 "all of them": that can only mean "however many are configured", which is
 the caller's knowledge.

 A parameter rather than a field on the record, because a route is where a
 record is bound rather than part of what it is, and the caller that knows
 the routing policy is not always the one that built the record.
 */
int foldspace_push_log(foldspace_client *client,
                       const foldspace_log_record *record,
                       uint64_t now_nanos,
                       uint64_t metadata_id,
                       uint64_t route,
                       int *admission,
                       foldspace_progress *progress);

/*
 Writes what the buffer pool has been doing. See `foldspace_core_stats`.
 */
int foldspace_stats(const foldspace_client *client, foldspace_core_stats *out);

/*
 Writes what one sender has delivered and given up on. See
 `foldspace_sender_counts`.
 */
int foldspace_sender_stats(const foldspace_client *client,
                           uint64_t sender_id,
                           foldspace_sender_counts *out);

/*
 Seals whatever is open, so an idle source's last partial batch goes out.
 */
int foldspace_flush(foldspace_client *client, int *admission, foldspace_progress *progress);

/*
 Closes admission and seals what is open, so the retained set can drain.
 */
int foldspace_begin_shutdown(foldspace_client *client,
                             int *admission,
                             foldspace_progress *progress);

/*
 Whether everything admitted has been resolved.
 */
int foldspace_is_drained(const foldspace_client *client, uint8_t *out);

/*
 Gives up on everything undelivered, reporting a final outcome for each
 record still seated. Idempotent.
 */
int foldspace_force_shutdown(foldspace_client *client, foldspace_progress *progress);

/*
 Drains one sender's effects into `out`, which the caller frees with
 `foldspace_effects_free`. Effects are produced only on seal and surface
 only here.
 */
int foldspace_poll_sender(foldspace_client *client, uint64_t sender_id, foldspace_effects *out);

/*
 Frees an effect list and empties `effects`. The leases it carried are the
 caller's and are unaffected.
 */
void foldspace_effects_free(foldspace_effects *effects);

/*
 Releases a lease.
 */
void foldspace_lease_free(foldspace_lease *lease);

/*
 Handles one sender's successful stream open.

 This and the other stream handlers are stream-qualified, so an event from
 a generation the library has already replaced is discarded rather than
 applied to its replacement.
 */
int foldspace_handle_stream_opened(foldspace_client *client,
                                   uint64_t sender_id,
                                   uint64_t stream_id,
                                   foldspace_progress *progress);

/*
 Handles an acknowledgement on one sender.
 */
int foldspace_handle_ack(foldspace_client *client,
                         uint64_t sender_id,
                         uint64_t stream_id,
                         uint32_t batch_id,
                         int32_t status,
                         foldspace_progress *progress);

/*
 Handles a sender's stream failure.
 */
int foldspace_handle_stream_error(foldspace_client *client,
                                  uint64_t sender_id,
                                  uint64_t stream_id,
                                  foldspace_str message,
                                  foldspace_progress *progress);

/*
 Handles a sender stream that cannot be opened and is not expected to become
 openable, such as an endpoint with no credential to dial with and none on
 its way. The sender gives up everything it holds, whatever its class, and
 drops what seals for it until one of its streams opens.
 */
int foldspace_handle_stream_abandoned(foldspace_client *client,
                                      uint64_t sender_id,
                                      uint64_t stream_id,
                                      foldspace_str message,
                                      foldspace_progress *progress);

/*
 Handles a sender's stream-qualified timer. `timer_kind` is one of the
 `FOLDSPACE_TIMER_*` values; any other is `FOLDSPACE_ERR_UNKNOWN_TIMER_KIND`,
 and the timer is not handled.
 */
int foldspace_handle_timer(foldspace_client *client,
                           uint64_t sender_id,
                           uint64_t stream_id,
                           int timer_kind,
                           foldspace_progress *progress);

/*
 Whether a notification is waiting, read without taking the library's lock.
 */
int foldspace_has_notifications(const foldspace_client *client, uint8_t *out);

/*
 Drains the notifications waiting into `out`, which the caller frees with
 `foldspace_notifications_free`.
 */
int foldspace_poll_notifications(foldspace_client *client, foldspace_notifications *out);

/*
 Frees a notification list and empties `notifications`, invalidating the
 metadata ids read from it.
 */
void foldspace_notifications_free(foldspace_notifications *notifications);

/*
 How far the readings passed to `foldspace_push_log` have advanced since
 this was last asked. Zero across a window in which records were admitted is
 a caller passing one cached timestamp forever, which freezes eviction and
 rotation.
 */
int foldspace_take_clock_advance_nanos(foldspace_client *client, uint64_t *out);

#ifdef __cplusplus
}  // extern "C"
#endif  // __cplusplus

#endif  /* FOLDSPACE_GO_H */
