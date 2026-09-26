// State variables
let socket = null;
let trafficChart = null;

// Real-time chart data arrays
const maxChartPoints = 30;
const chartLabels = Array(maxChartPoints).fill('');
const chartAllowedData = Array(maxChartPoints).fill(0);
const chartBlockedData = Array(maxChartPoints).fill(0);

// Temporary counters for current second (updated by incoming logs)
let allowedThisSecond = 0;
let blockedThisSecond = 0;

// Initialize app when DOM loads
document.addEventListener('DOMContentLoaded', () => {
    initChart();
    fetchConfig();
    fetchBans();
    connectWebSocket();
    setupEventListeners();

    // Periodically refresh ban list (every 5 seconds)
    setInterval(fetchBans, 5000);

    // Periodically update the chart dataset (every 1 second)
    setInterval(updateChartData, 1000);
});

// Setup Chart.js
function initChart() {
    const ctx = document.getElementById('trafficChart').getContext('2d');
    
    // Create gradient fills
    const greenGradient = ctx.createLinearGradient(0, 0, 0, 200);
    greenGradient.addColorStop(0, 'rgba(16, 185, 129, 0.2)');
    greenGradient.addColorStop(1, 'rgba(16, 185, 129, 0.0)');

    const redGradient = ctx.createLinearGradient(0, 0, 0, 200);
    redGradient.addColorStop(0, 'rgba(239, 68, 68, 0.2)');
    redGradient.addColorStop(1, 'rgba(239, 68, 68, 0.0)');

    trafficChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: chartLabels,
            datasets: [
                {
                    label: 'Allowed',
                    data: chartAllowedData,
                    borderColor: '#10b981',
                    backgroundColor: greenGradient,
                    fill: true,
                    tension: 0.3,
                    borderWidth: 2,
                    pointRadius: 0,
                    pointHoverRadius: 4,
                },
                {
                    label: 'Blocked',
                    data: chartBlockedData,
                    borderColor: '#ef4444',
                    backgroundColor: redGradient,
                    fill: true,
                    tension: 0.3,
                    borderWidth: 2,
                    pointRadius: 0,
                    pointHoverRadius: 4,
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: {
                    display: true,
                    labels: {
                        color: '#94a3b8',
                        font: { family: 'Outfit', size: 12 }
                    }
                },
                tooltip: {
                    mode: 'index',
                    intersect: false,
                }
            },
            scales: {
                x: {
                    grid: { display: false },
                    ticks: { display: false }
                },
                y: {
                    grid: { color: 'rgba(255, 255, 255, 0.04)' },
                    ticks: {
                        color: '#94a3b8',
                        font: { family: 'JetBrains Mono', size: 10 },
                        stepSize: 10,
                        beginAtZero: true
                    }
                }
            }
        }
    });
}

// Push local counter values to Chart and shift array
function updateChartData() {
    // Push values
    chartAllowedData.push(allowedThisSecond);
    chartBlockedData.push(blockedThisSecond);

    // Keep size consistent
    if (chartAllowedData.length > maxChartPoints) {
        chartAllowedData.shift();
        chartBlockedData.shift();
    }

    // Reset counters for next second
    allowedThisSecond = 0;
    blockedThisSecond = 0;

    // Update chart
    if (trafficChart) {
        trafficChart.update('none'); // Update without animation for performance
    }
}

// Connect to real-time WebSocket channel
function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/ws`;
    
    addTerminalRow('system', `Connecting to WebSocket: ${wsUrl}`);
    socket = new WebSocket(wsUrl);

    socket.onopen = () => {
        addTerminalRow('system', 'WebSocket connection established successfully.');
        document.getElementById('status-text').innerText = "Shield Active (Online)";
        document.getElementById('status-text').parentElement.style.color = '#10b981';
        document.getElementById('status-text').parentElement.style.borderColor = 'rgba(16, 185, 129, 0.2)';
        document.getElementById('status-text').parentElement.style.background = 'rgba(16, 185, 129, 0.1)';
    };

    socket.onmessage = (event) => {
        const payload = JSON.parse(event.data);
        if (payload.type === 'stats') {
            updateStatsUI(payload.data);
        } else if (payload.type === 'log') {
            appendLogEvent(payload.data);
        }
    };

    socket.onclose = () => {
        addTerminalRow('system', 'WebSocket connection closed. Retrying in 3 seconds...');
        document.getElementById('status-text').innerText = "Disconnected";
        document.getElementById('status-text').parentElement.style.color = '#ef4444';
        document.getElementById('status-text').parentElement.style.borderColor = 'rgba(239, 68, 68, 0.2)';
        document.getElementById('status-text').parentElement.style.background = 'rgba(239, 68, 68, 0.1)';
        
        setTimeout(connectWebSocket, 3000);
    };

    socket.onerror = (error) => {
        addTerminalRow('system', 'WebSocket error occurred.');
        socket.close();
    };
}

// Update Counters & Stats UI elements
function updateStatsUI(data) {
    document.getElementById('stat-allowed').innerText = data.allowed_total.toLocaleString();
    document.getElementById('stat-blocked').innerText = data.blocked_total.toLocaleString();
    document.getElementById('stat-banned').innerText = data.active_bans.toLocaleString();
    document.getElementById('stat-ram').innerText = `${data.memory_mb.toFixed(2)} MB`;
    document.getElementById('stat-goroutines').innerText = `Goroutines: ${data.goroutines}`;
}

// Log incoming request and increment chart cache
function appendLogEvent(log) {
    let type = 'success';
    let text = `[${formatTime(log.timestamp)}] ALLOWED | IP: ${log.IP} | Path: ${log.method} ${log.path}`;

    if (!log.allowed) {
        if (log.banned) {
            type = 'warn';
            text = `[${formatTime(log.timestamp)}] BANNED | IP: ${log.IP} | Path: ${log.method} ${log.path} | Reason: ${log.reason}`;
            blockedThisSecond++;
        } else {
            type = 'blocked';
            text = `[${formatTime(log.timestamp)}] BLOCKED | IP: ${log.IP} | Path: ${log.method} ${log.path} | Reason: ${log.reason}`;
            blockedThisSecond++;
        }
    } else {
        allowedThisSecond++;
    }

    addTerminalRow(type, text);
}

// Add a row to the interactive terminal component
function addTerminalRow(type, text) {
    const terminal = document.getElementById('log-terminal');
    const row = document.createElement('div');
    row.className = `terminal-row ${type}`;
    row.innerText = text;

    terminal.appendChild(row);

    // Keep only last 100 rows
    while (terminal.childNodes.length > 100) {
        terminal.removeChild(terminal.firstChild);
    }

    // Scroll to bottom
    terminal.scrollTop = terminal.scrollHeight;
}

// Fetch settings config
function fetchConfig() {
    fetch('/api/config')
        .then(res => res.json())
        .then(cfg => {
            document.getElementById('input-rps').value = cfg.rate_limit_rps;
            document.getElementById('val-rps').innerText = cfg.rate_limit_rps;
            
            document.getElementById('input-rpm').value = cfg.rate_limit_rpm;
            document.getElementById('val-rpm').innerText = cfg.rate_limit_rpm;
            
            document.getElementById('input-ban-duration').value = cfg.ban_duration_seconds;
            document.getElementById('input-autoban').checked = cfg.auto_ban_enabled;

            document.getElementById('info-mode').innerText = cfg.backend_url ? 'Reverse Proxy' : 'Mock Server';
            
            addTerminalRow('system', `Configuration loaded. RPS Limit: ${cfg.rate_limit_rps}`);
        })
        .catch(err => {
            addTerminalRow('system', 'Error loading configuration.');
        });
}

// Fetch active bans and render table
function fetchBans() {
    fetch('/api/bans')
        .then(res => res.json())
        .then(bans => {
            const tbody = document.getElementById('bans-list-body');
            
            if (!bans || bans.length === 0) {
                tbody.innerHTML = `<tr><td colspan="4" class="empty-state">No banned IP addresses currently.</td></tr>`;
                return;
            }

            tbody.innerHTML = '';
            const now = new Date();

            bans.forEach(ban => {
                const expiresAt = new Date(ban.expires_at);
                const remainingSec = Math.max(0, Math.round((expiresAt - now) / 1000));
                
                const tr = document.createElement('tr');
                tr.innerHTML = `
                    <td style="font-family: var(--font-mono); color: #38bdf8; font-weight: 500;">${ban.ip}</td>
                    <td style="font-family: var(--font-mono); color: #f59e0b;">${remainingSec} sec</td>
                    <td>${ban.reason}</td>
                    <td><button class="btn btn-secondary btn-sm" onclick="unbanIP('${ban.ip}')" style="color: #ef4444; border-color: rgba(239, 68, 68, 0.2);">Unban</button></td>
                `;
                tbody.appendChild(tr);
            });
        });
}

// Unban client helper (triggered by table buttons)
window.unbanIP = function(ip) {
    fetch('/api/unban', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ip })
    })
    .then(res => res.json())
    .then(res => {
        if (res.status === 'ok') {
            addTerminalRow('system', `${ip} unbanned manually.`);
            fetchBans();
        }
    });
};

// UI Interactions
function setupEventListeners() {
    // Config form sliders
    const rpsSlider = document.getElementById('input-rps');
    const rpsVal = document.getElementById('val-rps');
    rpsSlider.addEventListener('input', () => {
        rpsVal.innerText = rpsSlider.value;
    });

    const rpmSlider = document.getElementById('input-rpm');
    const rpmVal = document.getElementById('val-rpm');
    rpmSlider.addEventListener('input', () => {
        rpmVal.innerText = rpmSlider.value;
    });

    // DoS simulator slider
    const simSlider = document.getElementById('sim-rps');
    const simVal = document.getElementById('val-sim-rps');
    simSlider.addEventListener('input', () => {
        simVal.innerText = simSlider.value;
    });

    // Save config form handler
    document.getElementById('settings-form').addEventListener('submit', (e) => {
        e.preventDefault();
        
        const payload = {
            rate_limit_rps: parseFloat(rpsSlider.value),
            rate_limit_rpm: parseFloat(rpmSlider.value),
            ban_duration_seconds: parseInt(document.getElementById('input-ban-duration').value),
            auto_ban_enabled: document.getElementById('input-autoban').checked,
            // Keep existing whitelist and blacklist intact
            whitelist: ["127.0.0.1"],
            blacklist: []
        };

        fetch('/api/config', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        })
        .then(res => res.json())
        .then(res => {
            if (res.status === 'ok') {
                addTerminalRow('system', 'Shield configuration updated successfully.');
                fetchConfig();
            }
        })
        .catch(err => {
            addTerminalRow('system', 'Error saving configuration settings.');
        });
    });

    // Clear logs button
    document.getElementById('btn-clear-logs').addEventListener('click', () => {
        document.getElementById('log-terminal').innerHTML = '';
        addTerminalRow('system', 'Terminal logs cleared.');
    });

    // Clear all bans button
    document.getElementById('btn-clear-bans').addEventListener('click', () => {
        if (confirm('Are you sure you want to unban all IP addresses?')) {
            fetch('/api/bans')
                .then(res => res.json())
                .then(bans => {
                    if (!bans || bans.length === 0) return;
                    
                    let promises = bans.map(ban => {
                        return fetch('/api/unban', {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/json' },
                            body: JSON.stringify({ ip: ban.ip })
                        });
                    });
                    
                    Promise.all(promises).then(() => {
                        addTerminalRow('system', 'All active bans cleared.');
                        fetchBans();
                    });
                });
        }
    });

    // Simulator Start
    document.getElementById('btn-start-sim').addEventListener('click', () => {
        const rps = parseInt(simSlider.value);
        const duration = parseInt(document.getElementById('sim-duration').value);

        fetch('/api/simulate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ rps, duration })
        })
        .then(res => res.json())
        .then(res => {
            if (res.status === 'started') {
                document.getElementById('btn-start-sim').classList.add('hidden');
                document.getElementById('btn-stop-sim').classList.remove('hidden');
                document.getElementById('sim-status').innerHTML = `<span style="color: #ef4444; font-weight: bold;">⚠️ Simulation Active: Sending ${rps} req/sec!</span>`;
                addTerminalRow('system', `DoS test simulation started: ${rps} RPS, ${duration} seconds.`);
            }
        });
    });

    // Simulator Stop
    document.getElementById('btn-stop-sim').addEventListener('click', () => {
        fetch('/api/simulate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ stop: true })
        })
        .then(res => res.json())
        .then(res => {
            if (res.status === 'stopped') {
                resetSimulatorUI();
                addTerminalRow('system', 'Attack simulation stopped.');
            }
        });
    });
}

function resetSimulatorUI() {
    document.getElementById('btn-start-sim').classList.remove('hidden');
    document.getElementById('btn-stop-sim').classList.add('hidden');
    document.getElementById('sim-status').innerText = 'Simulation idle.';
}

// Helpers
function formatTime(isoString) {
    const d = new Date(isoString);
    const hrs = String(d.getHours()).padStart(2, '0');
    const mins = String(d.getMinutes()).padStart(2, '0');
    const secs = String(d.getSeconds()).padStart(2, '0');
    return `${hrs}:${mins}:${secs}`;
}
