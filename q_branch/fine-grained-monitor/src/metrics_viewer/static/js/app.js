/**
 * App - Main entry point that wires everything together.
 *
 * Creates the store, sets up UI callbacks, initializes the application.
 */

import * as DataStore from './data-store.js';
import { Actions, Effects, initialState, reduce, canAddPanel, canRemovePanel } from './state-machine.js';
import {
    executeEffects,
    Api,
    registerHandler,
    parseDashboardParams,
    loadDashboard,
    buildContainerFilterParams,
    computeTimeRangeFromContainers,
} from './effects.js';

// ============================================================
// STORE
// ============================================================

let state = { ...initialState };
const listeners = new Set();

/**
 * Get current state (readonly).
 */
export function getState() {
    return state;
}

/**
 * Subscribe to state changes.
 * @param {Function} listener
 * @returns {Function} Unsubscribe function
 */
export function subscribe(listener) {
    listeners.add(listener);
    return () => listeners.delete(listener);
}

/**
 * Dispatch an action to update state.
 * @param {Object} action
 */
export function dispatch(action) {
    console.log('[dispatch] Action:', action.type);
    const result = reduce(state, action);
    const prevState = state;
    state = result.state;

    // Notify listeners
    if (state !== prevState) {
        for (const listener of listeners) {
            try {
                listener(state, prevState);
            } catch (err) {
                console.error('Listener error:', err);
            }
        }
    }

    // Execute effects
    if (result.effects.length > 0) {
        console.log('[dispatch] Effects to execute:', result.effects.map(e => e.type));
        executeEffects(result.effects, effectContext);
    }
}

// ============================================================
// EFFECT CONTEXT
// ============================================================

// UI callbacks - set by UI module
let uiCallbacks = {
    renderPanel: null,
    renderAllPanels: null,
    renderSidebar: null,
    syncTimeRange: null,
};

const effectContext = {
    dispatch,
    getState,
    Actions,
    get renderPanel() { return uiCallbacks.renderPanel; },
    get renderAllPanels() { return uiCallbacks.renderAllPanels; },
    get renderSidebar() { return uiCallbacks.renderSidebar; },
    get syncTimeRange() { return uiCallbacks.syncTimeRange; },
};

/**
 * Register UI callbacks for effects.
 */
export function setUICallbacks(callbacks) {
    uiCallbacks = { ...uiCallbacks, ...callbacks };
}

// ============================================================
// UPLOT SYNC
// ============================================================

let uplotSync = null;

/**
 * Get or create the uPlot sync instance.
 * All panels should use this for synchronized zoom/pan.
 */
export function getUPlotSync() {
    if (!uplotSync) {
        uplotSync = uPlot.sync('panels');
    }
    return uplotSync;
}

// ============================================================
// DATA ACCESS HELPERS
// ============================================================

/**
 * Get timeseries data for a panel from the DataStore.
 * @param {Object} panel
 * @returns {Object} {containerId: [{time_ms, value}]}
 */
export function getTimeseriesForPanel(panel) {
    return DataStore.getTimeseriesForContainers(panel.metric, state.selectedContainerIds);
}

/**
 * Get study result for a panel's study.
 * @param {Object} panel
 * @param {Object} study
 * @returns {Object|undefined}
 */
export function getStudyResultForPanel(panel, study) {
    const key = DataStore.studyKey(panel.metric, study.containerId, study.type);
    return DataStore.getStudyResult(key);
}

/**
 * Get all timeseries timestamps across all panels (for full range calculation).
 */
export function getAllTimestamps() {
    const timestamps = new Set();
    for (const panel of state.panels) {
        const data = getTimeseriesForPanel(panel);
        for (const points of Object.values(data)) {
            for (const p of points) {
                timestamps.add(p.time_ms);
            }
        }
    }
    return Array.from(timestamps).sort((a, b) => a - b);
}

// ============================================================
// INITIALIZATION
// ============================================================

/**
 * Initialize the application.
 */
export async function init() {
    console.log('[App] Initializing...');

    try {
        // REQ-MV-033: Check for dashboard URL params
        const { dashboardUrl, dashboardInline, templateVars } = parseDashboardParams();
        const dashboard = await loadDashboard(dashboardUrl, dashboardInline, templateVars);

        // Fetch initial data
        const [metrics, instanceInfo] = await Promise.all([
            Api.fetchMetrics(),
            Api.fetchInstance(),
        ]);

        // Set metrics
        dispatch({ type: Actions.SET_METRICS, metrics });

        // REQ-MV-036: Determine panels from dashboard or use default
        let panelMetrics;
        if (dashboard?.panels?.length > 0) {
            // Use dashboard panel config (limit to 5 panels)
            panelMetrics = dashboard.panels.slice(0, 5).map(p => p.metric);
            console.log('[App] Dashboard panels:', panelMetrics);
        } else {
            // Default: single panel with default metric
            panelMetrics = [findDefaultMetric(metrics)];
        }

        // REQ-MV-036: Set up panels from dashboard config
        if (dashboard?.panels?.length > 0) {
            // Dashboard specifies panels - override default metric selection
            // Update first panel's metric
            state = {
                ...state,
                panels: state.panels.map((p, i) =>
                    i === 0 ? { ...p, metric: panelMetrics[0] } : p
                ),
            };

            // Add additional panels from dashboard
            for (let i = 1; i < panelMetrics.length; i++) {
                if (state.panels.length < state.maxPanels) {
                    const newPanel = {
                        id: state.nextPanelId,
                        metric: panelMetrics[i],
                        loading: false,
                        studies: [],
                    };
                    state = {
                        ...state,
                        panels: [...state.panels, newPanel],
                        nextPanelId: state.nextPanelId + 1,
                    };
                }
            }
            console.log('[App] Created', state.panels.length, 'panels from dashboard');
        }

        // REQ-MV-034: Build container filter params from dashboard
        const filterParams = buildContainerFilterParams(dashboard);
        filterParams.set('metric', panelMetrics[0] || '');

        // Fetch containers (with dashboard filters if specified)
        let containers = [];
        if (panelMetrics[0]) {
            const url = `/api/containers?${filterParams.toString()}`;
            const res = await fetch(url);
            const data = await res.json();
            containers = data.containers || [];
            dispatch({ type: Actions.SET_CONTAINERS, containers });

            // REQ-MV-034: Auto-select all filtered containers when dashboard specifies filters
            if (dashboard?.containers && containers.length > 0) {
                const containerIds = containers.map(c => c.short_id);
                dispatch({ type: Actions.SET_SELECTED_CONTAINERS, containerIds });
                console.log('[App] Auto-selected', containerIds.length, 'containers from dashboard filter');
            }

            // REQ-MV-035: Compute time range from container bounds
            if (dashboard?.time_range) {
                const timeRange = computeTimeRangeFromContainers(containers, dashboard.time_range);
                if (timeRange) {
                    state = {
                        ...state,
                        fullTimeRange: timeRange,
                        timeRange: { ...timeRange },
                    };
                    console.log('[App] Time range from containers:', new Date(timeRange.min), '-', new Date(timeRange.max));
                }
            }

            // Set up initial refs for all panels
            for (const cid of state.selectedContainerIds) {
                for (const panel of state.panels) {
                    if (panel.metric) {
                        const key = DataStore.timeseriesKey(panel.metric, cid);
                        DataStore.addRef(key, panel.id);
                    }
                }
            }
        }

        dispatch({ type: Actions.INIT_COMPLETE, fullTimeRange: state.fullTimeRange });
        console.log('[App] Initialization complete');

        return { instanceInfo, dashboard };
    } catch (err) {
        console.error('[App] Initialization failed:', err);
        throw err;
    }
}

/**
 * Find the best default metric.
 */
function findDefaultMetric(metrics) {
    // Prefer total_cpu_usage_millicores if available
    const preferred = metrics.find(m => m.name === 'total_cpu_usage_millicores');
    if (preferred) return preferred.name;

    // Fallback to first metric
    return metrics[0]?.name || null;
}

// ============================================================
// ACTION HELPERS (convenience wrappers)
// ============================================================

export function addPanel(metric) {
    if (!canAddPanel(state)) return false;
    dispatch({ type: Actions.ADD_PANEL, metric });
    return true;
}

export function removePanel(panelId) {
    if (!canRemovePanel(state)) return false;
    dispatch({ type: Actions.REMOVE_PANEL, panelId });
    return true;
}

export function setPanelMetric(panelId, metric) {
    dispatch({ type: Actions.SET_PANEL_METRIC, panelId, metric });
}

export function setSelectedContainers(containerIds) {
    dispatch({ type: Actions.SET_SELECTED_CONTAINERS, containerIds });
}

export function toggleContainer(containerId) {
    dispatch({ type: Actions.TOGGLE_CONTAINER, containerId });
}

export function addStudy(panelId, studyType) {
    dispatch({ type: Actions.ADD_STUDY, panelId, studyType });
}

export function removeStudy(panelId) {
    dispatch({ type: Actions.REMOVE_STUDY, panelId });
}

export function setTimeRange(min, max) {
    dispatch({ type: Actions.SET_TIME_RANGE, min, max });
}

export function resetTimeRange() {
    dispatch({ type: Actions.RESET_TIME_RANGE });
}

export function setMetricSearch(query) {
    dispatch({ type: Actions.SET_METRIC_SEARCH, query });
}

export function setContainerSearch(query) {
    dispatch({ type: Actions.SET_CONTAINER_SEARCH, query });
}

// ============================================================
// EXPORTS FOR DEBUGGING
// ============================================================

// Expose for console debugging
if (typeof window !== 'undefined') {
    window.__APP_DEBUG__ = {
        getState,
        dispatch,
        Actions,
        DataStore,
        getStats: DataStore.getStats,
        getRefsSnapshot: DataStore.getRefsSnapshot,
    };
}
