/**
 * Data Provider Interface - Abstraction for data sources.
 *
 * This module provides a unified interface for loading metrics data,
 * whether from a live API server or from embedded parquet files.
 *
 * Usage:
 *   import { Api, setProvider } from './data-provider.js';
 *
 *   // Default: uses ApiProvider (HTTP fetch)
 *   const metrics = await Api.fetchMetrics();
 *
 *   // Switch to parquet mode:
 *   import { createParquetProvider } from './parquet-provider.js';
 *   setProvider(createParquetProvider(parquetBlob));
 */

// ============================================================
// PROVIDER INTERFACE (duck typing)
// ============================================================

/**
 * Data Provider Interface.
 * Any provider must implement these async methods:
 *
 * @typedef {Object} DataProvider
 * @property {() => Promise<string[]>} getMetrics
 *   Returns list of available metric names.
 *
 * @property {(metric: string, filters: DashboardConfig|null, timeRange: string) => Promise<Container[]>} getContainers
 *   Returns containers that have data for the given metric.
 *   Filters come from dashboard config (namespace, label_selector, name_pattern).
 *
 * @property {(metric: string, containerIds: string[], timeRange: string) => Promise<Record<string, Array<[number, number]>>>} getTimeseries
 *   Returns timeseries data keyed by container ID.
 *   Each value is an array of [timestamp_ms, value] pairs.
 *
 * @property {(studyType: string, metric: string, containerId: string, timeRange: string) => Promise<StudyResult|null>} getStudy
 *   Returns study/analysis results for a container's metric data.
 *
 * @property {() => Promise<{node?: string, namespace?: string}>} getInstance
 *   Returns instance metadata (node name, namespace).
 */

// ============================================================
// PROVIDER MANAGEMENT
// ============================================================

let currentProvider = null;

/**
 * Set the active data provider.
 * @param {DataProvider} provider
 */
export function setProvider(provider) {
    currentProvider = provider;
    console.log('[DataProvider] Provider set:', provider.name || 'unnamed');
}

/**
 * Get the current data provider.
 * @returns {DataProvider}
 * @throws {Error} if no provider is set
 */
export function getProvider() {
    if (!currentProvider) {
        throw new Error('No data provider configured. Call setProvider() first.');
    }
    return currentProvider;
}

/**
 * Check if a provider is configured.
 * @returns {boolean}
 */
export function hasProvider() {
    return currentProvider !== null;
}

// ============================================================
// API FACADE
// ============================================================

/**
 * Unified API facade that delegates to the current provider.
 * This maintains the same interface as the original Api object
 * so effect handlers don't need to change.
 */
export const Api = {
    async fetchMetrics() {
        return getProvider().getMetrics();
    },

    async fetchContainers(metricName, dashboard = null, timeRange = '1h') {
        return getProvider().getContainers(metricName, dashboard, timeRange);
    },

    async fetchTimeseries(metricName, containerIds, timeRange = '1h') {
        if (containerIds.length === 0) return {};
        return getProvider().getTimeseries(metricName, containerIds, timeRange);
    },

    async fetchStudy(studyType, metricName, containerId, timeRange = '1h') {
        return getProvider().getStudy(studyType, metricName, containerId, timeRange);
    },

    async fetchInstance() {
        return getProvider().getInstance();
    },
};
