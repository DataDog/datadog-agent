/**
 * UI Module - Components, Chart Rendering, and Theme Management.
 *
 * This module contains all UI-specific code:
 * - ThemeManager: Dark/Light theme handling
 * - Components: Pure rendering functions for sidebar
 * - ChartRenderer: uPlot chart rendering and interactions
 * - Event setup and render functions
 */

import * as App from './app.js';
import * as DataStore from './data-store.js';

// ============================================================
// CONSTANTS
// ============================================================

export const COLORS = [
    '#1f77b4', '#ff7f0e', '#2ca02c', '#d62728', '#9467bd',
    '#8c564b', '#e377c2', '#7f7f7f', '#bcbd22', '#17becf'
];

// ============================================================
// THEME MANAGEMENT
// ============================================================

export const ThemeManager = {
    STORAGE_KEY: 'fgm-theme-preference',

    getStoredPreference() {
        try {
            return localStorage.getItem(this.STORAGE_KEY) || 'auto';
        } catch {
            return 'auto';
        }
    },

    setStoredPreference(value) {
        try {
            localStorage.setItem(this.STORAGE_KEY, value);
        } catch {
            // localStorage not available
        }
    },

    getEffectiveTheme(preference) {
        if (preference === 'auto') {
            if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
                return 'dark';
            }
            return 'light';
        }
        return preference;
    },

    applyTheme(theme) {
        if (theme === 'dark') {
            document.documentElement.setAttribute('data-theme', 'dark');
        } else {
            document.documentElement.removeAttribute('data-theme');
        }
    },

    updateButtons(preference) {
        document.querySelectorAll('.theme-toggle-btn').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.themeValue === preference);
        });
    },

    setPreference(preference) {
        this.setStoredPreference(preference);
        const effectiveTheme = this.getEffectiveTheme(preference);
        this.applyTheme(effectiveTheme);
        this.updateButtons(preference);
    },

    init() {
        const preference = this.getStoredPreference();
        const effectiveTheme = this.getEffectiveTheme(preference);
        this.applyTheme(effectiveTheme);

        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', () => {
                this.updateButtons(preference);
                this.setupListeners();
            });
        } else {
            this.updateButtons(preference);
            this.setupListeners();
        }

        if (window.matchMedia) {
            window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
                const currentPref = this.getStoredPreference();
                if (currentPref === 'auto') {
                    const effectiveTheme = this.getEffectiveTheme('auto');
                    this.applyTheme(effectiveTheme);
                    this.onThemeChange();
                }
            });
        }
    },

    setupListeners() {
        document.querySelectorAll('.theme-toggle-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                this.setPreference(btn.dataset.themeValue);
                this.onThemeChange();
            });
        });
    },

    onThemeChange() {
        setTimeout(() => {
            const state = App.getState();
            if (state.selectedContainerIds.length > 0) {
                const preservedXRange = ChartRenderer.getCurrentXRange();
                ChartRenderer.renderAllPanels();
            }
        }, 50);
    }
};

// ============================================================
// HELPER FUNCTIONS
// ============================================================

function escapeHtml(str) {
    if (str == null) return '';
    const div = document.createElement('div');
    div.textContent = String(str);
    return div.innerHTML;
}

function escapeAttr(str) {
    if (str == null) return '';
    return String(str).replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

function getContainerDisplayName(containerInfo, fallbackId) {
    if (containerInfo?.pod_name && containerInfo?.container_name) {
        return `${containerInfo.pod_name}/${containerInfo.container_name}`;
    }
    return containerInfo?.pod_name || fallbackId;
}

function getContainerColor(containerId, selectedIds) {
    const index = selectedIds.indexOf(containerId);
    if (index === -1) return '#ddd';
    return COLORS[index % COLORS.length];
}

function getCssVar(name) {
    return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

export function setLoading(isLoading, message = 'Loading data...') {
    const overlay = document.getElementById('loadingOverlay');
    const textEl = overlay?.querySelector('.loading-text');
    if (overlay) {
        overlay.style.display = isLoading ? 'flex' : 'none';
        if (textEl && message) {
            textEl.textContent = message;
        }
    }
}

// ============================================================
// COMPONENTS (Pure Rendering Functions)
// ============================================================

export const Components = {
    MetricList(state) {
        if (state.metrics.length === 0) {
            return `
                <div style="padding: 16px; text-align: center;">
                    <div style="font-size: 24px; margin-bottom: 8px;">⏳</div>
                    <div style="color: #666; font-size: 13px; font-weight: 500;">Waiting for data...</div>
                    <div style="color: #888; font-size: 11px; margin-top: 4px;">
                        Collector is writing metrics.<br>
                        Page will refresh automatically.
                    </div>
                </div>
            `;
        }

        let filtered = state.metrics;
        if (state.metricSearchQuery?.trim()) {
            const query = state.metricSearchQuery.toLowerCase();
            filtered = state.metrics.filter(m => m.name.toLowerCase().includes(query));
        }

        if (filtered.length === 0) {
            return '<div style="padding: 8px; color: #888; font-size: 12px;">No metrics match search</div>';
        }

        // Get first panel's metric for selection highlight
        const selectedMetric = state.panels[0]?.metric;

        return filtered.map(m => {
            const isSelected = m.name === selectedMetric;
            const selectedClass = isSelected ? 'selected' : '';
            return `
                <div class="metric-item ${selectedClass}" data-metric="${escapeAttr(m.name)}">
                    <span class="metric-name">${escapeHtml(m.name)}</span>
                </div>
            `;
        }).join('');
    },

    ContainerList(state) {
        if (state.containers.length === 0) {
            return '<div style="padding: 8px; color: #888; font-size: 12px;">No containers found</div>';
        }

        const now = Date.now();
        const ONE_HOUR_MS = 60 * 60 * 1000;

        return state.containers.map(c => {
            const containerId = c.short_id;
            const displayName = getContainerDisplayName(c, containerId);
            const isSelected = state.selectedContainerIds.includes(containerId);
            const color = getContainerColor(containerId, state.selectedContainerIds);
            const selectedClass = isSelected ? 'selected' : '';

            const lastSeenMs = c.last_seen_ms || 0;
            const isStale = lastSeenMs > 0 && (now - lastSeenMs) > ONE_HOUR_MS;
            const staleStyle = isStale ? 'opacity: 0.6;' : '';

            // Check if any panel has a study for this container
            const hasStudy = state.panels.some(p =>
                p.studies.some(s => s.containerId === containerId)
            );
            const studyBtnStyle = hasStudy
                ? 'background: #007bff; color: white; opacity: 1;'
                : '';

            return `
                <label class="container-item ${selectedClass}" style="${staleStyle}">
                    <input type="checkbox"
                           value="${escapeAttr(containerId)}"
                           ${isSelected ? 'checked' : ''}
                           data-container-checkbox>
                    <span class="container-color" style="background: ${color}"></span>
                    <span class="container-id" title="${escapeAttr(containerId)}">${escapeHtml(displayName)}</span>
                    <button class="study-btn"
                            style="${studyBtnStyle}"
                            data-study-btn="${escapeAttr(containerId)}"
                            title="Run Study">📊</button>
                </label>
            `;
        }).join('');
    },

    // REQ-MV-020: Panel tabs in sidebar
    PanelTabs(state) {
        const { panels, maxPanels } = state;
        const canAdd = panels.length < maxPanels;
        const canRemove = panels.length > 1;

        const tabs = panels.map((panel, i) => {
            const metric = panel.metric || 'No metric';
            const shortMetric = metric.length > 20 ? metric.substring(0, 17) + '...' : metric;
            return `
                <div class="panel-tab" data-panel-id="${panel.id}" title="${escapeAttr(metric)}">
                    <span class="panel-tab-number">${i + 1}</span>
                    <span class="panel-tab-metric">${escapeHtml(shortMetric)}</span>
                    ${canRemove ? `<button class="panel-tab-remove" data-remove-panel="${panel.id}" title="Remove panel">&times;</button>` : ''}
                </div>
            `;
        }).join('');

        const addButton = canAdd
            ? `<button class="panel-tab-add" data-add-panel title="Add panel">+</button>`
            : '';

        return `
            <div class="panel-tabs-container">
                ${tabs}
                ${addButton}
            </div>
        `;
    },

    StudyPanel(state) {
        // Find active studies across all panels
        const activeStudies = [];
        for (const panel of state.panels) {
            for (const study of panel.studies) {
                if (!study.loading) {
                    activeStudies.push({ ...study, panelId: panel.id, metric: panel.metric });
                }
            }
        }

        if (activeStudies.length === 0) {
            return `
                <div style="color: #888; font-size: 12px;">
                    Click the study icon on a container to analyze
                </div>
            `;
        }

        return activeStudies.map(study => {
            const container = state.containers.find(c => c.short_id === study.containerId);
            const displayName = getContainerDisplayName(container, study.containerId);
            const key = DataStore.studyKey(study.metric, study.containerId, study.type);
            const result = DataStore.getStudyResult(key);

            if (study.type === 'changepoint') {
                return Components.ChangepointPanel(state, study, displayName, result);
            } else {
                return Components.PeriodicityPanel(state, study, displayName, result);
            }
        }).join('');
    },

    PeriodicityPanel(state, study, displayName, result) {
        const windowCount = result?.windows?.length || 0;

        let dominantPeriod = 'N/A';
        let avgConfidence = 'N/A';
        let groupedSummaryHtml = '';

        if (result && result.windows && result.windows.length > 0) {
            const periodGroups = {};
            result.windows.forEach(w => {
                const p = Math.round(w.metrics.period);
                if (!periodGroups[p]) {
                    periodGroups[p] = [];
                }
                periodGroups[p].push(w);
            });

            const sortedPeriods = Object.keys(periodGroups)
                .map(p => parseInt(p))
                .sort((a, b) => periodGroups[b].length - periodGroups[a].length);
            dominantPeriod = sortedPeriods[0] + 's';

            const totalScore = result.windows.reduce((sum, w) => sum + (w.metrics.score || 0), 0);
            avgConfidence = Math.round((totalScore / result.windows.length) * 100) + '%';

            const formatTime = (ms) => {
                const d = new Date(ms);
                return d.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
            };

            groupedSummaryHtml = sortedPeriods.map(period => {
                const windows = periodGroups[period];
                windows.sort((a, b) => a.start_time_ms - b.start_time_ms);

                const windowLines = windows.map(w => {
                    const startTime = formatTime(w.start_time_ms);
                    const endTime = formatTime(w.end_time_ms);
                    const confidence = Math.round((w.metrics.score || 0) * 100);
                    const amplitude = (w.metrics.amplitude || 0).toFixed(1);
                    return `<div class="period-window-item"
                                data-window-start="${w.start_time_ms}"
                                data-window-end="${w.end_time_ms}"
                                title="Click to zoom">${startTime} - ${endTime} (${confidence}%, amp: ${amplitude})</div>`;
                }).join('');

                return `
                    <div class="period-group">
                        <div class="period-group-header">${period}s period (${windows.length} windows)</div>
                        <div class="period-group-windows">${windowLines}</div>
                    </div>
                `;
            }).join('');
        }

        const summarySection = groupedSummaryHtml ? `
            <div class="study-panel-summary">
                <div class="study-panel-summary-title">Detected Patterns</div>
                ${groupedSummaryHtml}
            </div>
        ` : '';

        return `
            <div class="study-panel" data-panel-id="${study.panelId}" data-study-type="${study.type}" data-container-id="${study.containerId}">
                <div class="study-panel-header">
                    <span>Periodicity Study</span>
                    <button class="study-panel-close" data-close-study title="Remove study">&times;</button>
                </div>
                <div class="study-panel-container" title="${escapeAttr(study.containerId)}">${escapeHtml(displayName)}</div>
                <div class="study-panel-stats">
                    <div><strong>${windowCount}</strong> periodic windows</div>
                    <div>Dominant period: <strong>${dominantPeriod}</strong></div>
                    <div>Avg confidence: <strong>${avgConfidence}</strong></div>
                </div>
                ${summarySection}
            </div>
        `;
    },

    ChangepointPanel(state, study, displayName, result) {
        const changepoints = result?.windows || [];
        const count = changepoints.length;

        let largestMagnitude = 0;
        let summaryHtml = '';

        if (count > 0) {
            largestMagnitude = changepoints.reduce((max, w) =>
                Math.max(max, w.metrics?.magnitude || 0), 0);

            const formatTime = (ms) => {
                const d = new Date(ms);
                return d.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
            };

            const changepointLines = changepoints
                .sort((a, b) => a.start_time_ms - b.start_time_ms)
                .map(w => {
                    const time = formatTime(w.start_time_ms);
                    const direction = (w.metrics?.direction || 0) > 0 ? '+' : '';
                    const magnitude = (w.metrics?.magnitude || 0).toFixed(1);
                    const before = (w.metrics?.value_before || 0).toFixed(1);
                    const after = (w.metrics?.value_after || 0).toFixed(1);
                    return `<div class="period-window-item"
                                data-changepoint-time="${w.start_time_ms}"
                                title="Click to zoom: ${before} → ${after}">${time}: ${direction}${magnitude}</div>`;
                }).join('');

            summaryHtml = `
                <div class="study-panel-summary">
                    <div class="study-panel-summary-title">Detected Changes</div>
                    <div class="period-group-windows">${changepointLines}</div>
                </div>
            `;
        }

        return `
            <div class="study-panel" style="background: var(--changepoint-panel-bg); border-color: var(--changepoint-panel-border);"
                 data-panel-id="${study.panelId}" data-study-type="${study.type}" data-container-id="${study.containerId}">
                <div class="study-panel-header">
                    <span>Changepoint Study</span>
                    <button class="study-panel-close" data-close-study title="Remove study">&times;</button>
                </div>
                <div class="study-panel-container" title="${escapeAttr(study.containerId)}">${escapeHtml(displayName)}</div>
                <div class="study-panel-stats">
                    <div><strong>${count}</strong> changepoint${count !== 1 ? 's' : ''} detected</div>
                    <div>Largest change: <strong>${largestMagnitude.toFixed(1)}</strong></div>
                </div>
                ${summaryHtml}
            </div>
        `;
    },

    Legend(state, panelId) {
        if (state.selectedContainerIds.length === 0) return '';

        return state.selectedContainerIds.map((id, i) => {
            const container = state.containers.find(c => c.short_id === id);
            const displayName = getContainerDisplayName(container, id);
            const qos = container?.qos_class || '?';
            return `
                <div class="legend-item">
                    <div class="legend-color" style="background: ${COLORS[i % COLORS.length]}"></div>
                    <span title="${escapeAttr(id)}">${escapeHtml(displayName)} (${escapeHtml(qos)})</span>
                </div>
            `;
        }).join('');
    }
};

// ============================================================
// CHART RENDERER
// ============================================================

// Store uPlot instances per panel
const plotInstances = new Map(); // panelId -> { main: uPlot, overview: uPlot, rangeOverlay: element }

export const ChartRenderer = {
    getCurrentXRange(panelId) {
        const instance = panelId ? plotInstances.get(panelId) : plotInstances.values().next().value;
        if (instance?.main?.scales?.x) {
            return {
                min: instance.main.scales.x.min,
                max: instance.main.scales.x.max
            };
        }
        return null;
    },

    // REQ-MV-020: Render all panels
    renderAllPanels() {
        const state = App.getState();
        const panelsContainer = document.getElementById('panelsContainer');
        if (!panelsContainer) return;

        // Destroy removed panels
        for (const [panelId, instance] of plotInstances.entries()) {
            if (!state.panels.find(p => p.id === panelId)) {
                if (instance.main) instance.main.destroy();
                if (instance.overview) instance.overview.destroy();
                plotInstances.delete(panelId);
            }
        }

        // Clear and rebuild panels HTML structure
        panelsContainer.innerHTML = state.panels.map(panel => `
            <div class="panel" data-panel-id="${panel.id}">
                <div class="panel-header">
                    <select class="panel-metric-select" data-panel-id="${panel.id}">
                        ${state.metrics.map(m => `
                            <option value="${escapeAttr(m.name)}" ${m.name === panel.metric ? 'selected' : ''}>
                                ${escapeHtml(m.name)}
                            </option>
                        `).join('')}
                    </select>
                </div>
                <div class="panel-chart-area">
                    ${panel.loading ? `
                        <div class="loading-overlay">
                            <div class="loading-spinner"></div>
                            <div class="loading-text">Loading data...</div>
                        </div>
                    ` : ''}
                    <div class="legend" id="legend-${panel.id}"></div>
                    <div class="panel-main-chart" id="mainChart-${panel.id}"></div>
                </div>
            </div>
        `).join('');

        // Render each panel's chart
        for (const panel of state.panels) {
            if (!panel.loading) {
                ChartRenderer.renderPanel(panel.id);
            }
        }

        // Add shared overview at bottom
        ChartRenderer.renderSharedOverview();
    },

    renderPanel(panelId) {
        const state = App.getState();
        const panel = state.panels.find(p => p.id === panelId);
        if (!panel) return;

        const chartDiv = document.getElementById(`mainChart-${panelId}`);
        const legendDiv = document.getElementById(`legend-${panelId}`);
        if (!chartDiv) return;

        const containerIds = state.selectedContainerIds;
        const timeseries = App.getTimeseriesForPanel(panel);

        // Build uPlot data
        const allTimes = new Set();
        containerIds.forEach(id => {
            const data = timeseries[id] || [];
            data.forEach(p => allTimes.add(p.time_ms));
        });
        const timestamps = Array.from(allTimes).sort((a, b) => a - b);

        if (timestamps.length === 0) {
            chartDiv.innerHTML = '<div class="empty-state">Select containers to display timeseries</div>';
            if (legendDiv) legendDiv.innerHTML = '';
            return;
        }

        // Build value arrays
        const timeToIdx = new Map(timestamps.map((t, i) => [t, i]));
        const series = containerIds.map(id => {
            const values = new Array(timestamps.length).fill(null);
            const data = timeseries[id] || [];
            data.forEach(p => {
                const idx = timeToIdx.get(p.time_ms);
                if (idx !== undefined) values[idx] = p.value;
            });
            return values;
        });

        // Check if all series values are null
        let hasAnyData = false;
        series.forEach(s => s.forEach(v => { if (v !== null) hasAnyData = true; }));
        if (!hasAnyData) {
            chartDiv.innerHTML = `<div class="empty-state">No data for ${escapeHtml(panel.metric)}</div>`;
            if (legendDiv) legendDiv.innerHTML = '';
            return;
        }

        // Convert to uPlot format
        const uplotData = [timestamps.map(t => t / 1000), ...series];

        // Build series config
        const uplotSeries = [
            { label: 'Time' },
            ...containerIds.map((id, i) => ({
                label: id,
                stroke: COLORS[i % COLORS.length],
                width: 1.5,
                points: { show: false }
            }))
        ];

        // Build legend
        if (legendDiv) {
            legendDiv.innerHTML = Components.Legend(state, panelId);
        }

        // REQ-MV-025: Use shared uPlot sync
        const sync = App.getUPlotSync();

        // Calculate Y range
        let yMax = 0;
        series.forEach(s => s.forEach(v => { if (v !== null && v > yMax) yMax = v; }));
        if (yMax === 0) yMax = 1;

        const opts = {
            width: chartDiv.clientWidth,
            height: chartDiv.clientHeight || 200,
            series: uplotSeries,
            scales: {
                x: { time: true },
                y: { range: (u, min, max) => [0, Math.max(max * 1.1, 1)] }
            },
            cursor: {
                drag: { x: true, y: false, setScale: true },
                sync: { key: sync.key }
            },
            hooks: {
                draw: [(u) => ChartRenderer.drawStudyOverlay(u, panel)],
            },
            axes: [
                { stroke: getCssVar('--chart-axis'), grid: { stroke: getCssVar('--chart-grid') } },
                { stroke: getCssVar('--chart-axis'), grid: { stroke: getCssVar('--chart-grid') }, size: 80 }
            ],
            padding: [8, 16, 8, 8],
            legend: { show: false }
        };

        // Destroy previous instance
        const existing = plotInstances.get(panelId);
        if (existing?.main) {
            existing.main.destroy();
        }

        chartDiv.innerHTML = '';
        const plot = new uPlot(opts, uplotData, chartDiv);

        plotInstances.set(panelId, {
            main: plot,
            overview: existing?.overview || null,
            rangeOverlay: existing?.rangeOverlay || null,
            timestamps,
            yMax
        });
    },

    // REQ-MV-025: Shared overview for all panels
    renderSharedOverview() {
        const state = App.getState();
        const overviewDiv = document.getElementById('sharedOverview');
        if (!overviewDiv) return;

        // Get all timestamps from all panels
        const allTimestamps = App.getAllTimestamps();
        if (allTimestamps.length === 0) {
            overviewDiv.innerHTML = '';
            return;
        }

        // Use first panel's data for overview visualization
        const firstPanel = state.panels[0];
        if (!firstPanel) return;

        const timeseries = App.getTimeseriesForPanel(firstPanel);
        const containerIds = state.selectedContainerIds;

        // Build series for overview
        const timeToIdx = new Map(allTimestamps.map((t, i) => [t, i]));
        const series = containerIds.map(id => {
            const values = new Array(allTimestamps.length).fill(null);
            const data = timeseries[id] || [];
            data.forEach(p => {
                const idx = timeToIdx.get(p.time_ms);
                if (idx !== undefined) values[idx] = p.value;
            });
            return values;
        });

        const uplotData = [allTimestamps.map(t => t / 1000), ...series];
        const uplotSeries = [
            { label: 'Time' },
            ...containerIds.map((id, i) => ({
                label: id,
                stroke: COLORS[i % COLORS.length],
                width: 1
            }))
        ];

        const overviewOpts = {
            width: overviewDiv.clientWidth,
            height: 60,
            series: uplotSeries,
            scales: {
                x: { time: true },
                y: { range: (u, min, max) => [0, Math.max(max * 1.1, 1)] }
            },
            cursor: { show: false },
            axes: [{ show: false }, { show: false }],
            legend: { show: false }
        };

        // Clean up existing
        const firstInstance = plotInstances.get(firstPanel.id);
        if (firstInstance?.overview) {
            firstInstance.overview.destroy();
        }

        overviewDiv.innerHTML = '';
        const overviewPlot = new uPlot(overviewOpts, uplotData, overviewDiv);

        // Update instance
        if (firstInstance) {
            firstInstance.overview = overviewPlot;
        }

        // Create range overlay
        ChartRenderer.createRangeOverlay(overviewDiv, overviewPlot, allTimestamps);
    },

    drawStudyOverlay(u, panel) {
        const state = App.getState();
        if (panel.studies.length === 0) return;

        const ctx = u.ctx;
        const { left, top, width, height } = u.bbox;
        const plotRight = left + width;
        const plotBottom = top + height;

        ctx.save();
        ctx.beginPath();
        ctx.rect(left, top, width, height);
        ctx.clip();

        for (const study of panel.studies) {
            const key = DataStore.studyKey(panel.metric, study.containerId, study.type);
            const result = DataStore.getStudyResult(key);
            if (!result?.windows) continue;

            const idx = state.selectedContainerIds.indexOf(study.containerId);
            const color = COLORS[idx >= 0 ? idx % COLORS.length : 0];

            if (study.type === 'changepoint') {
                ChartRenderer.drawChangepointMarkers(ctx, u, result.windows, color, left, top, plotRight, plotBottom);
            } else {
                ChartRenderer.drawPeriodicityRegions(ctx, u, result.windows, color, left, top, plotRight, plotBottom);
            }
        }

        ctx.restore();
    },

    drawPeriodicityRegions(ctx, u, windows, color, left, top, plotRight, plotBottom) {
        windows.forEach(w => {
            let x1 = u.valToPos(w.start_time_ms / 1000, 'x', true);
            let x2 = u.valToPos(w.end_time_ms / 1000, 'x', true);

            if (x2 < left || x1 > plotRight) return;
            x1 = Math.max(x1, left);
            x2 = Math.min(x2, plotRight);

            // Shaded region
            ctx.fillStyle = color + '20';
            ctx.fillRect(x1, top, x2 - x1, plotBottom - top);

            // Period markers
            const period = w.metrics.period || 10;
            const periodMs = period * 1000;
            ctx.strokeStyle = color;
            ctx.setLineDash([4, 4]);
            ctx.lineWidth = 1;

            for (let t = w.start_time_ms; t <= w.end_time_ms; t += periodMs) {
                const x = u.valToPos(t / 1000, 'x', true);
                if (x >= left && x <= plotRight) {
                    ctx.beginPath();
                    ctx.moveTo(x, top);
                    ctx.lineTo(x, plotBottom);
                    ctx.stroke();
                }
            }
            ctx.setLineDash([]);
        });
    },

    drawChangepointMarkers(ctx, u, windows, color, left, top, plotRight, plotBottom) {
        windows.forEach(w => {
            const x = u.valToPos(w.start_time_ms / 1000, 'x', true);
            if (x < left || x > plotRight) return;

            // Solid vertical line
            ctx.strokeStyle = color;
            ctx.setLineDash([]);
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.moveTo(x, top);
            ctx.lineTo(x, plotBottom);
            ctx.stroke();

            // Direction arrow
            const direction = w.metrics?.direction || 0;
            const arrowSize = 8;
            const arrowY = direction > 0 ? top + 15 : plotBottom - 15;
            const arrowDir = direction > 0 ? 1 : -1;

            ctx.fillStyle = color;
            ctx.beginPath();
            ctx.moveTo(x, arrowY);
            ctx.lineTo(x - arrowSize / 2, arrowY + arrowDir * arrowSize);
            ctx.lineTo(x + arrowSize / 2, arrowY + arrowDir * arrowSize);
            ctx.closePath();
            ctx.fill();
        });
    },

    createRangeOverlay(overviewDiv, overviewPlot, timestamps) {
        const existing = overviewDiv.querySelector('.range-overlay');
        if (existing) existing.remove();

        const overlay = document.createElement('div');
        overlay.className = 'range-overlay';
        overlay.innerHTML = `
            <div class="range-dim left"></div>
            <div class="range-selection">
                <div class="range-handle left"></div>
                <div class="range-handle right"></div>
            </div>
            <div class="range-dim right"></div>
        `;
        overviewDiv.appendChild(overlay);

        const dimLeft = overlay.querySelector('.range-dim.left');
        const dimRight = overlay.querySelector('.range-dim.right');
        const selection = overlay.querySelector('.range-selection');
        const handleLeft = overlay.querySelector('.range-handle.left');
        const handleRight = overlay.querySelector('.range-handle.right');

        const getChartBounds = () => {
            const bbox = overviewPlot.bbox;
            return {
                left: bbox.left / devicePixelRatio,
                width: bbox.width / devicePixelRatio
            };
        };

        const valToPos = (val) => {
            const bounds = getChartBounds();
            return overviewPlot.valToPos(val, 'x') + bounds.left;
        };

        const posToVal = (px) => {
            const bounds = getChartBounds();
            return overviewPlot.posToVal(px - bounds.left, 'x');
        };

        // Store for drag state
        let rangeDrag = { active: false, mode: null, startX: 0, startLeft: 0, startWidth: 0 };

        const updateOverlayFromSync = () => {
            // Get range from first panel
            const firstInstance = plotInstances.values().next().value;
            if (!firstInstance?.main) return;

            const xScale = firstInstance.main.scales.x;
            const bounds = getChartBounds();

            const left = valToPos(xScale.min);
            const right = valToPos(xScale.max);
            const width = right - left;

            selection.style.left = left + 'px';
            selection.style.width = width + 'px';
            dimLeft.style.width = left + 'px';
            dimRight.style.left = right + 'px';
            dimRight.style.width = (bounds.left + bounds.width - right) + 'px';
        };

        // Initial update
        requestAnimationFrame(updateOverlayFromSync);

        // Set up sync update callback
        overlay._updateFromSync = updateOverlayFromSync;

        // Drag handlers
        handleLeft.addEventListener('mousedown', (e) => {
            e.stopPropagation();
            rangeDrag = { active: true, mode: 'resize-left', startX: e.clientX, startLeft: selection.offsetLeft, startWidth: selection.offsetWidth };
        });

        handleRight.addEventListener('mousedown', (e) => {
            e.stopPropagation();
            rangeDrag = { active: true, mode: 'resize-right', startX: e.clientX, startLeft: selection.offsetLeft, startWidth: selection.offsetWidth };
        });

        selection.addEventListener('mousedown', (e) => {
            if (e.target.classList.contains('range-handle')) return;
            rangeDrag = { active: true, mode: 'pan', startX: e.clientX, startLeft: selection.offsetLeft, startWidth: selection.offsetWidth };
        });

        const onDimClick = (e) => {
            const overlayRect = overlay.getBoundingClientRect();
            rangeDrag = { active: true, mode: 'select', startX: e.clientX, startLeft: e.clientX - overlayRect.left, startWidth: 0 };
        };
        dimLeft.addEventListener('mousedown', onDimClick);
        dimRight.addEventListener('mousedown', onDimClick);

        // Global mouse handlers
        const onMouseMove = (e) => {
            if (!rangeDrag.active) return;

            const bounds = getChartBounds();
            const overlayRect = overlay.getBoundingClientRect();
            const delta = e.clientX - rangeDrag.startX;
            const currentX = e.clientX - overlayRect.left;

            let newLeft, newWidth;

            switch (rangeDrag.mode) {
                case 'pan':
                    newLeft = rangeDrag.startLeft + delta;
                    newWidth = rangeDrag.startWidth;
                    newLeft = Math.max(bounds.left, Math.min(newLeft, bounds.left + bounds.width - newWidth));
                    break;
                case 'resize-left':
                    const rightEdge = rangeDrag.startLeft + rangeDrag.startWidth;
                    newLeft = Math.max(bounds.left, Math.min(rangeDrag.startLeft + delta, rightEdge - 20));
                    newWidth = rightEdge - newLeft;
                    break;
                case 'resize-right':
                    newLeft = rangeDrag.startLeft;
                    newWidth = Math.max(20, Math.min(rangeDrag.startWidth + delta, bounds.left + bounds.width - newLeft));
                    break;
                case 'select':
                    const x1 = Math.max(bounds.left, Math.min(rangeDrag.startLeft, currentX));
                    const x2 = Math.min(bounds.left + bounds.width, Math.max(rangeDrag.startLeft, currentX));
                    newLeft = x1;
                    newWidth = x2 - x1;
                    break;
            }

            selection.style.left = newLeft + 'px';
            selection.style.width = newWidth + 'px';
            dimLeft.style.width = newLeft + 'px';
            dimRight.style.left = (newLeft + newWidth) + 'px';
            dimRight.style.width = (bounds.left + bounds.width - newLeft - newWidth) + 'px';

            // Update all panels
            if (newWidth > 10) {
                const minVal = posToVal(newLeft);
                const maxVal = posToVal(newLeft + newWidth);
                for (const [panelId, instance] of plotInstances) {
                    if (instance.main) {
                        instance.main.setScale('x', { min: minVal, max: maxVal });
                    }
                }
            }
        };

        const onMouseUp = () => {
            rangeDrag = { active: false, mode: null, startX: 0, startLeft: 0, startWidth: 0 };
        };

        document.addEventListener('mousemove', onMouseMove);
        document.addEventListener('mouseup', onMouseUp);
    },

    resetZoom() {
        const timestamps = App.getAllTimestamps();
        if (timestamps.length === 0) return;

        const xMin = timestamps[0] / 1000;
        const xMax = timestamps[timestamps.length - 1] / 1000;

        for (const [panelId, instance] of plotInstances) {
            if (instance.main) {
                instance.main.setScale('x', { min: xMin, max: xMax });
            }
        }
    },

    // REQ-MV-025: Sync time range across all panels
    syncTimeRange(reset = false) {
        if (reset) {
            ChartRenderer.resetZoom();
        } else {
            // Get range from first panel and apply to all
            const firstInstance = plotInstances.values().next().value;
            if (!firstInstance?.main) return;

            const xScale = firstInstance.main.scales.x;
            for (const [panelId, instance] of plotInstances) {
                if (instance.main && instance !== firstInstance) {
                    instance.main.setScale('x', { min: xScale.min, max: xScale.max });
                }
            }
        }
    }
};

// ============================================================
// RENDER FUNCTION
// ============================================================

export function render(state, prevState) {
    // Update metric list
    const metricList = document.getElementById('metricList');
    if (metricList) {
        metricList.innerHTML = Components.MetricList(state);
        const selectedMetric = metricList.querySelector('.metric-item.selected');
        if (selectedMetric) {
            selectedMetric.scrollIntoView({ block: 'nearest', behavior: 'instant' });
        }
    }

    // Update container list
    const containerList = document.getElementById('containerList');
    if (containerList) {
        containerList.innerHTML = Components.ContainerList(state);
    }

    // Update panel tabs
    const panelTabsContainer = document.getElementById('panelTabs');
    if (panelTabsContainer) {
        panelTabsContainer.innerHTML = Components.PanelTabs(state);
    }

    // Update studies panel
    const studiesContent = document.getElementById('studiesContent');
    if (studiesContent) {
        studiesContent.innerHTML = Components.StudyPanel(state);
    }
}

// ============================================================
// EVENT SETUP
// ============================================================

export function setupEventListeners() {
    // Metric list clicks
    document.getElementById('metricList')?.addEventListener('click', (e) => {
        const item = e.target.closest('.metric-item');
        if (item) {
            const state = App.getState();
            const panelId = state.panels[0]?.id;
            if (panelId) {
                App.setPanelMetric(panelId, item.dataset.metric);
            }
        }
    });

    // Metric search
    let metricSearchTimeout;
    document.getElementById('metricSearch')?.addEventListener('input', (e) => {
        clearTimeout(metricSearchTimeout);
        metricSearchTimeout = setTimeout(() => {
            App.setMetricSearch(e.target.value);
            render(App.getState());
        }, 150);
    });

    // Container search
    let containerSearchTimeout;
    document.getElementById('containerSearch')?.addEventListener('input', (e) => {
        clearTimeout(containerSearchTimeout);
        containerSearchTimeout = setTimeout(() => {
            App.setContainerSearch(e.target.value);
        }, 300);
    });

    // Container checkboxes
    document.getElementById('containerList')?.addEventListener('change', (e) => {
        if (e.target.hasAttribute('data-container-checkbox')) {
            App.toggleContainer(e.target.value);
        }
    });

    // Study buttons on containers
    document.getElementById('containerList')?.addEventListener('click', (e) => {
        const studyBtn = e.target.closest('[data-study-btn]');
        if (studyBtn) {
            e.preventDefault();
            const state = App.getState();
            const studyType = document.getElementById('studyTypeSelect')?.value || 'periodicity';
            const panelId = state.panels[0]?.id;
            if (panelId) {
                App.addStudy(panelId, studyType, studyBtn.dataset.studyBtn);
            }
        }
    });

    // Action buttons
    document.querySelector('.action-buttons')?.addEventListener('click', (e) => {
        const btn = e.target.closest('button');
        if (!btn) return;

        switch (btn.dataset.action) {
            case 'clear':
                App.setSelectedContainers([]);
                break;
            case 'reset':
                ChartRenderer.resetZoom();
                break;
        }
    });

    // Panel tabs
    document.getElementById('panelTabs')?.addEventListener('click', (e) => {
        const addBtn = e.target.closest('[data-add-panel]');
        if (addBtn) {
            const state = App.getState();
            const lastPanel = state.panels[state.panels.length - 1];
            App.addPanel(lastPanel?.metric || state.metrics[0]?.name);
            return;
        }

        const removeBtn = e.target.closest('[data-remove-panel]');
        if (removeBtn) {
            App.removePanel(parseInt(removeBtn.dataset.removePanel));
            return;
        }
    });

    // Panel metric select changes
    document.getElementById('panelsContainer')?.addEventListener('change', (e) => {
        const select = e.target.closest('.panel-metric-select');
        if (select) {
            App.setPanelMetric(parseInt(select.dataset.panelId), select.value);
        }
    });

    // Studies panel interactions
    document.getElementById('studiesContent')?.addEventListener('click', (e) => {
        const closeBtn = e.target.closest('[data-close-study]');
        if (closeBtn) {
            const panel = e.target.closest('.study-panel');
            if (panel) {
                App.removeStudy(
                    parseInt(panel.dataset.panelId),
                    panel.dataset.studyType,
                    panel.dataset.containerId
                );
            }
            return;
        }

        // Window click for zoom
        const windowItem = e.target.closest('.period-window-item');
        if (windowItem) {
            const changepointTime = windowItem.dataset.changepointTime;
            if (changepointTime) {
                const centerMs = parseFloat(changepointTime);
                const padding = 30000;
                zoomToTimeRange(centerMs - padding, centerMs + padding);
            } else {
                const startMs = parseFloat(windowItem.dataset.windowStart);
                const endMs = parseFloat(windowItem.dataset.windowEnd);
                if (!isNaN(startMs) && !isNaN(endMs)) {
                    zoomToTimeRange(startMs, endMs);
                }
            }
        }
    });

    // Window resize
    window.addEventListener('resize', () => {
        for (const [panelId, instance] of plotInstances) {
            if (instance.main) {
                const chartDiv = document.getElementById(`mainChart-${panelId}`);
                if (chartDiv) {
                    instance.main.setSize({ width: chartDiv.clientWidth, height: chartDiv.clientHeight });
                }
            }
        }

        const overviewDiv = document.getElementById('sharedOverview');
        const firstInstance = plotInstances.values().next().value;
        if (overviewDiv && firstInstance?.overview) {
            firstInstance.overview.setSize({ width: overviewDiv.clientWidth, height: 60 });
        }
    });

    // Wheel zoom on panels container
    document.getElementById('panelsContainer')?.addEventListener('wheel', (e) => {
        const panelEl = e.target.closest('.panel');
        if (!panelEl) return;

        const panelId = parseInt(panelEl.dataset.panelId);
        const instance = plotInstances.get(panelId);
        if (!instance?.main) return;

        e.preventDefault();

        const plot = instance.main;
        const rect = plot.root.getBoundingClientRect();
        const x = e.clientX - rect.left;
        const xVal = plot.posToVal(x, 'x');

        const xRange = plot.scales.x;
        const factor = e.deltaY > 0 ? 1.2 : 0.8;
        const newMin = xVal - (xVal - xRange.min) * factor;
        const newMax = xVal + (xRange.max - xVal) * factor;

        // Apply to all panels (synced)
        for (const [pid, inst] of plotInstances) {
            if (inst.main) {
                inst.main.setScale('x', { min: newMin, max: newMax });
            }
        }
    }, { passive: false });
}

function zoomToTimeRange(startMs, endMs) {
    const startSec = startMs / 1000;
    const endSec = endMs / 1000;
    const duration = endSec - startSec;
    const padding = duration * 0.1;

    for (const [panelId, instance] of plotInstances) {
        if (instance.main) {
            instance.main.setScale('x', {
                min: startSec - padding,
                max: endSec + padding
            });
        }
    }
}

// ============================================================
// INITIALIZATION
// ============================================================

export async function initialize() {
    console.log('[UI] Initializing...');

    // Initialize theme
    ThemeManager.init();

    // Set up UI callbacks for effects
    App.setUICallbacks({
        renderPanel: ChartRenderer.renderPanel,
        renderAllPanels: ChartRenderer.renderAllPanels,
        renderSidebar: () => render(App.getState()),
        syncTimeRange: ChartRenderer.syncTimeRange,
    });

    // Subscribe render to state changes
    App.subscribe(render);

    // Set up event listeners
    setupEventListeners();

    // Initialize the app (fetches data)
    setLoading(true, 'Loading metrics...');
    try {
        const result = await App.init();

        // Update instance info
        if (result?.instanceInfo) {
            updateInstanceInfo(result.instanceInfo);
        }

        // Initial render
        render(App.getState());
        ChartRenderer.renderAllPanels();
    } finally {
        setLoading(false);
    }

    console.log('[UI] Initialization complete');
}

function updateInstanceInfo(info) {
    const container = document.getElementById('instanceInfo');
    if (!container) return;

    if (!info.pod_name) {
        container.innerHTML = '';
        return;
    }

    let nodeHtml = '';
    if (info.node_name) {
        nodeHtml = `<div class="node-name">Node: ${escapeHtml(info.node_name)}</div>`;
    }

    container.innerHTML = `
        <div class="instance-info">
            <div class="pod-name">${escapeHtml(info.pod_name)}</div>
            ${nodeHtml}
        </div>
    `;
}
