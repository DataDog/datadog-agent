/**
 * DataStore - Reference-counted cache for timeseries and study data.
 *
 * Data lives outside reactive state to avoid expensive cloning.
 * Reference counting enables automatic cleanup when panels are removed.
 *
 * Key format:
 *   Timeseries: `${metric}:${containerId}`
 *   Study: `${metric}:${containerId}:${studyType}`
 */

// Private storage - not exported, accessed via functions
const _timeseries = new Map();  // key -> [{time_ms, value}, ...]
const _studyResults = new Map(); // key -> study result object
const _refs = new Map();         // key -> Set<panelId>

// ============================================================
// KEY UTILITIES
// ============================================================

/**
 * Create a timeseries cache key.
 * @param {string} metric
 * @param {string} containerId
 * @returns {string}
 */
export function timeseriesKey(metric, containerId) {
    return `${metric}:${containerId}`;
}

/**
 * Create a study result cache key.
 * @param {string} metric
 * @param {string} containerId
 * @param {string} studyType
 * @returns {string}
 */
export function studyKey(metric, containerId, studyType) {
    return `${metric}:${containerId}:${studyType}`;
}

/**
 * Get all timeseries keys needed for a panel.
 * @param {string} metric
 * @param {string[]} containerIds
 * @returns {string[]}
 */
export function getTimeseriesKeysForPanel(metric, containerIds) {
    return containerIds.map(cid => timeseriesKey(metric, cid));
}

// ============================================================
// REFERENCE COUNTING
// ============================================================

/**
 * Add a reference from a panel to a cache key.
 * @param {string} key
 * @param {number|string} panelId
 */
export function addRef(key, panelId) {
    if (!_refs.has(key)) {
        _refs.set(key, new Set());
    }
    _refs.get(key).add(panelId);
}

/**
 * Remove a reference from a panel to a cache key.
 * @param {string} key
 * @param {number|string} panelId
 * @returns {boolean} True if this was the last reference (safe to evict)
 */
export function removeRef(key, panelId) {
    const refs = _refs.get(key);
    if (!refs) return true;

    refs.delete(panelId);
    if (refs.size === 0) {
        _refs.delete(key);
        return true; // Last reference removed
    }
    return false;
}

/**
 * Get current reference count for a key.
 * @param {string} key
 * @returns {number}
 */
export function getRefCount(key) {
    return _refs.get(key)?.size || 0;
}

/**
 * Check if a key has any references.
 * @param {string} key
 * @returns {boolean}
 */
export function hasRefs(key) {
    return getRefCount(key) > 0;
}

// ============================================================
// TIMESERIES DATA
// ============================================================

/**
 * Store timeseries data in the cache.
 * @param {string} key
 * @param {Array<{time_ms: number, value: number}>} data
 */
export function setTimeseries(key, data) {
    _timeseries.set(key, data);
}

/**
 * Get timeseries data from cache.
 * @param {string} key
 * @returns {Array<{time_ms: number, value: number}>|undefined}
 */
export function getTimeseries(key) {
    return _timeseries.get(key);
}

/**
 * Check if timeseries data exists in cache.
 * @param {string} key
 * @returns {boolean}
 */
export function hasTimeseries(key) {
    return _timeseries.has(key);
}

/**
 * Remove timeseries data from cache (only if no refs).
 * @param {string} key
 * @returns {boolean} True if evicted, false if still has refs
 */
export function evictTimeseries(key) {
    if (hasRefs(key)) {
        return false;
    }
    _timeseries.delete(key);
    return true;
}

/**
 * Get timeseries data for multiple containers.
 * @param {string} metric
 * @param {string[]} containerIds
 * @returns {Object<string, Array>} Map of containerId -> data
 */
export function getTimeseriesForContainers(metric, containerIds) {
    const result = {};
    for (const cid of containerIds) {
        const key = timeseriesKey(metric, cid);
        const data = getTimeseries(key);
        if (data) {
            result[cid] = data;
        }
    }
    return result;
}

/**
 * Find which container IDs are missing from cache for a metric.
 * @param {string} metric
 * @param {string[]} containerIds
 * @returns {string[]}
 */
export function getMissingContainerIds(metric, containerIds) {
    return containerIds.filter(cid => !hasTimeseries(timeseriesKey(metric, cid)));
}

// ============================================================
// STUDY RESULTS
// ============================================================

/**
 * Store study result in the cache.
 * @param {string} key
 * @param {Object} result
 */
export function setStudyResult(key, result) {
    _studyResults.set(key, result);
}

/**
 * Get study result from cache.
 * @param {string} key
 * @returns {Object|undefined}
 */
export function getStudyResult(key) {
    return _studyResults.get(key);
}

/**
 * Check if study result exists in cache.
 * @param {string} key
 * @returns {boolean}
 */
export function hasStudyResult(key) {
    return _studyResults.has(key);
}

/**
 * Remove study result from cache (only if no refs).
 * @param {string} key
 * @returns {boolean} True if evicted, false if still has refs
 */
export function evictStudyResult(key) {
    if (hasRefs(key)) {
        return false;
    }
    _studyResults.delete(key);
    return true;
}

// ============================================================
// BULK OPERATIONS
// ============================================================

/**
 * Update references when a panel changes metric.
 * Removes refs for old metric, adds refs for new metric.
 * @param {number|string} panelId
 * @param {string} oldMetric
 * @param {string} newMetric
 * @param {string[]} containerIds
 * @returns {string[]} Keys that are now safe to evict
 */
export function updatePanelMetricRefs(panelId, oldMetric, newMetric, containerIds) {
    const toEvict = [];

    // Remove old refs
    if (oldMetric) {
        for (const cid of containerIds) {
            const key = timeseriesKey(oldMetric, cid);
            if (removeRef(key, panelId)) {
                toEvict.push(key);
            }
        }
    }

    // Add new refs
    for (const cid of containerIds) {
        const key = timeseriesKey(newMetric, cid);
        addRef(key, panelId);
    }

    return toEvict;
}

/**
 * Update references when container selection changes.
 * @param {Array<{id: number|string, metric: string}>} panels
 * @param {string[]} oldContainerIds
 * @param {string[]} newContainerIds
 * @returns {string[]} Keys that are now safe to evict
 */
export function updateContainerSelectionRefs(panels, oldContainerIds, newContainerIds) {
    const toEvict = [];
    const removedIds = oldContainerIds.filter(id => !newContainerIds.includes(id));
    const addedIds = newContainerIds.filter(id => !oldContainerIds.includes(id));

    for (const panel of panels) {
        // Remove refs for removed containers
        for (const cid of removedIds) {
            const key = timeseriesKey(panel.metric, cid);
            if (removeRef(key, panel.id)) {
                toEvict.push(key);
            }
        }

        // Add refs for added containers
        for (const cid of addedIds) {
            const key = timeseriesKey(panel.metric, cid);
            addRef(key, panel.id);
        }
    }

    return toEvict;
}

/**
 * Remove all references for a panel (when panel is removed).
 * @param {number|string} panelId
 * @param {string} metric
 * @param {string[]} containerIds
 * @param {Array<{type: string, containerId: string}>} studies
 * @returns {string[]} Keys that are now safe to evict
 */
export function removePanelRefs(panelId, metric, containerIds, studies = []) {
    const toEvict = [];

    // Remove timeseries refs
    for (const cid of containerIds) {
        const key = timeseriesKey(metric, cid);
        if (removeRef(key, panelId)) {
            toEvict.push(key);
        }
    }

    // Remove study refs
    for (const study of studies) {
        const key = studyKey(metric, study.containerId, study.type);
        if (removeRef(key, panelId)) {
            toEvict.push(key);
        }
    }

    return toEvict;
}

/**
 * Evict all keys in the list that have no references.
 * @param {string[]} keys
 */
export function evictUnreferenced(keys) {
    for (const key of keys) {
        if (!hasRefs(key)) {
            _timeseries.delete(key);
            _studyResults.delete(key);
        }
    }
}

// ============================================================
// DEBUG / TESTING
// ============================================================

/**
 * Get cache statistics for debugging.
 * @returns {{timeseriesCount: number, studyCount: number, refCount: number}}
 */
export function getStats() {
    return {
        timeseriesCount: _timeseries.size,
        studyCount: _studyResults.size,
        refCount: _refs.size,
    };
}

/**
 * Clear all data (for testing).
 */
export function clear() {
    _timeseries.clear();
    _studyResults.clear();
    _refs.clear();
}

/**
 * Clear all timeseries data (for time range changes).
 * Keeps refs intact so panels will refetch.
 */
export function clearAllTimeseries() {
    _timeseries.clear();
}

/**
 * Get a snapshot of all refs (for testing/debugging).
 * @returns {Object<string, string[]>}
 */
export function getRefsSnapshot() {
    const snapshot = {};
    for (const [key, refs] of _refs.entries()) {
        snapshot[key] = Array.from(refs);
    }
    return snapshot;
}
