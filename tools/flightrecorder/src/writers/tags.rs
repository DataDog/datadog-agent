/// Tag decomposition: splits tags into reserved per-key columns + overflow.
///
/// Reserved tags are stored as values only (e.g., `"web01"` not `"host:web01"`).
/// Non-reserved tags are pipe-joined into a single overflow string.

/// Reserved tag keys for metrics. Order matters — index into DecomposedTags::reserved.
pub const METRIC_RESERVED_KEYS: &[&str] =
    &["host", "device", "source", "service", "env", "version", "team"];

/// Reserved tag keys for logs. Logs already have hostname/source as dedicated
/// columns, so only these 4 keys get their own columns.
pub const LOG_RESERVED_KEYS: &[&str] = &["service", "env", "version", "team"];

pub struct DecomposedTags<const N: usize> {
    /// One value per reserved key, empty string if absent.
    pub reserved: [String; N],
    /// Pipe-joined non-reserved tags (full `key:value` form).
    pub overflow: String,
}

/// Decompose a FlatBuffers tag vector into metric reserved columns + overflow.
pub fn decompose_tags_for_metrics(
    tags: Option<flatbuffers::Vector<'_, flatbuffers::ForwardsUOffset<&str>>>,
) -> DecomposedTags<7> {
    decompose_inner(tags, METRIC_RESERVED_KEYS)
}

/// Decompose a FlatBuffers tag vector into log reserved columns + overflow.
/// Only extracts service/env/version/team. Everything else (including host,
/// device, source which are already dedicated log columns) goes to overflow.
pub fn decompose_tags_for_logs(
    tags: Option<flatbuffers::Vector<'_, flatbuffers::ForwardsUOffset<&str>>>,
) -> DecomposedTags<4> {
    decompose_inner(tags, LOG_RESERVED_KEYS)
}

fn decompose_inner<const N: usize>(
    tags: Option<flatbuffers::Vector<'_, flatbuffers::ForwardsUOffset<&str>>>,
    reserved_keys: &[&str],
) -> DecomposedTags<N> {
    debug_assert_eq!(reserved_keys.len(), N);

    let mut reserved = std::array::from_fn(|_| String::new());
    let mut overflow_parts: Vec<&str> = Vec::new();

    if let Some(tl) = tags {
        for j in 0..tl.len() {
            let tag = tl.get(j);
            if let Some(colon) = tag.find(':') {
                let key = &tag[..colon];
                let value = &tag[colon + 1..];
                if let Some(idx) = reserved_keys.iter().position(|&k| k == key) {
                    reserved[idx] = value.to_string();
                    continue;
                }
            }
            // Tags without `:` or with non-reserved keys go to overflow.
            overflow_parts.push(tag);
        }
    }

    DecomposedTags {
        reserved,
        overflow: overflow_parts.join("|"),
    }
}

/// Decompose a pre-joined tag string (pipe-separated) for metrics.
/// Used when tags come from the context map as a single string.
pub fn decompose_joined_tags_for_metrics(joined: &str) -> DecomposedTags<7> {
    decompose_joined_inner(joined, METRIC_RESERVED_KEYS)
}

fn decompose_joined_inner<const N: usize>(
    joined: &str,
    reserved_keys: &[&str],
) -> DecomposedTags<N> {
    debug_assert_eq!(reserved_keys.len(), N);

    let mut reserved = std::array::from_fn(|_| String::new());
    let mut overflow_parts: Vec<&str> = Vec::new();

    if !joined.is_empty() {
        for tag in joined.split('|') {
            if let Some(colon) = tag.find(':') {
                let key = &tag[..colon];
                let value = &tag[colon + 1..];
                if let Some(idx) = reserved_keys.iter().position(|&k| k == key) {
                    reserved[idx] = value.to_string();
                    continue;
                }
            }
            overflow_parts.push(tag);
        }
    }

    DecomposedTags {
        reserved,
        overflow: overflow_parts.join("|"),
    }
}

/// Decompose a FlatBuffers tag vector directly into interners, avoiding
/// intermediate String allocations. Used by LogsWriter where tags are
/// decomposed on every row (no context_key indirection).
///
/// `reserved_interners` must have exactly `N` elements, matching `reserved_keys`.
/// Returns the overflow string (pipe-joined non-reserved tags).
pub fn decompose_tags_into_interners(
    tags: Option<flatbuffers::Vector<'_, flatbuffers::ForwardsUOffset<&str>>>,
    reserved_keys: &[&str],
    reserved_interners: &mut [&mut super::intern::StringInterner],
    overflow_interner: &mut super::intern::StringInterner,
) {
    debug_assert_eq!(reserved_keys.len(), reserved_interners.len());

    if let Some(tl) = tags {
        // Track which reserved keys were found so we can intern "" for the rest.
        let n = reserved_keys.len();
        let mut found = vec![false; n];
        let mut overflow_parts: Vec<&str> = Vec::new();

        for j in 0..tl.len() {
            let tag = tl.get(j);
            if let Some(colon) = tag.find(':') {
                let key = &tag[..colon];
                let value = &tag[colon + 1..];
                if let Some(idx) = reserved_keys.iter().position(|&k| k == key) {
                    reserved_interners[idx].intern(value);
                    found[idx] = true;
                    continue;
                }
            }
            overflow_parts.push(tag);
        }

        // Intern "" for any reserved keys not found in this row.
        for (idx, was_found) in found.iter().enumerate() {
            if !was_found {
                reserved_interners[idx].intern("");
            }
        }

        // Build overflow: join and intern.
        if overflow_parts.is_empty() {
            overflow_interner.intern("");
        } else {
            // We need a temporary String for the join, but it's one allocation
            // per row instead of N+1 (DecomposedTags was 4 Strings + 1 join).
            let joined = overflow_parts.join("|");
            overflow_interner.intern_owned(joined);
        }
    } else {
        // No tags at all — intern "" for everything.
        for interner in reserved_interners.iter_mut() {
            interner.intern("");
        }
        overflow_interner.intern("");
    }
}

/// Decompose a pre-joined tag string (pipe-separated) directly into interners.
/// Used by MetricsWriter to avoid storing DecomposedTags in the context map.
/// The joined string is borrowed from the context map — no allocation for the
/// reserved values (they're interned as &str slices into the joined string).
pub fn decompose_joined_into_interners(
    joined: &str,
    reserved_keys: &[&str],
    reserved_interners: &mut [&mut super::intern::StringInterner],
    overflow_interner: &mut super::intern::StringInterner,
) {
    debug_assert_eq!(reserved_keys.len(), reserved_interners.len());

    if joined.is_empty() {
        for interner in reserved_interners.iter_mut() {
            interner.intern("");
        }
        overflow_interner.intern("");
        return;
    }

    let n = reserved_keys.len();
    let mut found = vec![false; n];
    let mut overflow_parts: Vec<&str> = Vec::new();

    for tag in joined.split('|') {
        if let Some(colon) = tag.find(':') {
            let key = &tag[..colon];
            let value = &tag[colon + 1..];
            if let Some(idx) = reserved_keys.iter().position(|&k| k == key) {
                reserved_interners[idx].intern(value);
                found[idx] = true;
                continue;
            }
        }
        overflow_parts.push(tag);
    }

    for (idx, was_found) in found.iter().enumerate() {
        if !was_found {
            reserved_interners[idx].intern("");
        }
    }

    if overflow_parts.is_empty() {
        overflow_interner.intern("");
    } else {
        let joined = overflow_parts.join("|");
        overflow_interner.intern_owned(joined);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_decompose_joined_metrics_basic() {
        let d = decompose_joined_tags_for_metrics("host:web01|env:prod|service:api|custom:foo");
        assert_eq!(d.reserved[0], "web01"); // host
        assert_eq!(d.reserved[1], ""); // device
        assert_eq!(d.reserved[2], ""); // source
        assert_eq!(d.reserved[3], "api"); // service
        assert_eq!(d.reserved[4], "prod"); // env
        assert_eq!(d.reserved[5], ""); // version
        assert_eq!(d.reserved[6], ""); // team
        assert_eq!(d.overflow, "custom:foo");
    }

    #[test]
    fn test_decompose_joined_all_reserved() {
        let d = decompose_joined_tags_for_metrics(
            "host:h|device:d|source:s|service:svc|env:e|version:v|team:t",
        );
        assert_eq!(d.reserved, ["h", "d", "s", "svc", "e", "v", "t"]);
        assert_eq!(d.overflow, "");
    }

    #[test]
    fn test_decompose_joined_empty() {
        let d = decompose_joined_tags_for_metrics("");
        assert_eq!(d.reserved, ["", "", "", "", "", "", ""]);
        assert_eq!(d.overflow, "");
    }

    #[test]
    fn test_decompose_joined_no_colon_goes_to_overflow() {
        let d = decompose_joined_tags_for_metrics("host:web01|bare_tag|env:prod");
        assert_eq!(d.reserved[0], "web01");
        assert_eq!(d.reserved[4], "prod");
        assert_eq!(d.overflow, "bare_tag");
    }

    #[test]
    fn test_decompose_joined_multiple_overflow() {
        let d = decompose_joined_tags_for_metrics("custom1:a|host:h|custom2:b");
        assert_eq!(d.reserved[0], "h");
        assert_eq!(d.overflow, "custom1:a|custom2:b");
    }

    #[test]
    fn test_decompose_joined_logs() {
        let d = decompose_joined_inner::<4>(
            "host:h|service:svc|env:prod|version:v1|team:ops|custom:x",
            LOG_RESERVED_KEYS,
        );
        assert_eq!(d.reserved[0], "svc"); // service
        assert_eq!(d.reserved[1], "prod"); // env
        assert_eq!(d.reserved[2], "v1"); // version
        assert_eq!(d.reserved[3], "ops"); // team
        // host and custom go to overflow for logs
        assert_eq!(d.overflow, "host:h|custom:x");
    }
}
