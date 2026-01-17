// Dashboard functionality
document.addEventListener('DOMContentLoaded', function() {
    loadServerStatus();
    loadPeers();
    loadRecentChanges();
});

async function loadServerStatus() {
    try {
        const response = await fetch('/api/files');
        const files = await response.json();

        const statusDiv = document.getElementById('server-status');
        statusDiv.innerHTML = `
            <p class="status-healthy">✓ Server is running</p>
            <p>Files synced: ${files.length}</p>
            <p>Last sync: ${new Date().toLocaleString()}</p>
        `;
    } catch (error) {
        document.getElementById('server-status').innerHTML =
            '<p class="status-error">✗ Server error</p>';
        console.error('Failed to load server status:', error);
    }
}

async function loadPeers() {
    try {
        const response = await fetch('/api/peers');
        const peers = await response.json();

        const peersDiv = document.getElementById('peers-list');
        let html = '<ul>';

        for (const [name, peer] of Object.entries(peers)) {
            html += `<li>${name} (${peer.host}:${peer.port})</li>`;
        }

        if (Object.keys(peers).length === 0) {
            html += '<li>No peers connected</li>';
        }

        html += '</ul>';
        peersDiv.innerHTML = html;
    } catch (error) {
        document.getElementById('peers-list').innerHTML =
            '<p class="status-error">Failed to load peers</p>';
        console.error('Failed to load peers:', error);
    }
}

async function loadRecentChanges() {
    try {
        const response = await fetch('/api/changes?limit=10');
        const changes = await response.json();

        const changesDiv = document.getElementById('recent-changes');
        let html = '<ul>';

        changes.forEach(change => {
            const time = new Date(change.sync_time).toLocaleString();
            html += `<li>${time}: ${change.action} - ${change.path} (${change.server_name})</li>`;
        });

        if (changes.length === 0) {
            html += '<li>No recent changes</li>';
        }

        html += '</ul>';
        changesDiv.innerHTML = html;
    } catch (error) {
        document.getElementById('recent-changes').innerHTML =
            '<p class="status-error">Failed to load recent changes</p>';
        console.error('Failed to load recent changes:', error);
    }
}

// Refresh data every 30 seconds
setInterval(() => {
    loadServerStatus();
    loadPeers();
    loadRecentChanges();
}, 30000);