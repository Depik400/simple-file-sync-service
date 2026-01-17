// History functionality
document.addEventListener('DOMContentLoaded', function() {
    const urlParams = new URLSearchParams(window.location.search);
    const fileParam = urlParams.get('file');

    if (fileParam) {
        document.getElementById('file-path').value = fileParam;
        searchHistory();
    }

    document.getElementById('search-btn').addEventListener('click', searchHistory);
    document.getElementById('file-path').addEventListener('keypress', function(e) {
        if (e.key === 'Enter') {
            searchHistory();
        }
    });
});

async function searchHistory() {
    const filePath = document.getElementById('file-path').value.trim();
    const tbody = document.getElementById('history-body');

    if (!filePath) {
        tbody.innerHTML = '<tr><td colspan="5">Please enter a file path</td></tr>';
        return;
    }

    tbody.innerHTML = '<tr><td colspan="5">Searching...</td></tr>';

    try {
        const response = await fetch(`/api/history/${encodeURIComponent(filePath)}?limit=100`);
        const history = await response.json();

        if (history.length === 0) {
            tbody.innerHTML = '<tr><td colspan="5">No history found for this file</td></tr>';
            return;
        }

        let html = '';
        history.forEach(record => {
            const time = new Date(record.sync_time).toLocaleString();
            const size = formatBytes(record.size);
            const actionClass = getActionClass(record.action);

            html += `
                <tr>
                    <td>${time}</td>
                    <td>${record.path}</td>
                    <td><span class="${actionClass}">${record.action}</span></td>
                    <td>${record.server_name}</td>
                    <td>${size}</td>
                </tr>
            `;
        });

        tbody.innerHTML = html;
    } catch (error) {
        tbody.innerHTML = '<tr><td colspan="5" class="status-error">Failed to load history</td></tr>';
        console.error('Failed to load history:', error);
    }
}

function formatBytes(bytes) {
    if (bytes === 0) return '0 Bytes';

    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));

    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function getActionClass(action) {
    switch (action) {
        case 'created':
            return 'action-created';
        case 'modified':
            return 'action-modified';
        case 'deleted':
            return 'action-deleted';
        case 'downloaded':
            return 'action-downloaded';
        default:
            return 'action-other';
    }
}