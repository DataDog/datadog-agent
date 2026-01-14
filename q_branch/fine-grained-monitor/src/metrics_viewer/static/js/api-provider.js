/**
 * API Data Provider - HTTP fetch implementation.
 *
 * This provider fetches data from the metrics viewer HTTP API.
 * It's the default provider for live/cluster deployments.
 */

/**
 * Create an API-based data provider.
 * @param {string} [baseUrl=''] - Base URL for API requests (empty for same-origin)
 * @returns {DataProvider}
 */
export function createApiProvider(baseUrl = '') {
    return {
        name: 'ApiProvider',

        async getMetrics() {
            const res = await fetch(`${baseUrl}/api/metrics`);
            const data = await res.json();
            return data.metrics || [];
        },

        async getContainers(metricName, dashboard = null, timeRange = '1h') {
            const params = new URLSearchParams();
            params.set('metric', metricName);
            params.set('range', timeRange);

            // Apply dashboard container filters if present
            if (dashboard?.containers) {
                const { namespace, label_selector, name_pattern } = dashboard.containers;
                if (namespace) {
                    params.set('namespace', namespace);
                }
                if (label_selector && typeof label_selector === 'object') {
                    const labels = Object.entries(label_selector)
                        .map(([k, v]) => `${k}:${v}`)
                        .join(',');
                    if (labels) {
                        params.set('labels', labels);
                    }
                }
                if (name_pattern) {
                    params.set('search', name_pattern);
                }
            }

            const res = await fetch(`${baseUrl}/api/containers?${params.toString()}`);
            const data = await res.json();
            return data.containers || [];
        },

        async getTimeseries(metricName, containerIds, timeRange = '1h') {
            const params = new URLSearchParams();
            params.set('metric', metricName);
            params.set('containers', containerIds.join(','));
            params.set('range', timeRange);

            const res = await fetch(`${baseUrl}/api/timeseries?${params.toString()}`);
            const data = await res.json();

            // Transform array response to object keyed by container
            const result = {};
            for (const item of data) {
                result[item.container] = item.data;
            }
            return result;
        },

        async getStudy(studyType, metricName, containerId, timeRange = '1h') {
            const params = new URLSearchParams();
            params.set('metric', metricName);
            params.set('containers', containerId);
            params.set('range', timeRange);

            const res = await fetch(`${baseUrl}/api/study/${studyType}?${params.toString()}`);
            const data = await res.json();
            return data.results?.[0] || null;
        },

        async getInstance() {
            const res = await fetch(`${baseUrl}/api/instance`);
            return res.json();
        },
    };
}
