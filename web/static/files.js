// File browser functionality
document.addEventListener('DOMContentLoaded', function() {
    loadFiles();

    document.getElementById('server-select').addEventListener('change', loadFiles);
});

async function loadFiles() {
    const serverSelect = document.getElementById('server-select');
    const selectedServer = serverSelect.value;
    const tbody = document.getElementById('files-body');

    tbody.innerHTML = '<tr><td colspan="4">Loading files...</td></tr>';

    try {
        let url = '/api/files';
        if (selectedServer !== '{{.ServerName}}') {
            // For remote servers, we'd need a different endpoint
            // For now, we'll show local files
            url = '/api/files';
        }

        const response = await fetch(url);
        const files = await response.json();

        if (files.length === 0) {
            tbody.innerHTML = '<tr><td colspan="4">No files found</td></tr>';
            return;
        }

        let html = '';
        files.forEach(file => {
            const size = formatBytes(file.size);
            const modified = new Date(file.modified).toLocaleString();

            html += `
                <tr>
                    <td>${file.path}</td>
                    <td>${size}</td>
                    <td>${modified}</td>
                    <td>
                        <a href="/api/files/${selectedServer}/${file.path}" class="btn btn-primary" download>Download</a>
                        <button class="btn btn-success" onclick="viewHistory('${file.path}')">History</button>
                    </td>
                </tr>
            `;
        });

        tbody.innerHTML = html;
    } catch (error) {
        tbody.innerHTML = '<tr><td colspan="4" class="status-error">Failed to load files</td></tr>';
        console.error('Failed to load files:', error);
    }
}

function formatBytes(bytes) {
    if (bytes === 0) return '0 Bytes';

    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));

    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function viewHistory(filePath) {
    window.location.href = `/history?file=${encodeURIComponent(filePath)}`;
}