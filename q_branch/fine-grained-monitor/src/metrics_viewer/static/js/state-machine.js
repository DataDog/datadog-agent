/**
 * State Machine - Pure reducer with actions and effect descriptors.
 *
 * The reducer is a pure function: (state, action) -> {state, effects}
 * Effects are descriptors, not executed here - the effect executor handles them.
 *
 * State is kept lightweight - only IDs and UI state, no heavy data.
 * Heavy data (timeseries, study results) lives in DataStore.
 */

// ============================================================
// ACTION TYPES
// ============================================================

export const Actions = {
    // Panel management
    ADD_PANEL: 'ADD_PANEL',
    REMOVE_PANEL: 'REMOVE_PANEL',
    SET_PANEL_METRIC: 'SET_PANEL_METRIC',
    SET_PANEL_LOADING: 'SET_PANEL_LOADING',

    // Container selection
    SET_SELECTED_CONTAINERS: 'SET_SELECTED_CONTAINERS',
    TOGGLE_CONTAINER: 'TOGGLE_CONTAINER',

    // Time range (for sync tracking, actual sync via uPlot)
    SET_TIME_RANGE: 'SET_TIME_RANGE',
    RESET_TIME_RANGE: 'RESET_TIME_RANGE',

    // Studies
    ADD_STUDY: 'ADD_STUDY',
    REMOVE_STUDY: 'REMOVE_STUDY',
    SET_STUDY_LOADING: 'SET_STUDY_LOADING',

    // Metadata
    SET_METRICS: 'SET_METRICS',
    SET_CONTAINERS: 'SET_CONTAINERS',

    // UI state
    SET_METRIC_SEARCH: 'SET_METRIC_SEARCH',
    SET_CONTAINER_SEARCH: 'SET_CONTAINER_SEARCH',
    TOGGLE_GROUP_BY_NAME: 'TOGGLE_GROUP_BY_NAME',

    // Initialization
    INIT_COMPLETE: 'INIT_COMPLETE',

    // REQ-MV-037: Data time range selection
    SET_DATA_TIME_RANGE: 'SET_DATA_TIME_RANGE',
};

// ============================================================
// EFFECT TYPES
// ============================================================

export const Effects = {
    // Data fetching
    FETCH_METRICS: 'FETCH_METRICS',
    FETCH_CONTAINERS: 'FETCH_CONTAINERS',
    FETCH_TIMESERIES: 'FETCH_TIMESERIES',
    FETCH_STUDY: 'FETCH_STUDY',

    // Cache management
    EVICT_KEYS: 'EVICT_KEYS',
    UPDATE_REFS: 'UPDATE_REFS',

    // UI updates
    RENDER_PANELS: 'RENDER_PANELS',
    RENDER_SIDEBAR: 'RENDER_SIDEBAR',
    SYNC_TIME_RANGE: 'SYNC_TIME_RANGE',
};

// ============================================================
// INITIAL STATE
// ============================================================

export const initialState = {
    // Initialization
    initialized: false,

    // Metadata (from API)
    metrics: [],        // [{name, sample_count}]
    containers: [],     // [{short_id, pod_name, namespace, qos_class, last_seen_ms}]

    // Panel state - lightweight, just IDs and config
    panels: [
        // Initial panel
        {
            id: 1,
            metric: null,       // Set after metrics load
            loading: false,
            study: null,        // 'periodicity' | 'changepoint' | null
        }
    ],
    nextPanelId: 2,

    // Shared selection
    selectedContainerIds: [],

    // Time range (null = full range, set by uPlot sync)
    timeRange: {
        min: null,
        max: null,
    },
    fullTimeRange: {
        min: null,
        max: null,
    },

    // UI state
    metricSearchQuery: '',
    containerSearchQuery: '',

    // Display options
    groupByContainerName: true, // Group containers with same name in legend/charts

    // REQ-MV-037: Data time range for API queries (1h, 1d, 1w, all)
    // REQ-MV-038: Default to 1 hour
    dataTimeRange: '1h',

    // REQ-MV-033: Dashboard config (null when no dashboard active)
    dashboard: null,

    // Constraints
    maxPanels: 5,
};

// ============================================================
// REDUCER
// ============================================================

/**
 * Pure reducer function.
 * @param {Object} state - Current state
 * @param {Object} action - Action with type and payload
 * @returns {{state: Object, effects: Array}} New state and effects to execute
 */
export function reduce(state, action) {
    switch (action.type) {
        // --------------------------------------------------------
        // PANEL MANAGEMENT
        // --------------------------------------------------------

        case Actions.ADD_PANEL: {
            if (state.panels.length >= state.maxPanels) {
                return { state, effects: [] };
            }

            const newPanel = {
                id: state.nextPanelId,
                metric: action.metric,
                loading: true,
                study: null,
            };

            const newState = {
                ...state,
                panels: [...state.panels, newPanel],
                nextPanelId: state.nextPanelId + 1,
            };

            return {
                state: newState,
                effects: [
                    {
                        type: Effects.UPDATE_REFS,
                        panelId: newPanel.id,
                        oldMetric: null,
                        newMetric: action.metric,
                        containerIds: state.selectedContainerIds,
                    },
                    {
                        type: Effects.FETCH_TIMESERIES,
                        panelId: newPanel.id,
                        metric: action.metric,
                        containerIds: state.selectedContainerIds,
                    },
                    { type: Effects.RENDER_PANELS },
                    { type: Effects.RENDER_SIDEBAR },
                ],
            };
        }

        case Actions.REMOVE_PANEL: {
            if (state.panels.length <= 1) {
                return { state, effects: [] };
            }

            const panel = state.panels.find(p => p.id === action.panelId);
            if (!panel) {
                return { state, effects: [] };
            }

            const newState = {
                ...state,
                panels: state.panels.filter(p => p.id !== action.panelId),
            };

            return {
                state: newState,
                effects: [
                    {
                        type: Effects.UPDATE_REFS,
                        panelId: panel.id,
                        oldMetric: panel.metric,
                        newMetric: null,
                        containerIds: state.selectedContainerIds,
                        oldStudy: panel.study,
                        remove: true,
                    },
                    { type: Effects.RENDER_PANELS },
                    { type: Effects.RENDER_SIDEBAR },
                ],
            };
        }

        case Actions.SET_PANEL_METRIC: {
            const panel = state.panels.find(p => p.id === action.panelId);
            if (!panel || panel.metric === action.metric) {
                return { state, effects: [] };
            }

            const oldMetric = panel.metric;
            const updatedPanel = {
                ...panel,
                metric: action.metric,
                loading: true,
                study: null, // Clear study on metric change
            };

            const newState = {
                ...state,
                panels: state.panels.map(p =>
                    p.id === action.panelId ? updatedPanel : p
                ),
            };

            return {
                state: newState,
                effects: [
                    {
                        type: Effects.UPDATE_REFS,
                        panelId: panel.id,
                        oldMetric,
                        newMetric: action.metric,
                        containerIds: state.selectedContainerIds,
                        oldStudy: panel.study,
                    },
                    {
                        type: Effects.FETCH_TIMESERIES,
                        panelId: panel.id,
                        metric: action.metric,
                        containerIds: state.selectedContainerIds,
                    },
                    { type: Effects.RENDER_PANELS },
                    { type: Effects.RENDER_SIDEBAR },
                ],
            };
        }

        case Actions.SET_PANEL_LOADING: {
            const newState = {
                ...state,
                panels: state.panels.map(p =>
                    p.id === action.panelId ? { ...p, loading: action.loading } : p
                ),
            };
            return { state: newState, effects: [] };
        }

        // --------------------------------------------------------
        // CONTAINER SELECTION
        // --------------------------------------------------------

        case Actions.SET_SELECTED_CONTAINERS: {
            const oldIds = state.selectedContainerIds;
            const newIds = action.containerIds;

            // No change
            if (JSON.stringify(oldIds.sort()) === JSON.stringify(newIds.sort())) {
                return { state, effects: [] };
            }

            const newState = {
                ...state,
                selectedContainerIds: newIds,
                panels: state.panels.map(p => ({ ...p, loading: true })),
            };

            // Build effects for each panel
            const effects = [
                {
                    type: Effects.UPDATE_REFS,
                    panels: state.panels,
                    oldContainerIds: oldIds,
                    newContainerIds: newIds,
                    bulk: true,
                },
            ];

            // Fetch timeseries for each panel
            for (const panel of state.panels) {
                effects.push({
                    type: Effects.FETCH_TIMESERIES,
                    panelId: panel.id,
                    metric: panel.metric,
                    containerIds: newIds,
                });
            }

            effects.push({ type: Effects.RENDER_PANELS });
            effects.push({ type: Effects.RENDER_SIDEBAR });

            return { state: newState, effects };
        }

        case Actions.TOGGLE_CONTAINER: {
            const current = state.selectedContainerIds;
            const newIds = current.includes(action.containerId)
                ? current.filter(id => id !== action.containerId)
                : [...current, action.containerId];

            return reduce(state, {
                type: Actions.SET_SELECTED_CONTAINERS,
                containerIds: newIds,
            });
        }

        // --------------------------------------------------------
        // TIME RANGE
        // --------------------------------------------------------

        case Actions.SET_TIME_RANGE: {
            const newState = {
                ...state,
                timeRange: {
                    min: action.min,
                    max: action.max,
                },
            };
            return { state: newState, effects: [] };
        }

        case Actions.RESET_TIME_RANGE: {
            const newState = {
                ...state,
                timeRange: {
                    min: state.fullTimeRange.min,
                    max: state.fullTimeRange.max,
                },
            };
            return {
                state: newState,
                effects: [{ type: Effects.SYNC_TIME_RANGE, reset: true }],
            };
        }

        // --------------------------------------------------------
        // STUDIES
        // --------------------------------------------------------

        case Actions.ADD_STUDY: {
            const panel = state.panels.find(p => p.id === action.panelId);
            if (!panel || !panel.metric) {
                return { state, effects: [] };
            }

            // Check if same study already active
            if (panel.study === action.studyType) {
                return { state, effects: [] };
            }

            const updatedPanel = {
                ...panel,
                study: action.studyType,
                loading: true,
            };

            const newState = {
                ...state,
                panels: state.panels.map(p =>
                    p.id === action.panelId ? updatedPanel : p
                ),
            };

            return {
                state: newState,
                effects: [
                    {
                        type: Effects.FETCH_STUDY,
                        panelId: panel.id,
                        metric: panel.metric,
                        studyType: action.studyType,
                        containerIds: state.selectedContainerIds,
                    },
                    { type: Effects.RENDER_PANELS },
                    { type: Effects.RENDER_SIDEBAR },
                ],
            };
        }

        case Actions.REMOVE_STUDY: {
            const panel = state.panels.find(p => p.id === action.panelId);
            if (!panel || !panel.study) {
                return { state, effects: [] };
            }

            const oldStudy = panel.study;
            const updatedPanel = {
                ...panel,
                study: null,
            };

            const newState = {
                ...state,
                panels: state.panels.map(p =>
                    p.id === action.panelId ? updatedPanel : p
                ),
            };

            return {
                state: newState,
                effects: [
                    {
                        type: Effects.UPDATE_REFS,
                        panelId: panel.id,
                        removeStudy: {
                            metric: panel.metric,
                            studyType: oldStudy,
                        },
                    },
                    { type: Effects.RENDER_PANELS },
                    { type: Effects.RENDER_SIDEBAR },
                ],
            };
        }

        case Actions.SET_STUDY_LOADING: {
            const newState = {
                ...state,
                panels: state.panels.map(p =>
                    p.id === action.panelId ? { ...p, loading: action.loading } : p
                ),
            };
            return { state: newState, effects: [] };
        }

        // --------------------------------------------------------
        // METADATA
        // --------------------------------------------------------

        case Actions.SET_METRICS: {
            const newState = {
                ...state,
                metrics: action.metrics,
            };

            // Set default metric for panels that don't have one
            if (action.metrics.length > 0) {
                const defaultMetric = action.metrics[0].name;
                newState.panels = state.panels.map(p =>
                    p.metric === null ? { ...p, metric: defaultMetric } : p
                );
            }

            return { state: newState, effects: [] };
        }

        case Actions.SET_CONTAINERS: {
            const newState = {
                ...state,
                containers: action.containers,
            };
            return {
                state: newState,
                effects: [{ type: Effects.RENDER_SIDEBAR }],
            };
        }

        // --------------------------------------------------------
        // UI STATE
        // --------------------------------------------------------

        case Actions.SET_METRIC_SEARCH: {
            const newState = {
                ...state,
                metricSearchQuery: action.query,
            };
            return { state: newState, effects: [] };
        }

        case Actions.SET_CONTAINER_SEARCH: {
            const newState = {
                ...state,
                containerSearchQuery: action.query,
            };
            return { state: newState, effects: [] };
        }

        case Actions.TOGGLE_GROUP_BY_NAME: {
            const newState = {
                ...state,
                groupByContainerName: !state.groupByContainerName,
            };
            return {
                state: newState,
                effects: [
                    { type: Effects.RENDER_PANELS },
                    { type: Effects.RENDER_SIDEBAR },
                ],
            };
        }

        // --------------------------------------------------------
        // INITIALIZATION
        // --------------------------------------------------------

        case Actions.INIT_COMPLETE: {
            const newState = {
                ...state,
                initialized: true,
                fullTimeRange: action.fullTimeRange || state.fullTimeRange,
                dashboard: action.dashboard || state.dashboard,
            };
            return { state: newState, effects: [] };
        }

        // --------------------------------------------------------
        // REQ-MV-037: DATA TIME RANGE
        // --------------------------------------------------------

        case Actions.SET_DATA_TIME_RANGE: {
            if (state.dataTimeRange === action.range) {
                return { state, effects: [] };
            }

            const newState = {
                ...state,
                dataTimeRange: action.range,
                // Mark all panels as loading since we'll refetch data
                panels: state.panels.map(p => ({ ...p, loading: true })),
            };

            return {
                state: newState,
                effects: [
                    // Fetch containers for the new time range
                    { type: Effects.FETCH_CONTAINERS, metric: state.panels[0]?.metric },
                    { type: Effects.RENDER_SIDEBAR },
                ],
            };
        }

        default:
            console.warn('Unknown action type:', action.type);
            return { state, effects: [] };
    }
}

// ============================================================
// SELECTORS (derive data from state)
// ============================================================

/**
 * Get panel by ID.
 */
export function getPanel(state, panelId) {
    return state.panels.find(p => p.id === panelId);
}

/**
 * Check if we can add more panels.
 */
export function canAddPanel(state) {
    return state.panels.length < state.maxPanels;
}

/**
 * Check if we can remove panels.
 */
export function canRemovePanel(state) {
    return state.panels.length > 1;
}

/**
 * Get all unique metrics currently in use by panels.
 */
export function getActiveMetrics(state) {
    return [...new Set(state.panels.map(p => p.metric).filter(Boolean))];
}

/**
 * Get all active studies across all panels.
 */
export function getAllStudies(state) {
    return state.panels
        .filter(p => p.study !== null)
        .map(p => ({ studyType: p.study, panelId: p.id, metric: p.metric }));
}

/**
 * Group containers by display name (pod_name/container_name).
 * Returns Map<displayName, containerIds[]>
 */
export function groupContainersByName(state) {
    const groups = new Map();

    for (const id of state.selectedContainerIds) {
        const container = state.containers.find(c => c.short_id === id);
        const displayName = container?.pod_name && container?.container_name
            ? `${container.pod_name}/${container.container_name}`
            : container?.pod_name || id;

        if (!groups.has(displayName)) {
            groups.set(displayName, []);
        }
        groups.get(displayName).push(id);
    }

    return groups;
}

/**
 * Get color index for a container, respecting grouping.
 * When grouping is enabled, all containers with the same name get the same color.
 */
export function getContainerColorIndex(state, containerId) {
    if (!state.groupByContainerName) {
        // No grouping - color by position in selection
        return state.selectedContainerIds.indexOf(containerId);
    }

    // Grouping enabled - color by group index
    const groups = groupContainersByName(state);
    let groupIndex = 0;
    for (const [displayName, ids] of groups) {
        if (ids.includes(containerId)) {
            return groupIndex;
        }
        groupIndex++;
    }
    return -1;
}
