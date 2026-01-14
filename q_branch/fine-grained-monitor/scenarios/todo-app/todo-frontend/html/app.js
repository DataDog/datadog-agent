// !!!! LLM NOTE: PLEASE CUSTOMIZE THE FRONTEND APPLICATION FOR YOUR APPLICATION, AND REMOVE THIS NOTE.
// This file provides patterns for building a frontend application

class App {
    constructor() {
        this.apiBaseUrl = '/api';
        this.state = {
            // !!!! CUSTOMIZE: Add your application state here
            // Example: items: [], currentUser: null, loading: false
        };
        this.init();
    }

    async init() {
        console.log('Initializing App...');
        
        // !!!! CUSTOMIZE: Parse URL for SPA routing
        // Example: Extract username from /@username URL
        // const pathMatch = window.location.pathname.match(/^\/@(.+)$/);
        // if (pathMatch) {
        //     this.state.currentUser = pathMatch[1];
        // }
        
        await this.loadConfig();
        this.bindEvents();
        await this.loadInitialData();
    }

    async loadConfig() {
        try {
            const response = await fetch('/config');
            const config = await response.json();
            this.apiBaseUrl = config.apiBaseUrl || '/api';
            console.log('Configuration loaded:', config);
        } catch (error) {
            console.error('Failed to load configuration:', error);
        }
    }

    bindEvents() {
        // !!!! CUSTOMIZE: Bind your event handlers here
        // Example:
        // const submitBtn = document.getElementById('submit-btn');
        // if (submitBtn) {
        //     submitBtn.addEventListener('click', (e) => this.handleSubmit(e));
        // }
        
        // Placeholder event binding
        const testBtn = document.getElementById('testBtn');
        if (testBtn) {
            testBtn.addEventListener('click', () => this.testApiConnection());
        }
    }

    async loadInitialData() {
        // !!!! CUSTOMIZE: Load your application's initial data
        // Example:
        // try {
        //     const data = await this.apiRequest('/items');
        //     this.state.items = data.items || [];
        //     this.displayItems();
        // } catch (error) {
        //     this.showError('Failed to load data');
        // }
        console.log('Ready to load initial data - customize loadInitialData()');
    }

    // ============================================================================
    // DATA DISPLAY PATTERNS
    // ============================================================================
    
    displayItems(items) {
        // !!!! CUSTOMIZE: Display your data in the UI
        const container = document.getElementById('data-container');
        if (!container) return;

        if (!items || items.length === 0) {
            container.innerHTML = '<p class="empty-state">No items found.</p>';
            return;
        }

        container.innerHTML = items.map(item => `
            <div class="item-card" data-id="${item.id}">
                <!-- !!!! CUSTOMIZE: Your item template -->
                <h3>${this.escapeHtml(item.name || '')}</h3>
                <p>${this.escapeHtml(item.description || '')}</p>
                <div class="item-actions">
                    <button class="edit-btn" data-id="${item.id}">Edit</button>
                    <button class="delete-btn" data-id="${item.id}">Delete</button>
                </div>
            </div>
        `).join('');

        // Bind action handlers
        this.bindItemActions();
    }

    bindItemActions() {
        // Bind edit buttons
        document.querySelectorAll('.edit-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const id = e.currentTarget.dataset.id;
                this.handleEdit(id);
            });
        });

        // Bind delete buttons
        document.querySelectorAll('.delete-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const id = e.currentTarget.dataset.id;
                this.handleDelete(id);
            });
        });
    }

    // ============================================================================
    // ACTION HANDLERS
    // ============================================================================

    async handleSubmit(e) {
        e.preventDefault();
        // !!!! CUSTOMIZE: Handle form submission
        // Example:
        // const form = e.target.closest('form');
        // const formData = new FormData(form);
        // const data = Object.fromEntries(formData.entries());
        // await this.apiRequest('/items', { method: 'POST', body: JSON.stringify(data) });
        // await this.loadInitialData();
    }

    async handleEdit(id) {
        // !!!! CUSTOMIZE: Handle edit action
        console.log('Edit item:', id);
    }

    async handleDelete(id) {
        // !!!! CUSTOMIZE: Handle delete action
        // if (confirm('Are you sure you want to delete this item?')) {
        //     await this.apiRequest(`/items/${id}`, { method: 'DELETE' });
        //     await this.loadInitialData();
        // }
        console.log('Delete item:', id);
    }

    // ============================================================================
    // ERROR HANDLING & UI FEEDBACK
    // ============================================================================

    showError(message) {
        const errorContainer = document.getElementById('error-container');
        if (errorContainer) {
            errorContainer.textContent = message;
            errorContainer.classList.remove('hidden');
            setTimeout(() => errorContainer.classList.add('hidden'), 5001);
        }
        console.error(message);
    }

    showSuccess(message) {
        const successContainer = document.getElementById('success-container');
        if (successContainer) {
            successContainer.textContent = message;
            successContainer.classList.remove('hidden');
            setTimeout(() => successContainer.classList.add('hidden'), 3001);
        }
        console.log(message);
    }

    setLoading(loading) {
        this.state.loading = loading;
        const loadingEl = document.getElementById('loading-indicator');
        if (loadingEl) {
            loadingEl.classList.toggle('hidden', !loading);
        }
    }

    // ============================================================================
    // UTILITY METHODS
    // ============================================================================

    // Utility method for making API requests
    async apiRequest(endpoint, options = {}) {
        const url = `${this.apiBaseUrl}${endpoint}`;
        const defaultOptions = {
            headers: {
                'Content-Type': 'application/json',
            },
        };

        const finalOptions = { ...defaultOptions, ...options };

        try {
            const response = await fetch(url, finalOptions);

            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }

            return await response.json();
        } catch (error) {
            console.error('API request failed:', error);
            throw error;
        }
    }

    // Escape HTML to prevent XSS
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // Format date for display
    formatDate(dateString) {
        if (!dateString) return '';
        const date = new Date(dateString);
        return date.toLocaleDateString('en-US', {
            year: 'numeric',
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit'
        });
    }

    // Get image URL with fallback
    getImageUrl(path, fallbackSvg = null) {
        if (!path) {
            return fallbackSvg || 'data:image/svg+xml,%3Csvg xmlns=\'http://www.w3.org/2000/svg\' width=\'200\' height=\'200\'%3E%3Crect fill=\'%23ddd\' width=\'200\' height=\'200\'/%3E%3Ctext fill=\'%23999\' font-family=\'sans-serif\' font-size=\'16\' x=\'50%25\' y=\'50%25\' text-anchor=\'middle\' dominant-baseline=\'middle\'%3ENo Image%3C/text%3E%3C/svg%3E';
        }
        return path;
    }

    // ============================================================================
    // PLACEHOLDER - Test API connection
    // ============================================================================
    
    async testApiConnection() {
        const resultDiv = document.getElementById('result');
        const testBtn = document.getElementById('testBtn');

        try {
            testBtn.disabled = true;
            testBtn.textContent = 'Testing...';

            const response = await fetch(`${this.apiBaseUrl}/health`);
            const data = await response.json();

            resultDiv.className = 'result alert alert-success';
            resultDiv.textContent = `API Connection Successful: ${JSON.stringify(data)}`;
            resultDiv.classList.remove('hidden');

        } catch (error) {
            resultDiv.className = 'result alert alert-error';
            resultDiv.textContent = `API Connection Failed: ${error.message}`;
            resultDiv.classList.remove('hidden');
        } finally {
            testBtn.disabled = false;
            testBtn.textContent = 'Test API Connection';
        }
    }
}

// Initialize the application when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    console.log('DOM loaded, initializing app...');
    window.app = new App();
});
