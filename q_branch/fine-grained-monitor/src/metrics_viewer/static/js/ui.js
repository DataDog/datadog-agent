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
import { groupContainersByName, getContainerColorIndex } from './state-machine.js';

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

function getContainerColor(containerId, state) {
    const index = getContainerColorIndex(state, containerId);
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
    // MetricList: Removed - metrics now selected via inline autocomplete in panel cards (REQ-MV-022, REQ-MV-023)

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
            const color = getContainerColor(containerId, state);
            const selectedClass = isSelected ? 'selected' : '';

            const lastSeenMs = c.last_seen_ms || 0;
            const isStale = lastSeenMs > 0 && (now - lastSeenMs) > ONE_HOUR_MS;
            const staleStyle = isStale ? 'opacity: 0.6;' : '';

            return `
                <label class="container-item ${selectedClass}" style="${staleStyle}">
                    <input type="checkbox"
                           value="${escapeAttr(containerId)}"
                           ${isSelected ? 'checked' : ''}
                           data-container-checkbox>
                    <span class="container-color" style="background: ${color}"></span>
                    <span class="container-id" title="${escapeAttr(containerId)}">
                        ${escapeHtml(displayName)}
                        <span style="color: #888; font-size: 0.85em; margin-left: 4px;">(${containerId.substring(0, 8)})</span>
                    </span>
                </label>
            `;
        }).join('');
    },

    // PanelTabs: Removed - replaced by PanelCards component (REQ-MV-021)

    // REQ-MV-021: Panel cards for sidebar
    PanelCards(state) {
        const { panels, maxPanels } = state;
        const canAdd = panels.length < maxPanels;
        const canRemove = panels.length > 1;

        const cards = panels.map((panel, i) => {
            const metric = panel.metric || 'No metric';
            const study = panel.study || 'none';
            const studyClearBtn = panel.study
                ? `<button class="study-clear" data-clear-study="${panel.id}" title="Clear study">&times;</button>`
                : '';

            return `
                <div class="panel-card" data-panel-id="${panel.id}">
                    <div class="panel-card-header">
                        <span class="panel-number">${i + 1}</span>
                        ${canRemove ? `<button class="panel-remove" data-remove-panel="${panel.id}" title="Remove panel">&times;</button>` : ''}
                    </div>
                    <div class="panel-card-body">
                        <div class="panel-field-row">
                            <label>Metric:</label>
                            <span class="panel-metric-value editable" data-edit-metric="${panel.id}" title="Click to edit">
                                ${escapeHtml(metric)}
                            </span>
                        </div>
                        <div class="panel-field-row">
                            <label>Study:</label>
                            <span class="panel-study-value editable" data-edit-study="${panel.id}" title="Click to edit">
                                ${escapeHtml(study)}
                            </span>
                            ${studyClearBtn}
                        </div>
                    </div>
                </div>
            `;
        }).join('');

        const addButton = canAdd
            ? `<button class="add-panel-btn" data-add-panel title="Add panel">+ Add Panel</button>`
            : '';

        return `
            ${cards}
            ${addButton}
        `;
    },

    // Legacy study panel components removed (REQ-MV-030)
    // Studies now displayed as chart annotations with tooltips

    Legend(state, panelId) {
        if (state.selectedContainerIds.length === 0) return '';

        // When grouping is enabled, show one legend entry per unique name
        if (state.groupByContainerName) {
            const groups = groupContainersByName(state);
            let groupIndex = 0;
            const legendItems = [];

            for (const [displayName, containerIds] of groups) {
                const count = containerIds.length;
                const firstContainer = state.containers.find(c => c.short_id === containerIds[0]);
                const qos = firstContainer?.qos_class || '?';
                const countLabel = count > 1 ? ` (${count})` : '';
                const allIds = containerIds.join(', ');

                legendItems.push(`
                    <div class="legend-item">
                        <div class="legend-color" style="background: ${COLORS[groupIndex % COLORS.length]}"></div>
                        <span title="${escapeAttr(allIds)}">${escapeHtml(displayName)}${countLabel}</span>
                    </div>
                `);
                groupIndex++;
            }
            return legendItems.join('');
        }

        // Non-grouped: one entry per container
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
// INLINE AUTOCOMPLETE (REQ-MV-022, REQ-MV-023, REQ-MV-029)
// ============================================================

let activeAutocomplete = null;

/**
 * Show inline autocomplete dropdown.
 * @param {HTMLElement} targetElement - Element to replace with input
 * @param {Object} options - { items, currentValue, fuzzyMatch, placeholder, onSelect, onCancel }
 */
function showInlineAutocomplete(targetElement, options) {
    // Close any existing autocomplete
    if (activeAutocomplete) {
        closeAutocomplete();
    }

    const { items, currentValue, fuzzyMatch, placeholder, onSelect, onCancel } = options;

    // Create input element
    const input = document.createElement('input');
    input.type = 'text';
    input.value = '';
    input.placeholder = placeholder || 'Type to search...';
    input.className = 'inline-autocomplete-input';
    input.style.cssText = `
        width: 100%;
        padding: 4px 6px;
        border: 1px solid var(--border-accent);
        border-radius: 3px;
        background: var(--bg-secondary);
        color: var(--text-primary);
        font-family: monospace;
        font-size: 12px;
        outline: none;
    `;

    // Create dropdown
    const dropdown = document.createElement('div');
    dropdown.className = 'inline-autocomplete-dropdown';
    dropdown.style.cssText = `
        position: absolute;
        z-index: 1000;
        background: var(--bg-secondary);
        border: 1px solid var(--border-primary);
        border-radius: 4px;
        max-height: 200px;
        overflow-y: auto;
        box-shadow: 0 4px 6px rgba(0,0,0,0.1);
        min-width: 200px;
    `;

    let filteredItems = items;
    let selectedIndex = -1;

    function fuzzyMatchFn(query, text) {
        query = query.toLowerCase();
        text = text.toLowerCase();
        let qi = 0;
        for (let ti = 0; ti < text.length && qi < query.length; ti++) {
            if (text[ti] === query[qi]) qi++;
        }
        return qi === query.length;
    }

    function updateDropdown() {
        const query = input.value.trim();
        filteredItems = query && fuzzyMatch
            ? items.filter(item => fuzzyMatchFn(query, item))
            : items;

        dropdown.innerHTML = filteredItems.slice(0, 10).map((item, i) => `
            <div class="autocomplete-item ${i === selectedIndex ? 'selected' : ''}"
                 data-index="${i}"
                 style="padding: 6px 10px; cursor: pointer; ${i === selectedIndex ? 'background: var(--bg-hover);' : ''}">
                ${escapeHtml(item)}
            </div>
        `).join('');

        if (filteredItems.length === 0) {
            dropdown.innerHTML = '<div style="padding: 6px 10px; color: var(--text-muted);">No matches</div>';
        }
    }

    function selectItem(index) {
        if (index >= 0 && index < filteredItems.length) {
            onSelect(filteredItems[index]);
            closeAutocomplete();
        }
    }

    function closeAutocomplete() {
        if (activeAutocomplete) {
            activeAutocomplete.input?.remove();
            activeAutocomplete.dropdown?.remove();
            activeAutocomplete = null;
        }
    }

    // Event handlers
    input.addEventListener('input', () => {
        selectedIndex = -1;
        updateDropdown();
        positionDropdown();
    });

    input.addEventListener('keydown', (e) => {
        if (e.key === 'ArrowDown') {
            e.preventDefault();
            selectedIndex = Math.min(selectedIndex + 1, filteredItems.length - 1);
            updateDropdown();
        } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            selectedIndex = Math.max(selectedIndex - 1, -1);
            updateDropdown();
        } else if (e.key === 'Enter') {
            e.preventDefault();
            if (selectedIndex >= 0) {
                selectItem(selectedIndex);
            } else if (filteredItems.length === 1) {
                selectItem(0);
            }
        } else if (e.key === 'Escape') {
            e.preventDefault();
            onCancel();
            closeAutocomplete();
        }
    });

    input.addEventListener('blur', (e) => {
        // Delay to allow dropdown click to register
        setTimeout(() => {
            if (activeAutocomplete) {
                onCancel();
                closeAutocomplete();
            }
        }, 200);
    });

    dropdown.addEventListener('click', (e) => {
        const item = e.target.closest('.autocomplete-item');
        if (item) {
            const index = parseInt(item.dataset.index);
            selectItem(index);
        }
    });

    dropdown.addEventListener('mouseenter', (e) => {
        const item = e.target.closest('.autocomplete-item');
        if (item) {
            selectedIndex = parseInt(item.dataset.index);
            updateDropdown();
        }
    });

    function positionDropdown() {
        const rect = input.getBoundingClientRect();
        dropdown.style.top = `${rect.bottom + window.scrollY + 2}px`;
        dropdown.style.left = `${rect.left + window.scrollX}px`;
        dropdown.style.width = `${Math.max(rect.width, 200)}px`;
    }

    // Replace target with input
    targetElement.replaceWith(input);
    document.body.appendChild(dropdown);

    // Initial render and position
    updateDropdown();
    positionDropdown();
    input.focus();

    activeAutocomplete = { input, dropdown, targetElement };
}

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

        // Clear and rebuild panels HTML structure (REQ-MV-023: removed metric dropdown)
        panelsContainer.innerHTML = state.panels.map(panel => `
            <div class="panel" data-panel-id="${panel.id}">
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

        console.log('[renderPanel] Panel:', panelId, 'Metric:', panel.metric);
        console.log('[renderPanel] selectedContainerIds count:', containerIds.length);
        console.log('[renderPanel] selectedContainerIds sample:', containerIds.slice(0, 3));
        console.log('[renderPanel] timeseries keys:', Object.keys(timeseries));
        console.log('[renderPanel] timeseries keys count:', Object.keys(timeseries).length);

        // Build uPlot data
        const allTimes = new Set();
        containerIds.forEach(id => {
            const data = timeseries[id] || [];
            data.forEach(p => allTimes.add(p.time_ms));
        });
        const timestamps = Array.from(allTimes).sort((a, b) => a - b);

        console.log('[renderPanel] timestamps count:', timestamps.length);

        if (timestamps.length === 0) {
            console.log('[renderPanel] No timestamps - showing empty state');
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

        // Build series config - use grouping-aware colors
        const uplotSeries = [
            { label: 'Time' },
            ...containerIds.map((id) => {
                const colorIdx = getContainerColorIndex(state, id);
                return {
                    label: id,
                    stroke: COLORS[colorIdx >= 0 ? colorIdx % COLORS.length : 0],
                    width: 1.5,
                    points: { show: false }
                };
            })
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
            ...containerIds.map((id) => {
                const colorIdx = getContainerColorIndex(state, id);
                return {
                    label: id,
                    stroke: COLORS[colorIdx >= 0 ? colorIdx % COLORS.length : 0],
                    width: 1
                };
            })
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

    // REQ-MV-030: Draw study overlays on chart
    drawStudyOverlay(u, panel) {
        const state = App.getState();
        if (!panel.study) return;

        const ctx = u.ctx;
        const { left, top, width, height } = u.bbox;
        const plotRight = left + width;
        const plotBottom = top + height;

        ctx.save();
        ctx.beginPath();
        ctx.rect(left, top, width, height);
        ctx.clip();

        // Draw study results for all selected containers
        for (const containerId of state.selectedContainerIds) {
            const key = DataStore.studyKey(panel.metric, containerId, panel.study);
            const result = DataStore.getStudyResult(key);
            if (!result?.windows) continue;

            const colorIndex = getContainerColorIndex(state, containerId);
            const color = COLORS[colorIndex >= 0 ? colorIndex % COLORS.length : 0];

            if (panel.study === 'changepoint') {
                ChartRenderer.drawChangepointMarkers(ctx, u, result.windows, color, left, top, plotRight, plotBottom);
            } else if (panel.study === 'periodicity') {
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
    // REQ-MV-037: Update time range selector
    const timeRangeSelect = document.getElementById('timeRangeSelect');
    if (timeRangeSelect) {
        timeRangeSelect.value = state.dataTimeRange || '1h';
        // Disable when dashboard controls time range
        const dashboardControlled = state.dashboard?.time_range != null;
        timeRangeSelect.disabled = dashboardControlled;
        timeRangeSelect.title = dashboardControlled
            ? 'Time range controlled by dashboard'
            : 'Select time range for data';
    }

    // Update panel cards (REQ-MV-021)
    const panelCardsContainer = document.getElementById('panelCards');
    if (panelCardsContainer) {
        panelCardsContainer.innerHTML = Components.PanelCards(state);
    }

    // Update container list
    const containerList = document.getElementById('containerList');
    if (containerList) {
        containerList.innerHTML = Components.ContainerList(state);
    }
}

// ============================================================
// EVENT SETUP
// ============================================================

export function setupEventListeners() {
    // REQ-MV-037: Time range selector
    document.getElementById('timeRangeSelect')?.addEventListener('change', (e) => {
        const state = App.getState();
        // REQ-MV-037: Ignore when dashboard controls time range
        if (state.dashboard?.time_range) {
            // Reset to dashboard value and ignore
            e.target.value = state.dataTimeRange;
            return;
        }
        App.setDataTimeRange(e.target.value);
    });

    // Legacy metric list/search listeners removed - now using inline autocomplete (REQ-MV-022, REQ-MV-023)

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

    // Study buttons: Removed - studies now managed via panel cards (REQ-MV-029)

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

    // Panel cards (REQ-MV-021, 022, 023, 029, 030)
    document.getElementById('panelCards')?.addEventListener('click', (e) => {
        // Add panel button (REQ-MV-022)
        const addBtn = e.target.closest('[data-add-panel]');
        if (addBtn) {
            const state = App.getState();

            // Create temporary card with autocomplete
            const panelCardsContainer = document.getElementById('panelCards');
            if (!panelCardsContainer) return;

            // Create temp span for autocomplete to attach to
            const tempSpan = document.createElement('span');
            tempSpan.className = 'panel-metric-value editable';
            tempSpan.textContent = 'Select metric...';
            tempSpan.style.cssText = 'padding: 4px 6px; display: inline-block;';

            // Insert before the "+ Add Panel" button
            addBtn.parentElement.insertBefore(tempSpan, addBtn);

            showInlineAutocomplete(tempSpan, {
                items: state.metrics.map(m => m.name),
                currentValue: null,
                fuzzyMatch: true,
                placeholder: 'Type to search metrics...',
                onSelect: (metric) => {
                    App.addPanel(metric);
                },
                onCancel: () => {
                    // Just re-render to remove temp element
                    render(App.getState());
                }
            });
            return;
        }

        // Remove panel button
        const removeBtn = e.target.closest('[data-remove-panel]');
        if (removeBtn) {
            App.removePanel(parseInt(removeBtn.dataset.removePanel));
            return;
        }

        // Clear study button
        const clearStudyBtn = e.target.closest('[data-clear-study]');
        if (clearStudyBtn) {
            App.removeStudy(parseInt(clearStudyBtn.dataset.clearStudy));
            return;
        }

        // Edit metric (REQ-MV-023)
        const editMetric = e.target.closest('[data-edit-metric]');
        if (editMetric) {
            const panelId = parseInt(editMetric.dataset.editMetric);
            const state = App.getState();
            const panel = state.panels.find(p => p.id === panelId);
            if (!panel) return;

            showInlineAutocomplete(editMetric, {
                items: state.metrics.map(m => m.name),
                currentValue: panel.metric,
                fuzzyMatch: true,
                placeholder: 'Type to search metrics...',
                onSelect: (metric) => {
                    App.setPanelMetric(panelId, metric);
                },
                onCancel: () => {
                    render(App.getState());
                }
            });
            return;
        }

        // Edit study (REQ-MV-029)
        const editStudy = e.target.closest('[data-edit-study]');
        if (editStudy) {
            const panelId = parseInt(editStudy.dataset.editStudy);
            const state = App.getState();

            if (state.selectedContainerIds.length === 0) {
                alert('Please select containers first');
                return;
            }

            showInlineAutocomplete(editStudy, {
                items: ['periodicity', 'changepoint'],
                currentValue: null,
                fuzzyMatch: false,
                placeholder: 'Select study type...',
                onSelect: (studyType) => {
                    App.addStudy(panelId, studyType);
                },
                onCancel: () => {
                    render(App.getState());
                }
            });
            return;
        }
    });

    // Legacy event listeners removed:
    // - Panel tabs: Removed - replaced by panel cards (REQ-MV-021)
    // - Panel metric select: Removed - now edited via sidebar (REQ-MV-023)
    // - Studies panel: Removed - now chart annotations (REQ-MV-030)

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
