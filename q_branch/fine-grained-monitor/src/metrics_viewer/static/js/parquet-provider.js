/**
 * Parquet Data Provider - parquet-wasm implementation.
 *
 * This provider reads data from embedded parquet files using parquet-wasm.
 * Used for self-contained HTML snapshots that don't require a server.
 *
 * TODO: Implement when we add the snapshot export feature.
 */

/**
 * Create a parquet-based data provider.
 * @param {ArrayBuffer|Uint8Array} parquetData - The parquet file data
 * @returns {DataProvider}
 */
export function createParquetProvider(parquetData) {
    // TODO: Initialize parquet-wasm
    // import { readParquet } from 'parquet-wasm';
    // const table = readParquet(new Uint8Array(parquetData));

    return {
        name: 'ParquetProvider',

        async getMetrics() {
            // TODO: Extract unique metric names from parquet data
            // The parquet schema has a 'metric' column we can get distinct values from
            throw new Error('ParquetProvider.getMetrics() not yet implemented');
        },

        async getContainers(metricName, dashboard = null, timeRange = '1h') {
            // TODO: Query parquet for containers matching metric and filters
            // Need to:
            // 1. Filter by metric name
            // 2. Apply dashboard filters (namespace, labels, name_pattern)
            // 3. Apply time range filter
            // 4. Return unique containers with metadata
            throw new Error('ParquetProvider.getContainers() not yet implemented');
        },

        async getTimeseries(metricName, containerIds, timeRange = '1h') {
            // TODO: Query parquet for timeseries data
            // Need to:
            // 1. Filter by metric name and container IDs
            // 2. Apply time range filter
            // 3. Return {containerId: [[timestamp, value], ...]}
            throw new Error('ParquetProvider.getTimeseries() not yet implemented');
        },

        async getStudy(studyType, metricName, containerId, timeRange = '1h') {
            // TODO: Studies require computation, not just data retrieval
            // Options:
            // 1. Pre-compute studies during export and embed results
            // 2. Run lightweight JS implementations of studies
            // 3. Return null (studies not available in snapshot mode)
            console.warn('ParquetProvider: Studies not available in snapshot mode');
            return null;
        },

        async getInstance() {
            // TODO: Return embedded instance metadata
            // This should be captured during export and embedded in the snapshot
            return {
                node: 'snapshot',
                namespace: 'snapshot',
            };
        },
    };
}

/**
 * Parse time range string to milliseconds.
 * @param {string} range - e.g., '1h', '6h', '24h', '7d'
 * @returns {number} Duration in milliseconds
 */
export function parseTimeRange(range) {
    const match = range.match(/^(\d+)([hmd])$/);
    if (!match) return 3600000; // default 1h

    const value = parseInt(match[1], 10);
    const unit = match[2];

    switch (unit) {
        case 'h': return value * 60 * 60 * 1000;
        case 'd': return value * 24 * 60 * 60 * 1000;
        case 'm': return value * 60 * 1000;
        default: return 3600000;
    }
}
