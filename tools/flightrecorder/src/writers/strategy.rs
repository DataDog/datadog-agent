use std::sync::Arc;

use vortex::layout::layouts::buffered::BufferedStrategy;
use vortex::layout::layouts::chunked::writer::ChunkedLayoutStrategy;
use vortex::layout::layouts::collect::CollectStrategy;
use vortex::layout::layouts::compressed::CompressingStrategy;
use vortex::layout::layouts::dict::writer::DictStrategy;
use vortex::layout::layouts::flat::writer::FlatLayoutStrategy;
use vortex::layout::layouts::repartition::{RepartitionStrategy, RepartitionWriterOptions};
use vortex::layout::layouts::table::TableStrategy;
use vortex::layout::layouts::zoned::writer::{ZonedLayoutOptions, ZonedStrategy};
use vortex::layout::LayoutStrategy;

const ONE_MEG: u64 = 1 << 20;
const ROW_BLOCK_SIZE: usize = 8192;

/// Full Vortex pipeline with compression concurrency pinned to 1 and a smaller
/// buffer to minimize transient memory. Used for metric files that benefit from
/// dictionary encoding and compression.
pub fn low_memory_strategy() -> Arc<dyn LayoutStrategy> {
    let flat: Arc<dyn LayoutStrategy> = Arc::new(FlatLayoutStrategy::default());

    let chunked = ChunkedLayoutStrategy::new(flat.clone());
    let buffered = BufferedStrategy::new(chunked, ONE_MEG / 2);
    let compressing = CompressingStrategy::new_btrblocks(buffered, true).with_concurrency(1);

    let coalescing = RepartitionStrategy::new(
        compressing,
        RepartitionWriterOptions {
            block_size_minimum: ONE_MEG,
            block_len_multiple: ROW_BLOCK_SIZE,
            block_size_target: Some(ONE_MEG),
            canonicalize: true,
        },
    );

    let compress_then_flat =
        CompressingStrategy::new_btrblocks(flat, false).with_concurrency(1);

    let dict = DictStrategy::new(
        coalescing.clone(),
        compress_then_flat.clone(),
        coalescing,
        Default::default(),
    );

    let stats = ZonedStrategy::new(
        dict,
        compress_then_flat.clone(),
        ZonedLayoutOptions {
            block_size: ROW_BLOCK_SIZE,
            ..Default::default()
        },
    );

    let repartition = RepartitionStrategy::new(
        stats,
        RepartitionWriterOptions {
            block_size_minimum: 0,
            block_len_multiple: ROW_BLOCK_SIZE,
            block_size_target: None,
            canonicalize: false,
        },
    );

    let validity_strategy = CollectStrategy::new(compress_then_flat);
    let table_strategy = TableStrategy::new(Arc::new(validity_strategy), Arc::new(repartition));

    Arc::new(table_strategy)
}

/// Minimal Vortex pipeline with no compression, no dictionary strategy, no zone
/// maps. Just flat serialization wrapped in a table layout. Files will be larger
/// but flush is near-instant. Compression is deferred to the merge pass
/// (`compact_strategy`).
pub fn fast_flush_strategy() -> Arc<dyn LayoutStrategy> {
    let flat: Arc<dyn LayoutStrategy> = Arc::new(FlatLayoutStrategy::default());
    let table = TableStrategy::new(flat.clone(), flat);
    Arc::new(table)
}

/// Full Vortex pipeline tuned for background compaction. Compared to
/// `low_memory_strategy`, uses larger buffers and higher compression
/// concurrency — better compression ratio at the cost of more memory.
pub fn compact_strategy() -> Arc<dyn LayoutStrategy> {
    let flat: Arc<dyn LayoutStrategy> = Arc::new(FlatLayoutStrategy::default());

    let chunked = ChunkedLayoutStrategy::new(flat.clone());
    let buffered = BufferedStrategy::new(chunked, 2 * ONE_MEG);
    let compressing = CompressingStrategy::new_btrblocks(buffered, true).with_concurrency(4);

    let coalescing = RepartitionStrategy::new(
        compressing,
        RepartitionWriterOptions {
            block_size_minimum: 2 * ONE_MEG,
            block_len_multiple: ROW_BLOCK_SIZE,
            block_size_target: Some(2 * ONE_MEG),
            canonicalize: true,
        },
    );

    let compress_then_flat =
        CompressingStrategy::new_btrblocks(flat, false).with_concurrency(4);

    let dict = DictStrategy::new(
        coalescing.clone(),
        compress_then_flat.clone(),
        coalescing,
        Default::default(),
    );

    let stats = ZonedStrategy::new(
        dict,
        compress_then_flat.clone(),
        ZonedLayoutOptions {
            block_size: ROW_BLOCK_SIZE,
            ..Default::default()
        },
    );

    let repartition = RepartitionStrategy::new(
        stats,
        RepartitionWriterOptions {
            block_size_minimum: 0,
            block_len_multiple: ROW_BLOCK_SIZE,
            block_size_target: None,
            canonicalize: false,
        },
    );

    let validity_strategy = CollectStrategy::new(compress_then_flat);
    let table_strategy = TableStrategy::new(Arc::new(validity_strategy), Arc::new(repartition));

    Arc::new(table_strategy)
}

