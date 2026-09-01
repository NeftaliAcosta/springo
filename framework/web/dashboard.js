let allBeans = [];
        let allEnv = [];

        function switchTab(tabId, btn) {
            if (!btn) {
                btn = Array.from(document.querySelectorAll('.tab-btn')).find(b => b.getAttribute('onclick') && b.getAttribute('onclick').includes(tabId));
            }
            if (!btn) return;

            document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));
            document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
            
            document.getElementById(tabId).classList.add('active');
            btn.classList.add('active');

            if (window.location.hash !== '#' + tabId) {
                history.replaceState(null, null, '#' + tabId);
            }

            if (tabId === 'tab-thread') loadThreadDump();
        }

        async function apiGet(path) {
            const r = await fetch(path);
            if (!r.ok) throw new Error(`API error: ${r.status}`);
            return r.json();
        }

        async function apiPost(path, body = {}) {
            const r = await fetch(path, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body)
            });
            return r.ok;
        }

        let uptimeSeconds = 0;
        let uptimeInterval = null;

        function parseGoDuration(durationStr) {
            if (!durationStr || durationStr === '---') return 0;
            let totalSeconds = 0;
            const regex = /(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?/;
            const matches = durationStr.match(regex);
            if (matches) {
                const hours = parseInt(matches[1] || 0, 10);
                const minutes = parseInt(matches[2] || 0, 10);
                const seconds = parseInt(matches[3] || 0, 10);
                totalSeconds = (hours * 3600) + (minutes * 60) + seconds;
            }
            return totalSeconds;
        }

        function formatDuration(totalSeconds) {
            const hours = Math.floor(totalSeconds / 3600);
            const minutes = Math.floor((totalSeconds % 3600) / 60);
            const seconds = totalSeconds % 60;
            
            let parts = [];
            if (hours > 0) parts.push(`${hours}h`);
            if (minutes > 0) parts.push(`${minutes}m`);
            if (seconds > 0 || parts.length === 0) parts.push(`${seconds}s`);
            return parts.join('');
        }

        function startUptimeTicker(initialDurationStr) {
            uptimeSeconds = parseGoDuration(initialDurationStr);
            document.getElementById('stat-uptime').textContent = formatDuration(uptimeSeconds);
            
            if (uptimeInterval) clearInterval(uptimeInterval);
            uptimeInterval = setInterval(() => {
                uptimeSeconds++;
                document.getElementById('stat-uptime').textContent = formatDuration(uptimeSeconds);
            }, 1000);
        }

        async function refreshAll() {
            await Promise.all([
                loadDashboard(),
                loadLoggers(),
                loadTasks(),
                loadDLQ(),
                loadBeans(),
                loadEnv()
            ]);
        }

        async function loadDashboard() {
            try {
                const info = await apiGet('/actuator/health');
                const statHealth = document.getElementById('stat-health');
                statHealth.textContent = info.status;
                statHealth.className = 'stat-value ' + (
                    info.status === 'UP' ? 'status-badge status-up' : 
                    info.status === 'DEGRADED' ? 'status-badge status-degraded' : 'status-badge status-down'
                );

                startUptimeTicker(info.uptime || '0s');

                if (info.system) {
                    document.getElementById('stat-goroutines').textContent = info.system.goroutines || '---';
                    document.getElementById('stat-memory').textContent = info.system.memory_usage || '---';
                    document.getElementById('sys-go-version').textContent = info.system.go_version || '---';
                    document.getElementById('sys-platform').textContent = info.system.platform || '---';
                }

                // Render databases
                const dbContainer = document.getElementById('db-health-container');
                dbContainer.innerHTML = '';
                
                if (info.components) {
                    const keys = Object.keys(info.components);
                    if (keys.length === 0) {
                        dbContainer.innerHTML = `<p class="text-secondary">No DB component health data collected.</p>`;
                    } else {
                        keys.forEach(key => {
                            const db = info.components[key];
                            const card = document.createElement('div');
                            card.style.marginBottom = '1rem';
                            card.style.padding = '0.75rem';
                            card.style.background = 'rgba(255, 255, 255, 0.02)';
                            card.style.borderRadius = '8px';
                            let detailText = 'No stats available.';
                            if (db.details) {
                                if (db.details.ping_time !== undefined) {
                                    detailText = `Ping: ${db.details.ping_time || '---'} | Pool Open/Active: ${db.details.open || 0}/${db.details.in_use || 0}`;
                                } else if (db.details.cache) {
                                    detailText = `Provider: ${db.details.cache} | Status: ${db.details.status || 'connected'}`;
                                } else {
                                    detailText = Object.entries(db.details).map(([k,v]) => `${k}: ${v}`).join(' | ');
                                }
                            }
                            card.innerHTML = `
                                <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:0.5rem;">
                                    <strong>${key}</strong>
                                    <span class="status-badge ${db.status === 'UP' ? 'status-up' : 'status-down'}">${db.status}</span>
                                </div>
                                <div style="font-size:0.8rem; color:var(--text-secondary)">
                                    ${detailText}
                                </div>
                            `;
                            dbContainer.appendChild(card);
                        });
                    }
                }
            } catch (err) {
                console.error(err);
            }
        }

        async function loadLoggers() {
            try {
                const data = await apiGet('/actuator/loggers');
                document.getElementById('global-log-level').value = data.level || 'INFO';
            } catch (err) {
                console.error(err);
            }
        }

        async function changeLogLevel(level) {
            try {
                const ok = await apiPost('/actuator/loggers', { configuredLevel: level });
                if (ok) alert(`Log level updated to ${level}`);
            } catch (err) {
                alert(`Failed to update log level: ${err}`);
            }
        }

        async function loadTasks() {
            try {
                const list = await apiGet('/actuator/shedlock');
                const tbody = document.getElementById('scheduler-table-body');
                tbody.innerHTML = '';
                
                if (!list || list.length === 0) {
                    tbody.innerHTML = `<tr><td colspan="8" class="text-secondary" style="text-align:center;">No scheduled tasks found.</td></tr>`;
                    return;
                }

                list.forEach(t => {
                    const lastRun = t.last_executed && !t.last_executed.startsWith('0001-01-01')
                        ? new Date(t.last_executed).toLocaleTimeString()
                        : 'Never';
                    const nextRun = t.next_expected && !t.next_expected.startsWith('0001-01-01')
                        ? new Date(t.next_expected).toLocaleTimeString()
                        : 'Never';

                    const row = document.createElement('tr');
                    row.innerHTML = `
                        <td><strong>${t.name}</strong></td>
                        <td><code style="font-family:'JetBrains Mono'; color:var(--brand-color)">${t.cron || t.fixed_rate || t.fixed_delay || 'Manual'}</code></td>
                        <td>${t.run_on_startup ? '✅' : '❌'} (P${t.priority})</td>
                        <td>${t.critical ? '⚠️ Yes' : 'No'}</td>
                        <td>${t.lock_enabled ? '🔐 Enabled' : '🔓 Disabled'}</td>
                        <td>${lastRun}</td>
                        <td>${nextRun}</td>
                        <td style="text-align: right;">
                            <button class="btn-table" onclick="triggerJob('${t.name}')">⚡ Trigger</button>
                        </td>
                    `;
                    tbody.appendChild(row);
                });
            } catch (err) {
                console.error(err);
            }
        }

        async function triggerJob(jobName) {
            try {
                const ok = await apiPost('/actuator/shedlock/trigger', { jobName: jobName });
                if (ok) {
                    alert(`Job '${jobName}' triggered manually.`);
                    setTimeout(loadTasks, 500);
                }
            } catch (err) {
                alert(`Failed to trigger job: ${err}`);
            }
        }

        async function loadDLQ() {
            try {
                const list = await apiGet('/actuator/dlq');
                const tbody = document.getElementById('dlq-table-body');
                tbody.innerHTML = '';

                if (!list || list.length === 0) {
                    tbody.innerHTML = `<tr><td colspan="7" class="text-secondary" style="text-align:center;">Dead Letter Queue is empty.</td></tr>`;
                    return;
                }

                list.forEach(item => {
                    const row = document.createElement('tr');
                    row.innerHTML = `
                        <td>${item.id}</td>
                        <td><strong>${item.event_name}</strong></td>
                        <td><span style="font-size:0.8rem">${item.listener_name || 'Default'}</span></td>
                        <td><span style="font-size:0.8rem; color:var(--danger-text)">${item.error}</span></td>
                        <td><pre>${item.payload}</pre></td>
                        <td style="text-align: center;">${item.retries}</td>
                        <td style="text-align: right;">
                            <div style="display:flex; gap:0.4rem; justify-content: flex-end;">
                                <button class="btn-table" onclick="retryDLQ(${item.id})">Retry</button>
                                <button class="btn-table btn-table-danger" onclick="purgeDLQ(${item.id})">Purge</button>
                            </div>
                        </td>
                    `;
                    tbody.appendChild(row);
                });
            } catch (err) {
                console.error(err);
            }
        }

        async function retryDLQ(id) {
            try {
                const ok = await apiPost(`/actuator/dlq/retry?id=${id}`);
                if (ok) {
                    alert('Event successfully queued back to EventBus!');
                    loadDLQ();
                }
            } catch (err) {
                alert(`Failed to retry event: ${err}`);
            }
        }

        async function purgeDLQ(id) {
            if (!confirm('Are you sure you want to delete this failed event?')) return;
            try {
                const ok = await apiPost(`/actuator/dlq/purge?id=${id}`);
                if (ok) {
                    loadDLQ();
                }
            } catch (err) {
                alert(`Failed to purge event: ${err}`);
            }
        }

        async function purgeAllDLQ() {
            if (!confirm('Are you sure you want to PURGE ALL failed events in the DLQ? This action is irreversible.')) return;
            try {
                const ok = await apiPost('/actuator/dlq/purge');
                if (ok) {
                    loadDLQ();
                }
            } catch (err) {
                alert(`Failed to purge DLQ: ${err}`);
            }
        }

        async function loadBeans() {
            try {
                allBeans = await apiGet('/actuator/beans');
                filterBeans();
            } catch (err) {
                console.error(err);
            }
        }

        function filterBeans() {
            const query = document.getElementById('bean-search').value.toLowerCase();
            const tbody = document.getElementById('beans-table-body');
            tbody.innerHTML = '';

            const filtered = allBeans.filter(b => b.name.toLowerCase().includes(query) || b.type.toLowerCase().includes(query));
            filtered.forEach(b => {
                const row = document.createElement('tr');
                row.innerHTML = `
                    <td><strong>${b.name}</strong></td>
                    <td><code style="font-family:'JetBrains Mono'; font-size:0.8rem">${b.type}</code></td>
                    <td><span class="logo-badge" style="font-size:0.7rem; border-color:var(--text-secondary); color:var(--text-secondary)">${b.scope}</span></td>
                `;
                tbody.appendChild(row);
            });
        }

        async function loadEnv() {
            try {
                const data = await apiGet('/actuator/env');
                allEnv = [];
                for (const prefix in data) {
                    const block = data[prefix];
                    flattenEnv(prefix, block);
                }
                filterEnv();
            } catch (err) {
                console.error(err);
            }
        }

        function flattenEnv(prefix, obj) {
            if (obj === null) return;
            if (typeof obj !== 'object') {
                allEnv.push({ key: prefix, value: obj });
                return;
            }
            for (const key in obj) {
                const val = obj[key];
                const fullKey = prefix ? `${prefix}.${key}` : key;
                if (typeof val === 'object' && val !== null) {
                    flattenEnv(fullKey, val);
                } else {
                    allEnv.push({ key: fullKey, value: val });
                }
            }
        }

        function filterEnv() {
            const query = document.getElementById('env-search').value.toLowerCase();
            const tbody = document.getElementById('env-table-body');
            tbody.innerHTML = '';

            const filtered = allEnv.filter(e => e.key.toLowerCase().includes(query) || String(e.value).toLowerCase().includes(query));
            filtered.forEach(e => {
                const row = document.createElement('tr');
                row.innerHTML = `
                    <td><code style="font-family:'JetBrains Mono'; font-size:0.8rem">${e.key}</code></td>
                    <td class="text-secondary"><code style="font-family:'JetBrains Mono'; font-size:0.8rem; color:#f8fafc">${e.value}</code></td>
                `;
                tbody.appendChild(row);
            });
        }

        async function loadThreadDump() {
            const container = document.getElementById('threaddump-container');
            container.textContent = 'Loading stack...';
            try {
                const res = await fetch('/actuator/threaddump');
                const text = await res.text();
                container.textContent = text;
            } catch (err) {
                container.textContent = `Error loading stack trace: ${err}`;
            }
        }

        // Init
        document.getElementById('sys-cores').textContent = navigator.hardwareConcurrency || '---';
        
        // Handle hash navigation on page load
        const initialTab = window.location.hash ? window.location.hash.substring(1) : 'tab-dashboard';
        if (document.getElementById(initialTab)) {
            switchTab(initialTab);
        }
        
        refreshAll();