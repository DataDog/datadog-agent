/**
 * Parquet Data Provider - parquet-wasm implementation.
 *
 * This provider reads data from parquet files using parquet-wasm.
 * Used for self-contained HTML snapshots that don't require a server.
 *
 * Dependencies (loaded via CDN):
 * - parquet-wasm: https://cdn.jsdelivr.net/npm/parquet-wasm@0.6.1/esm/parquet_wasm.js
 * - apache-arrow: https://cdn.jsdelivr.net/npm/apache-arrow@17.0.0/+esm
 */

// Column names in the parquet schema
const COLUMNS = {
    TIME: 'time',
    METRIC_NAME: 'metric_name',
    VALUE_INT: 'value_int',
    VALUE_FLOAT: 'value_float',
    CONTAINER_ID: 'l_container_id',
    CONTAINER_NAME: 'l_container_name',
    POD_NAME: 'l_pod_name',
    NAMESPACE: 'l_namespace',
    NODE_NAME: 'l_node_name',
    QOS_CLASS: 'l_qos_class',
};

/**
 * Create a parquet-based data provider.
 *
 * @param {Object} options
 * @param {ArrayBuffer|Uint8Array} options.parquetData - The parquet file data
 * @param {Object} [options.instanceInfo] - Optional instance metadata
 * @returns {Promise<DataProvider>}
 */
export async function createParquetProvider(options) {
    const { parquetData, instanceInfo = {} } = options;

    // Dynamically import parquet-wasm and arrow
    const [parquetWasm, arrow] = await Promise.all([
        import('https://cdn.jsdelivr.net/npm/parquet-wasm@0.6.1/esm/parquet_wasm.js'),
        import('https://cdn.jsdelivr.net/npm/apache-arrow@17.0.0/+esm'),
    ]);

    // Initialize parquet-wasm
    await parquetWasm.default();

    // Read the parquet file
    const uint8Data = parquetData instanceof Uint8Array
        ? parquetData
        : new Uint8Array(parquetData);

    const wasmTable = parquetWasm.readParquet(uint8Data);
    const ipcStream = wasmTable.intoIPCStream();
    const arrowTable = arrow.tableFromIPC(ipcStream);

    console.log('[ParquetProvider] Loaded table with', arrowTable.numRows, 'rows');

    // Build indexes for fast lookups
    const index = buildIndex(arrowTable);
    console.log('[ParquetProvider] Indexed', index.metrics.size, 'metrics,', index.containers.size, 'containers');

    return {
        name: 'ParquetProvider',

        async getMetrics() {
            // Return metric objects matching API format: {name: string}
            return Array.from(index.metrics).sort().map(name => ({ name }));
        },

        async getContainers(metricName, dashboard = null, timeRange = '1h') {
            const timeRangeMs = parseTimeRange(timeRange);
            const now = index.maxTime;
            const minTime = now - timeRangeMs;

            // Get containers that have this metric within the time range
            const containers = [];
            for (const [containerId, containerData] of index.containers.entries()) {
                // Check if container has this metric
                if (!containerData.metrics.has(metricName)) continue;

                // Check time range
                if (containerData.lastSeen < minTime) continue;

                // Apply dashboard filters
                if (dashboard?.containers) {
                    const { namespace, label_selector, name_pattern } = dashboard.containers;
                    if (namespace && containerData.namespace !== namespace) continue;
                    if (name_pattern) {
                        const pattern = new RegExp(name_pattern.replace(/\*/g, '.*'), 'i');
                        if (!pattern.test(containerData.podName) && !pattern.test(containerData.containerName)) {
                            continue;
                        }
                    }
                    // Note: label_selector not supported in parquet snapshots yet
                }

                containers.push({
                    short_id: containerId.slice(0, 12),
                    full_id: containerId,
                    container_name: containerData.containerName,
                    pod_name: containerData.podName,
                    namespace: containerData.namespace,
                    qos_class: containerData.qosClass,
                    first_seen_ms: containerData.firstSeen,
                    last_seen_ms: containerData.lastSeen,
                });
            }

            // Sort by last seen (most recent first)
            containers.sort((a, b) => b.last_seen_ms - a.last_seen_ms);
            return containers;
        },

        async getTimeseries(metricName, containerIds, timeRange = '1h') {
            const timeRangeMs = parseTimeRange(timeRange);
            const now = index.maxTime;
            const minTime = now - timeRangeMs;

            // Map short IDs to full IDs
            const shortToFull = new Map();
            for (const [fullId] of index.containers.entries()) {
                shortToFull.set(fullId.slice(0, 12), fullId);
            }

            const result = {};

            for (const shortId of containerIds) {
                const fullId = shortToFull.get(shortId);
                if (!fullId) continue;

                const points = [];
                const containerData = index.containers.get(fullId);
                if (!containerData) continue;

                const metricPoints = containerData.timeseries.get(metricName);
                if (!metricPoints) continue;

                for (const [ts, value] of metricPoints) {
                    if (ts >= minTime) {
                        // Return objects matching API format: {time_ms, value}
                        points.push({ time_ms: ts, value });
                    }
                }

                // Sort by timestamp
                points.sort((a, b) => a.time_ms - b.time_ms);
                result[shortId] = points;
            }

            return result;
        },

        async getStudy(studyType, metricName, containerId, timeRange = '1h') {
            // Studies require computation - not available in snapshot mode
            console.warn('[ParquetProvider] Studies not available in snapshot mode');
            return null;
        },

        async getInstance() {
            return {
                node: instanceInfo.node || 'snapshot',
                namespace: instanceInfo.namespace || 'snapshot',
            };
        },
    };
}

/**
 * Build an index from the Arrow table for fast lookups.
 * @param {arrow.Table} table
 * @returns {Object}
 */
function buildIndex(table) {
    const metrics = new Set();
    const containers = new Map();
    let minTime = Infinity;
    let maxTime = -Infinity;

    // Get column accessors
    const timeCol = table.getChild(COLUMNS.TIME);
    const metricCol = table.getChild(COLUMNS.METRIC_NAME);
    const valueIntCol = table.getChild(COLUMNS.VALUE_INT);
    const valueFloatCol = table.getChild(COLUMNS.VALUE_FLOAT);
    const containerIdCol = table.getChild(COLUMNS.CONTAINER_ID);
    const containerNameCol = table.getChild(COLUMNS.CONTAINER_NAME);
    const podNameCol = table.getChild(COLUMNS.POD_NAME);
    const namespaceCol = table.getChild(COLUMNS.NAMESPACE);
    const qosClassCol = table.getChild(COLUMNS.QOS_CLASS);

    const numRows = table.numRows;

    for (let i = 0; i < numRows; i++) {
        const timeValue = timeCol.get(i);
        const ts = typeof timeValue === 'bigint' ? Number(timeValue) : timeValue;
        const metric = metricCol.get(i);
        const containerId = containerIdCol.get(i);

        // Get value (prefer float, fallback to int)
        let value = valueFloatCol.get(i);
        if (value === null || value === undefined) {
            const intVal = valueIntCol.get(i);
            value = intVal !== null && intVal !== undefined
                ? (typeof intVal === 'bigint' ? Number(intVal) : intVal)
                : null;
        }

        if (value === null || value === undefined) continue;

        // Track metrics
        metrics.add(metric);

        // Track time bounds
        if (ts < minTime) minTime = ts;
        if (ts > maxTime) maxTime = ts;

        // Get or create container data
        let containerData = containers.get(containerId);
        if (!containerData) {
            containerData = {
                containerName: containerNameCol.get(i) || 'unknown',
                podName: podNameCol.get(i) || 'unknown',
                namespace: namespaceCol.get(i) || 'default',
                qosClass: qosClassCol.get(i) || 'BestEffort',
                firstSeen: ts,
                lastSeen: ts,
                metrics: new Set(),
                timeseries: new Map(),
            };
            containers.set(containerId, containerData);
        }

        // Update container time bounds
        if (ts < containerData.firstSeen) containerData.firstSeen = ts;
        if (ts > containerData.lastSeen) containerData.lastSeen = ts;

        // Track metric availability
        containerData.metrics.add(metric);

        // Store timeseries point
        let points = containerData.timeseries.get(metric);
        if (!points) {
            points = [];
            containerData.timeseries.set(metric, points);
        }
        points.push([ts, value]);
    }

    return { metrics, containers, minTime, maxTime };
}

/**
 * Parse time range string to milliseconds.
 * @param {string} range - e.g., '1h', '6h', '24h', '7d'
 * @returns {number} Duration in milliseconds
 */
function parseTimeRange(range) {
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

/**
 * Load a parquet file from a URL.
 * @param {string} url
 * @returns {Promise<ArrayBuffer>}
 */
export async function fetchParquetFile(url) {
    const response = await fetch(url);
    if (!response.ok) {
        throw new Error(`Failed to fetch parquet file: ${response.status}`);
    }
    return response.arrayBuffer();
}

/**
 * Load multiple parquet files and merge them.
 * @param {string[]} urls
 * @returns {Promise<ArrayBuffer>}
 */
export async function fetchAndMergeParquetFiles(urls) {
    // For now, just fetch the first one
    // TODO: Implement proper merging of multiple parquet files
    if (urls.length === 0) {
        throw new Error('No parquet URLs provided');
    }
    console.warn('[ParquetProvider] Multiple files not yet supported, using first file only');
    return fetchParquetFile(urls[0]);
}
