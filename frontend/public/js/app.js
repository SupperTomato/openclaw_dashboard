// OpenClaw Dashboard - Minimal JavaScript
// Handles keyboard shortcuts, SSE, notifications, and module filtering

(function() {
    'use strict';

    // ========== Keyboard Shortcuts ==========
    document.addEventListener('keydown', function(e) {
        // Skip if user is typing in an input
        if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT') {
            if (e.key === 'Escape') {
                e.target.blur();
            }
            return;
        }

        switch (e.key) {
            case '?':
                toggleModal('shortcuts-modal');
                break;
            case 'Escape':
                closeAllModals();
                break;
            case 'd':
                window.location.href = '/';
                break;
            case 'c':
                window.location.href = '/config';
                break;
            case '/':
                e.preventDefault();
                var search = document.getElementById('module-search');
                if (search) search.focus();
                break;
            case 'r':
                htmx.trigger(document.body, 'htmx:load');
                showToast('Refreshing...', 'info');
                break;
            default:
                // Number keys 1-9 to jump to modules
                var num = parseInt(e.key);
                if (num >= 1 && num <= 9) {
                    var cards = document.querySelectorAll('.module-card');
                    if (cards[num - 1]) {
                        cards[num - 1].scrollIntoView({ behavior: 'smooth', block: 'center' });
                        cards[num - 1].style.outline = '2px solid var(--accent)';
                        setTimeout(function() { cards[num - 1].style.outline = ''; }, 2000);
                    }
                }
        }
    });

    // ========== Modal ==========
    function toggleModal(id) {
        var modal = document.getElementById(id);
        if (modal) {
            modal.style.display = modal.style.display === 'none' ? 'flex' : 'none';
        }
    }

    function closeAllModals() {
        document.querySelectorAll('.modal').forEach(function(m) {
            m.style.display = 'none';
        });
        // Also close detail panel
        var detail = document.getElementById('module-detail');
        if (detail) detail.style.display = 'none';
    }

    // Close modal on backdrop click
    document.addEventListener('click', function(e) {
        if (e.target.classList.contains('modal')) {
            e.target.style.display = 'none';
        }
    });

    // ========== Module Search/Filter ==========
    window.filterModules = function(query) {
        query = query.toLowerCase().trim();
        var cards = document.querySelectorAll('.module-card');
        cards.forEach(function(card) {
            var name = (card.getAttribute('data-name') || '').toLowerCase();
            var id = (card.getAttribute('data-module') || '').toLowerCase();
            var visible = !query || name.indexOf(query) !== -1 || id.indexOf(query) !== -1;
            card.style.display = visible ? '' : 'none';
        });
    };

    // ========== Server-Sent Events ==========
    function connectSSE() {
        if (typeof EventSource === 'undefined') return;

        var evtSource = new EventSource('/api/events');

        evtSource.addEventListener('modules', function(e) {
            try {
                var modules = JSON.parse(e.data);
                updateModuleCountInNav(modules);
            } catch (err) {
                // Ignore parse errors
            }
        });

        evtSource.addEventListener('system', function(e) {
            try {
                var sys = JSON.parse(e.data);
                var uptimeEl = document.getElementById('sys-uptime');
                if (uptimeEl) uptimeEl.textContent = sys.uptime;
            } catch (err) {
                // Ignore parse errors
            }
        });

        evtSource.onerror = function() {
            evtSource.close();
            // Reconnect after 5 seconds
            setTimeout(connectSSE, 5000);
        };
    }

    function updateModuleCountInNav(modules) {
        var running = modules.filter(function(m) { return m.state === 'running'; }).length;
        var el = document.getElementById('sys-modules');
        if (el) el.textContent = running + '/' + modules.length;
    }

    // ========== Toast Notifications ==========
    window.showToast = function(message, type) {
        type = type || 'info';
        var container = document.getElementById('toast-container');
        if (!container) return;

        var toast = document.createElement('div');
        toast.className = 'toast ' + type;
        toast.textContent = message;
        container.appendChild(toast);

        setTimeout(function() {
            toast.style.opacity = '0';
            toast.style.transform = 'translateX(100%)';
            toast.style.transition = 'all 0.3s ease';
            setTimeout(function() { toast.remove(); }, 300);
        }, 4000);
    };

    // ========== Browser Notifications ==========
    function requestNotificationPermission() {
        if ('Notification' in window && Notification.permission === 'default') {
            Notification.requestPermission();
        }
    }

    window.sendBrowserNotification = function(title, body) {
        if ('Notification' in window && Notification.permission === 'granted') {
            new Notification(title, { body: body, icon: '/static/assets/icon.png' });
        }
    };

    // ========== htmx Event Handlers ==========
    document.body.addEventListener('htmx:afterSwap', function(e) {
        // Re-process any htmx elements in swapped content
    });

    document.body.addEventListener('htmx:responseError', function(e) {
        showToast('Request failed. Check your connection.', 'error');
    });

    // After htmx posts to module action endpoints, show feedback
    document.body.addEventListener('htmx:afterRequest', function(e) {
        if (e.detail.pathInfo && e.detail.pathInfo.requestPath &&
            e.detail.pathInfo.requestPath.indexOf('/api/modules/') === 0 &&
            e.detail.verb === 'post') {
            try {
                var resp = JSON.parse(e.detail.xhr.responseText);
                if (resp.success) {
                    showToast('Module ' + resp.action + ' successful', 'success');
                } else {
                    showToast('Error: ' + resp.error, 'error');
                }
            } catch (err) {
                // Ignore
            }
        }
    });

    // ========== Module Card Click ==========
    document.addEventListener('click', function(e) {
        var card = e.target.closest('.module-card');
        if (card && !e.target.closest('.btn')) {
            var moduleId = card.getAttribute('data-module');
            showModuleDetail(moduleId);
        }
    });

    function showModuleDetail(moduleId) {
        var detail = document.getElementById('module-detail');
        var title = document.getElementById('detail-title');
        var content = document.getElementById('detail-content');
        if (!detail || !content) return;

        title.textContent = 'Loading...';
        detail.style.display = 'block';

        fetch('/partial/module/' + moduleId)
            .then(function(r) { return r.text(); })
            .then(function(html) {
                content.innerHTML = html;
                var card = document.querySelector('[data-module="' + moduleId + '"]');
                if (card) {
                    title.textContent = card.getAttribute('data-name') + ' Details';
                }
            })
            .catch(function() {
                content.innerHTML = '<p class="text-danger">Failed to load module details.</p>';
            });

        detail.scrollIntoView({ behavior: 'smooth' });
    }

    // ========== Initialize ==========
    connectSSE();
    requestNotificationPermission();

    // Initial load of system stats
    var navStatus = document.getElementById('nav-status');
    if (navStatus) {
        fetch('/partial/system')
            .then(function(r) { return r.text(); })
            .then(function(html) { navStatus.innerHTML = html; });
    }

})();
