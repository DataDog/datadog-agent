/**
 * Effect Executor - Handles side effects from state machine.
 *
 * Effects are descriptors returned by the reducer.
 * This module executes them: API calls, cache updates, UI renders.
 */

import * as DataStore from './data-store.js';
import { Effects } from './state-machine.js';
import { Api, setProvider, hasProvider } from './data-provider.js';
import { createApiProvider } from './api-provider.js';

// Re-export Api for backwards compatibility
export { Api, setProvider };

// Initialize default provider (API-based)
if (!hasProvider()) {
    setProvider(createApiProvider());
}

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
            [] // No more studies array
        );

        // Remove study refs if panel had a study
        if (effect.oldStudy) {
            for (const cid of effect.containerIds) {
                const key = DataStore.studyKey(effect.oldMetric, cid, effect.oldStudy);
                if (DataStore.removeRef(key, effect.panelId)) {
                    keysToEvict.push(key);
                }
            }
        }
    } else if (effect.removeStudy) {
        // Study being removed from panel
        const { metric, studyType } = effect.removeStudy;
        const { getState } = context;
        const state = getState();

        for (const cid of state.selectedContainerIds) {
            const key = DataStore.studyKey(metric, cid, studyType);
            if (DataStore.removeRef(key, effect.panelId)) {
                keysToEvict.push(key);
            }
        }
    } else {
        // Panel metric change
        keysToEvict = DataStore.updatePanelMetricRefs(
            effect.panelId,
            effect.oldMetric,
            effect.newMetric,
            effect.containerIds
        );

        // Remove study refs if study was cleared on metric change
        if (effect.oldStudy) {
            for (const cid of effect.containerIds) {
                const key = DataStore.studyKey(effect.oldMetric, cid, effect.oldStudy);
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
 * REQ-MV-037: Uses dataTimeRange from state for API queries.
 */
registerHandler(Effects.FETCH_TIMESERIES, async (effect, context) => {
    const { panelId, metric, containerIds } = effect;
    const { dispatch, Actions, getState } = context;

    // REQ-MV-037: Get time range from state
    const state = getState();
    const timeRange = state.dataTimeRange || '1h';

    console.log('[FETCH_TIMESERIES] Starting:', { panelId, metric, containerCount: containerIds.length, timeRange });

    if (!metric || containerIds.length === 0) {
        console.log('[FETCH_TIMESERIES] Early exit - no metric or containers');
        dispatch({ type: Actions.SET_PANEL_LOADING, panelId, loading: false });
        return;
    }

    // Check what's missing from cache
    const missing = DataStore.getMissingContainerIds(metric, containerIds);
    console.log('[FETCH_TIMESERIES] Missing from cache:', missing.length, 'of', containerIds.length);

    if (missing.length > 0) {
        try {
            // REQ-MV-037: Pass time range to API
            const data = await Api.fetchTimeseries(metric, missing, timeRange);
            console.log('[FETCH_TIMESERIES] API returned keys:', Object.keys(data));
            console.log('[FETCH_TIMESERIES] First container ID from API:', Object.keys(data)[0]);
            console.log('[FETCH_TIMESERIES] First container ID expected:', missing[0]);

            // Store in cache
            for (const [cid, points] of Object.entries(data)) {
                const key = DataStore.timeseriesKey(metric, cid);
                console.log('[FETCH_TIMESERIES] Storing key:', key, 'points:', points.length);
                DataStore.setTimeseries(key, points);
            }
        } catch (err) {
            console.error('Failed to fetch timeseries:', err);
        }
    }

    console.log('[FETCH_TIMESERIES] DataStore stats after fetch:', DataStore.getStats());

    dispatch({ type: Actions.SET_PANEL_LOADING, panelId, loading: false });

    // Trigger panel render
    if (context.renderPanel) {
        const chartDiv = document.getElementById(`mainChart-${panelId}`);
        console.log('[FETCH_TIMESERIES] Calling renderPanel, chartDiv exists:', !!chartDiv);
        context.renderPanel(panelId);
    }
});

/**
 * Fetch study results for all containers on a panel.
 * REQ-MV-037: Uses dataTimeRange from state for API queries.
 */
registerHandler(Effects.FETCH_STUDY, async (effect, context) => {
    const { panelId, metric, containerIds, studyType } = effect;
    const { dispatch, Actions, getState } = context;

    // REQ-MV-037: Get time range from state
    const state = getState();
    const timeRange = state.dataTimeRange || '1h';

    console.log('[FETCH_STUDY] Starting:', { panelId, metric, studyType, containerCount: containerIds.length });

    // Fetch study for each container
    const fetchPromises = containerIds.map(async (containerId) => {
        const key = DataStore.studyKey(metric, containerId, studyType);

        // Check cache first
        if (!DataStore.hasStudyResult(key)) {
            try {
                // REQ-MV-037: Pass time range to API
                const result = await Api.fetchStudy(studyType, metric, containerId, timeRange);
                if (result) {
                    DataStore.setStudyResult(key, result);
                    DataStore.addRef(key, panelId);
                }
            } catch (err) {
                console.error(`Failed to fetch study for ${containerId}:`, err);
            }
        } else {
            // Already cached, just add ref
            DataStore.addRef(key, panelId);
        }
    });

    await Promise.all(fetchPromises);

    dispatch({
        type: Actions.SET_STUDY_LOADING,
        panelId,
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
 * REQ-MV-037: Uses dataTimeRange from state for API queries.
 * REQ-MV-039: Auto-deselects containers that don't exist in new time range.
 */
registerHandler(Effects.FETCH_CONTAINERS, async (effect, context) => {
    const { dispatch, Actions, getState } = context;

    const state = getState();
    const metric = effect.metric || state.panels[0]?.metric;
    const timeRange = state.dataTimeRange || '1h';

    if (!metric) return;

    try {
        // REQ-MV-034, REQ-MV-037: Pass dashboard config and time range for filtering
        const containers = await Api.fetchContainers(metric, state.dashboard, timeRange);
        dispatch({ type: Actions.SET_CONTAINERS, containers });

        // REQ-MV-039: Auto-deselect containers that no longer exist in the new range
        const availableIds = new Set(containers.map(c => c.short_id));
        const validSelection = state.selectedContainerIds.filter(id => availableIds.has(id));

        if (validSelection.length !== state.selectedContainerIds.length) {
            console.log('[FETCH_CONTAINERS] Auto-deselecting containers not in range:',
                state.selectedContainerIds.length - validSelection.length, 'removed');
            dispatch({ type: Actions.SET_SELECTED_CONTAINERS, containerIds: validSelection });
        } else if (validSelection.length > 0) {
            // Same containers, but we need to refetch timeseries for new range
            // Clear timeseries cache so data is refetched with new time range
            DataStore.clearAllTimeseries();

            // Directly dispatch force refresh - cleaner than working around SET_SELECTED_CONTAINERS deduplication
            dispatch({ type: Actions.FORCE_REFRESH_PANELS });
        }
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
            // But avoid double-prefixing if path already starts with dashboards/
            let fetchUrl = url;
            if (!url.startsWith('/') && !url.startsWith('http')) {
                fetchUrl = url.startsWith('dashboards/') ? `/${url}` : `/dashboards/${url}`;
            }
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
 * Compute time range from container bounds (derived from file timestamps).
 * @param {Array} containers - [{first_seen_ms, last_seen_ms}]
 * @param {Object} timeRangeConfig - {mode, padding_seconds}
 * @returns {{min: number, max: number}|null}
 */
export function computeTimeRangeFromContainers(containers, timeRangeConfig) {
    if (!timeRangeConfig || timeRangeConfig.mode !== 'from_containers') {
        return null;
    }

    // Find min first_seen and max last_seen from container file timestamps
    let minTime = Infinity;
    let maxTime = -Infinity;

    for (const c of containers) {
        const firstSeen = c.first_seen_ms;
        const lastSeen = c.last_seen_ms;

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

    // Apply padding
    const paddingMs = (timeRangeConfig.padding_seconds || 0) * 1000;
    return {
        min: minTime - paddingMs,
        max: maxTime + paddingMs,
    };
}
