/**
 * Effect Executor - Handles side effects from state machine.
 *
 * Effects are descriptors returned by the reducer.
 * This module executes them: API calls, cache updates, UI renders.
 */

import * as DataStore from './data-store.js';
import { Effects } from './state-machine.js';

// ============================================================
// EFFECT HANDLERS REGISTRY
// ============================================================

const handlers = new Map();

/**
 * Register an effect handler.
 * @param {string} effectType
 * @param {Function} handler - async (effect, context) => void
 */
export function registerHandler(effectType, handler) {
    handlers.set(effectType, handler);
}

/**
 * Execute a list of effects.
 * @param {Array} effects - Effect descriptors from reducer
 * @param {Object} context - Shared context (dispatch, getState, UI callbacks)
 */
export async function executeEffects(effects, context) {
    for (const effect of effects) {
        const handler = handlers.get(effect.type);
        if (handler) {
            try {
                await handler(effect, context);
            } catch (err) {
                console.error(`Effect ${effect.type} failed:`, err);
            }
        } else {
            console.warn(`No handler for effect: ${effect.type}`);
        }
    }
}

// ============================================================
// API MODULE
// ============================================================

export const Api = {
    async fetchMetrics() {
        const res = await fetch('/api/metrics');
        const data = await res.json();
        return data.metrics || [];
    },

    async fetchContainers(metricName) {
        const res = await fetch(`/api/containers?metric=${encodeURIComponent(metricName)}`);
        const data = await res.json();
        return data.containers || [];
    },

    async fetchTimeseries(metricName, containerIds) {
        if (containerIds.length === 0) return {};

        const res = await fetch(
            `/api/timeseries?metric=${encodeURIComponent(metricName)}&containers=${containerIds.join(',')}`
        );
        const data = await res.json();

        // Transform array response to object keyed by container
        const result = {};
        for (const item of data) {
            result[item.container] = item.data;
        }
        return result;
    },

    async fetchStudy(studyType, metricName, containerId) {
        const res = await fetch(
            `/api/study/${studyType}?metric=${encodeURIComponent(metricName)}&containers=${containerId}`
        );
        const data = await res.json();
        return data.results?.[0] || null;
    },

    async fetchInstance() {
        const res = await fetch('/api/instance');
        return res.json();
    },
};

// ============================================================
// BUILT-IN EFFECT HANDLERS
// ============================================================

/**
 * Handle reference counting updates.
 */
registerHandler(Effects.UPDATE_REFS, (effect, context) => {
    let keysToEvict = [];

    if (effect.bulk) {
        // Bulk update for container selection change
        keysToEvict = DataStore.updateContainerSelectionRefs(
            effect.panels,
            effect.oldContainerIds,
            effect.newContainerIds
        );
    } else if (effect.remove) {
        // Panel being removed
        keysToEvict = DataStore.removePanelRefs(
            effect.panelId,
            effect.oldMetric,
            effect.containerIds,
            effect.studies || []
        );
    } else if (effect.removeStudy) {
        // Single study being removed
        const key = DataStore.studyKey(
            effect.removeStudy.metric,
            effect.removeStudy.containerId,
            effect.removeStudy.studyType
        );
        if (DataStore.removeRef(key, effect.panelId)) {
            keysToEvict.push(key);
        }
    } else {
        // Panel metric change
        keysToEvict = DataStore.updatePanelMetricRefs(
            effect.panelId,
            effect.oldMetric,
            effect.newMetric,
            effect.containerIds
        );

        // Also remove study refs if studies were cleared
        if (effect.studies) {
            for (const study of effect.studies) {
                const key = DataStore.studyKey(effect.oldMetric, study.containerId, study.type);
                if (DataStore.removeRef(key, effect.panelId)) {
                    keysToEvict.push(key);
                }
            }
        }
    }

    // Evict unreferenced data
    if (keysToEvict.length > 0) {
        DataStore.evictUnreferenced(keysToEvict);
    }
});

/**
 * Fetch timeseries data for a panel.
 */
registerHandler(Effects.FETCH_TIMESERIES, async (effect, context) => {
    const { panelId, metric, containerIds } = effect;
    const { dispatch, Actions } = context;

    if (!metric || containerIds.length === 0) {
        dispatch({ type: Actions.SET_PANEL_LOADING, panelId, loading: false });
        return;
    }

    // Check what's missing from cache
    const missing = DataStore.getMissingContainerIds(metric, containerIds);

    if (missing.length > 0) {
        try {
            const data = await Api.fetchTimeseries(metric, missing);

            // Store in cache
            for (const [cid, points] of Object.entries(data)) {
                const key = DataStore.timeseriesKey(metric, cid);
                DataStore.setTimeseries(key, points);
            }
        } catch (err) {
            console.error('Failed to fetch timeseries:', err);
        }
    }

    dispatch({ type: Actions.SET_PANEL_LOADING, panelId, loading: false });

    // Trigger panel render
    if (context.renderPanel) {
        context.renderPanel(panelId);
    }
});

/**
 * Fetch study results.
 */
registerHandler(Effects.FETCH_STUDY, async (effect, context) => {
    const { panelId, metric, containerId, studyType } = effect;
    const { dispatch, Actions } = context;

    const key = DataStore.studyKey(metric, containerId, studyType);

    // Check cache first
    if (!DataStore.hasStudyResult(key)) {
        try {
            const result = await Api.fetchStudy(studyType, metric, containerId);
            if (result) {
                DataStore.setStudyResult(key, result);
                DataStore.addRef(key, panelId);
            }
        } catch (err) {
            console.error('Failed to fetch study:', err);
        }
    } else {
        // Already cached, just add ref
        DataStore.addRef(key, panelId);
    }

    dispatch({
        type: Actions.SET_STUDY_LOADING,
        panelId,
        studyType,
        containerId,
        loading: false,
    });

    // Trigger panel render
    if (context.renderPanel) {
        context.renderPanel(panelId);
    }
});

/**
 * Fetch available metrics.
 */
registerHandler(Effects.FETCH_METRICS, async (effect, context) => {
    const { dispatch, Actions } = context;

    try {
        const metrics = await Api.fetchMetrics();
        dispatch({ type: Actions.SET_METRICS, metrics });
    } catch (err) {
        console.error('Failed to fetch metrics:', err);
    }
});

/**
 * Fetch containers for current metric.
 */
registerHandler(Effects.FETCH_CONTAINERS, async (effect, context) => {
    const { dispatch, Actions, getState } = context;

    const state = getState();
    const metric = effect.metric || state.panels[0]?.metric;

    if (!metric) return;

    try {
        const containers = await Api.fetchContainers(metric);
        dispatch({ type: Actions.SET_CONTAINERS, containers });
    } catch (err) {
        console.error('Failed to fetch containers:', err);
    }
});

/**
 * Render all panels.
 */
registerHandler(Effects.RENDER_PANELS, (effect, context) => {
    if (context.renderAllPanels) {
        context.renderAllPanels();
    }
});

/**
 * Render sidebar.
 */
registerHandler(Effects.RENDER_SIDEBAR, (effect, context) => {
    if (context.renderSidebar) {
        context.renderSidebar();
    }
});

/**
 * Sync time range across panels.
 */
registerHandler(Effects.SYNC_TIME_RANGE, (effect, context) => {
    if (context.syncTimeRange) {
        context.syncTimeRange(effect.reset);
    }
});

/**
 * Evict specific keys from cache.
 */
registerHandler(Effects.EVICT_KEYS, (effect, context) => {
    DataStore.evictUnreferenced(effect.keys);
});

// ============================================================
// DASHBOARD LOADING (REQ-MV-033, REQ-MV-034, REQ-MV-035, REQ-MV-036)
// ============================================================

/**
 * Parse URL parameters for dashboard configuration.
 * @returns {Object} {dashboardUrl, dashboardInline, templateVars}
 */
export function parseDashboardParams() {
    const params = new URLSearchParams(window.location.search);
    const dashboardUrl = params.get('dashboard');
    const dashboardInline = params.get('dashboard_inline');

    // Collect template variables (e.g., run_id -> {{RUN_ID}})
    const templateVars = {};
    for (const [key, value] of params.entries()) {
        if (key !== 'dashboard' && key !== 'dashboard_inline') {
            templateVars[key.toUpperCase()] = value;
        }
    }

    return { dashboardUrl, dashboardInline, templateVars };
}

/**
 * Substitute template variables in a string.
 * @param {string} str
 * @param {Object} vars - {VAR_NAME: value}
 * @returns {string}
 */
function substituteTemplateVars(str, vars) {
    if (typeof str !== 'string') return str;
    return str.replace(/\{\{(\w+)\}\}/g, (match, varName) => {
        return vars[varName] !== undefined ? vars[varName] : match;
    });
}

/**
 * Recursively substitute template variables in an object.
 * @param {any} obj
 * @param {Object} vars
 * @returns {any}
 */
function substituteTemplateVarsDeep(obj, vars) {
    if (typeof obj === 'string') {
        return substituteTemplateVars(obj, vars);
    }
    if (Array.isArray(obj)) {
        return obj.map(item => substituteTemplateVarsDeep(item, vars));
    }
    if (obj && typeof obj === 'object') {
        const result = {};
        for (const [key, value] of Object.entries(obj)) {
            result[key] = substituteTemplateVarsDeep(value, vars);
        }
        return result;
    }
    return obj;
}

/**
 * Load and parse a dashboard configuration.
 * @param {string|null} url - Dashboard URL to fetch
 * @param {string|null} inlineBase64 - Base64-encoded inline dashboard
 * @param {Object} templateVars - Template variables for substitution
 * @returns {Promise<Object|null>} Parsed dashboard config or null on error
 */
export async function loadDashboard(url, inlineBase64, templateVars = {}) {
    try {
        let dashboardJson;

        if (inlineBase64) {
            // REQ-MV-033: Decode inline base64 dashboard
            const decoded = atob(inlineBase64);
            dashboardJson = JSON.parse(decoded);
        } else if (url) {
            // REQ-MV-033: Fetch dashboard from URL
            // For relative paths (not starting with / or http), prepend /dashboards/
            const fetchUrl = (url.startsWith('/') || url.startsWith('http')) ? url : `/dashboards/${url}`;
            const res = await fetch(fetchUrl);
            if (!res.ok) {
                throw new Error(`Failed to fetch dashboard: ${res.status} ${res.statusText}`);
            }
            dashboardJson = await res.json();
        } else {
            return null;
        }

        // Validate schema version
        if (dashboardJson.schema_version !== 1) {
            console.warn(`Unknown dashboard schema version: ${dashboardJson.schema_version}`);
        }

        // Substitute template variables (REQ-MV-033)
        const dashboard = substituteTemplateVarsDeep(dashboardJson, templateVars);

        console.log('[Dashboard] Loaded:', dashboard.name || 'Untitled');
        return dashboard;
    } catch (err) {
        console.error('[Dashboard] Failed to load:', err);
        return null;
    }
}

/**
 * Build container filter params from dashboard config.
 * @param {Object} dashboard
 * @returns {URLSearchParams}
 */
export function buildContainerFilterParams(dashboard) {
    const params = new URLSearchParams();

    if (!dashboard?.containers) {
        return params;
    }

    const { namespace, label_selector, name_pattern } = dashboard.containers;

    // REQ-MV-034: Namespace filter
    if (namespace) {
        params.set('namespace', namespace);
    }

    // REQ-MV-034: Label selector filter (key:value pairs)
    if (label_selector && typeof label_selector === 'object') {
        const labels = Object.entries(label_selector)
            .map(([k, v]) => `${k}:${v}`)
            .join(',');
        if (labels) {
            params.set('labels', labels);
        }
    }

    // REQ-MV-034: Name pattern filter
    if (name_pattern) {
        params.set('search', name_pattern);
    }

    return params;
}

/**
 * Compute time range from container bounds.
 * @param {Array} containers - [{first_seen_ms, last_seen_ms}]
 * @param {Object} timeRangeConfig - {mode, padding_seconds}
 * @returns {{min: number, max: number}|null}
 */
export function computeTimeRangeFromContainers(containers, timeRangeConfig) {
    if (!timeRangeConfig || timeRangeConfig.mode !== 'from_containers') {
        return null;
    }

    // REQ-MV-035: Find min first_seen and max last_seen
    let minTime = Infinity;
    let maxTime = -Infinity;

    for (const c of containers) {
        // Try both naming conventions (ms suffix or not)
        const firstSeen = c.first_seen_ms ?? c.first_seen;
        const lastSeen = c.last_seen_ms ?? c.last_seen;

        if (firstSeen && firstSeen < minTime) {
            minTime = firstSeen;
        }
        if (lastSeen && lastSeen > maxTime) {
            maxTime = lastSeen;
        }
    }

    // Fall back to last hour if no valid bounds
    if (minTime === Infinity || maxTime === -Infinity) {
        const now = Date.now();
        return { min: now - 3600000, max: now };
    }

    // Apply padding (REQ-MV-035)
    const paddingMs = (timeRangeConfig.padding_seconds || 0) * 1000;
    return {
        min: minTime - paddingMs,
        max: maxTime + paddingMs,
    };
}
