package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"file-sync/logger"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TransferStatus represents the status of a file transfer (download or upload)
type TransferStatus struct {
	FilePath   string
	ServerName string
	Type       string  // "download", "upload"
	Status     string  // "active", "completed", "failed", "paused"
	Progress   float64 // 0.0 to 100.0
	Speed      int64   // bytes per second
	TotalBytes int64
	Downloaded int64
	StartTime  time.Time
	LastUpdate time.Time
	Error      string
}

// ServerStatus represents the status of a peer server
type ServerStatus struct {
	Name       string
	Host       string
	Port       int
	Status     string // "online", "offline", "syncing"
	LastSeen   time.Time
	FilesCount int
	TotalSize  int64
}

// UIManager manages the terminal UI
type UIManager struct {
	app             *tview.Application
	transfersTable  *tview.Table
	serversTable    *tview.Table
	statusText      *tview.TextView
	logsText        *tview.TextView
	transfers       map[string]*TransferStatus // Active transfers
	transferHistory []*TransferStatus          // Completed transfers (last 50)
	servers         map[string]*ServerStatus
	mutex           sync.RWMutex
	stopChan        chan struct{} // Channel to signal stopping
}

// NewUIManager creates a new UI manager
func NewUIManager() *UIManager {
	return &UIManager{
		transfers:       make(map[string]*TransferStatus),
		transferHistory: make([]*TransferStatus, 0, 50),
		servers:         make(map[string]*ServerStatus),
		stopChan:        make(chan struct{}),
	}
}

// Start initializes and starts the terminal UI
func (ui *UIManager) Start() error {
	ui.app = tview.NewApplication()

	// Create main layout
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Header
	header := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText("File Sync Monitor - Press 'q' or 'Esc' to quit, use arrows to navigate")
	header.SetBorder(true).SetTitle("File Sync v1.0")

	// Transfers table (downloads and uploads)
	ui.transfersTable = tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false).
		SetFixed(1, 0) // Fix header row
	ui.transfersTable.SetBorder(true).SetTitle("File Transfers")

	// Servers table
	ui.serversTable = tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false)
	ui.serversTable.SetBorder(true).SetTitle("Peer Servers")

	// Status text
	ui.statusText = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true)
	ui.statusText.SetBorder(true).SetTitle("Status")

	// Logs text
	ui.logsText = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetScrollable(true)
	ui.logsText.SetBorder(true).SetTitle("Recent Logs")

	// Layout
	tablesFlex := tview.NewFlex().
		AddItem(ui.transfersTable, 0, 2, false).
		AddItem(ui.serversTable, 0, 1, false)

	bottomFlex := tview.NewFlex().
		AddItem(ui.statusText, 0, 1, false).
		AddItem(ui.logsText, 0, 1, false)

	flex.AddItem(header, 3, 1, false).
		AddItem(tablesFlex, 0, 4, false).
		AddItem(bottomFlex, 0, 2, false)

	// Set up key handling for exit
	ui.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape || event.Rune() == 'q' {
			logger.Info("UI exit requested by user")
			// Signal updateLoop to stop immediately
			select {
			case <-ui.stopChan:
				// Already closed
			default:
				close(ui.stopChan)
			}

			// Stop the app immediately - this will cause ui.Start() to return
			logger.Info("Stopping UI application...")
			ui.app.Stop()
			return nil
		}
		return event
	})

	// Initial display update
	ui.updateDisplay()

	// Start update goroutine
	go ui.updateLoop()

	return ui.app.SetRoot(flex, true).Run()
}

// updateLoop periodically updates the display
func (ui *UIManager) updateLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ui.app.QueueUpdateDraw(func() {
				ui.updateDisplay()
			})
		case <-ui.stopChan:
			logger.Debug("UI update loop stopped")
			return
		}
	}
}

// updateDisplay refreshes the UI with current data
func (ui *UIManager) updateDisplay() {
	ui.mutex.RLock()
	defer ui.mutex.RUnlock()

	ui.updateTransfersTable()
	ui.updateServersTable()
	ui.updateStatusText()
	ui.updateLogsText()
}

// updateTransfersTable updates the transfers table (active + history)
func (ui *UIManager) updateTransfersTable() {
	table := ui.transfersTable
	table.Clear()

	// Headers - add Type column
	headers := []string{"Type", "File", "Server", "Progress", "Speed", "Status", "Time"}
	for i, header := range headers {
		table.SetCell(0, i, tview.NewTableCell(header).
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetSelectable(false).
			SetExpansion(1))
	}

	// Data rows - first active transfers, then history
	row := 1

	// Active transfers
	for _, transfer := range ui.transfers {
		ui.addTransferRow(table, transfer, row)
		row++
	}

	// Recent completed transfers (last 10)
	historyCount := 10
	if len(ui.transferHistory) < historyCount {
		historyCount = len(ui.transferHistory)
	}

	for i := len(ui.transferHistory) - historyCount; i < len(ui.transferHistory); i++ {
		ui.addTransferRow(table, ui.transferHistory[i], row)
		row++
	}

	// If no transfers, show message
	if row == 1 {
		table.SetCell(1, 0, tview.NewTableCell("No file transfers").
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetSelectable(false))
	}
}

// addTransferRow adds a single transfer row to the table
func (ui *UIManager) addTransferRow(table *tview.Table, transfer *TransferStatus, row int) {
	// Type indicator
	typeStr := transfer.Type
	if transfer.Type == "download" {
		typeStr = "↓"
	} else if transfer.Type == "upload" {
		typeStr = "↑"
	}

	// File name (truncated)
	fileName := transfer.FilePath
	if len(fileName) > 30 {
		fileName = "..." + fileName[len(fileName)-27:]
	}

	// Progress bar or completion time
	var progressStr string
	if transfer.Status == "active" {
		progressStr = ui.createProgressBar(transfer.Progress, 15)
	} else {
		// For completed transfers, show completion time
		if !transfer.LastUpdate.IsZero() {
			progressStr = ui.formatDuration(time.Since(transfer.LastUpdate)) + " ago"
		} else {
			progressStr = "Done"
		}
	}

	// Speed
	speedStr := ui.formatSpeed(transfer.Speed)

	// Status with color
	statusCell := tview.NewTableCell(transfer.Status)
	switch transfer.Status {
	case "active":
		statusCell.SetTextColor(tview.Styles.PrimaryTextColor)
	case "completed":
		statusCell.SetTextColor(tview.Styles.SecondaryTextColor)
	case "failed":
		statusCell.SetTextColor(tview.Styles.TertiaryTextColor)
	case "paused":
		statusCell.SetTextColor(tview.Styles.ContrastSecondaryTextColor)
	}

	// Time
	timeStr := ""
	if transfer.Status == "active" && !transfer.StartTime.IsZero() {
		timeStr = ui.formatDuration(time.Since(transfer.StartTime))
	} else if !transfer.LastUpdate.IsZero() {
		timeStr = ui.formatDuration(time.Since(transfer.LastUpdate)) + " ago"
	}

	cells := []string{typeStr, fileName, transfer.ServerName, progressStr, speedStr, transfer.Status, timeStr}
	for col, cellText := range cells {
		cell := tview.NewTableCell(cellText).SetExpansion(1)
		if col == 5 { // Status column
			cell = statusCell
		}
		table.SetCell(row, col, cell)
	}
}

// updateServersTable updates the servers table
func (ui *UIManager) updateServersTable() {
	table := ui.serversTable
	table.Clear()

	// Headers
	headers := []string{"Server", "Address", "Status", "Files", "Size", "Last Seen"}
	for i, header := range headers {
		table.SetCell(0, i, tview.NewTableCell(header).
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetSelectable(false).
			SetExpansion(1))
	}

	// Data rows
	row := 1
	for _, server := range ui.servers {
		// Address
		address := fmt.Sprintf("%s:%d", server.Host, server.Port)

		// Size
		sizeStr := ui.formatSize(server.TotalSize)

		// Last seen
		lastSeenStr := ui.formatDuration(time.Since(server.LastSeen))

		// Status with color
		statusCell := tview.NewTableCell(server.Status)
		switch server.Status {
		case "online":
			statusCell.SetTextColor(tview.Styles.PrimaryTextColor)
		case "offline":
			statusCell.SetTextColor(tview.Styles.TertiaryTextColor)
		case "syncing":
			statusCell.SetTextColor(tview.Styles.ContrastSecondaryTextColor)
		}

		cells := []string{server.Name, address, server.Status,
			fmt.Sprintf("%d", server.FilesCount), sizeStr, lastSeenStr}
		for col, cellText := range cells {
			cell := tview.NewTableCell(cellText).SetExpansion(1)
			if col == 2 { // Status column
				cell = statusCell
			}
			table.SetCell(row, col, cell)
		}
		row++
	}

	// If no servers, show message
	if row == 1 {
		table.SetCell(1, 0, tview.NewTableCell("No peer servers configured").
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetSelectable(false))
	}
}

// updateStatusText updates the status text area
func (ui *UIManager) updateStatusText() {
	text := ui.statusText

	activeDownloads := 0
	totalSpeed := int64(0)

	for _, transfer := range ui.transfers {
		if transfer.Status == "active" {
			activeDownloads++
			totalSpeed += transfer.Speed
		}
	}

	onlineServers := 0
	totalFiles := 0
	totalSize := int64(0)

	for _, server := range ui.servers {
		if server.Status == "online" || server.Status == "syncing" {
			onlineServers++
		}
		totalFiles += server.FilesCount
		totalSize += server.TotalSize
	}

	status := fmt.Sprintf("[green]Active downloads: %d[white] | [green]Total speed: %s[white] | [green]Online servers: %d[white] | [green]Total files: %d[white] | [green]Total size: %s[white]",
		activeDownloads, ui.formatSpeed(totalSpeed), onlineServers, totalFiles, ui.formatSize(totalSize))

	text.SetText(status)
}

// updateLogsText updates the logs display
func (ui *UIManager) updateLogsText() {
	logsText := ui.logsText
	logsText.Clear()

	// Get recent logs
	recentLogs := logger.GetRecentLogs(20) // Last 20 log messages

	if len(recentLogs) == 0 {
		logsText.SetText("No logs available")
		return
	}

	// Display logs with color coding
	var coloredLogs []string
	for _, logLine := range recentLogs {
		coloredLine := logLine

		// Color code based on log level
		if strings.Contains(logLine, "[ERROR]") {
			coloredLine = fmt.Sprintf("[red]%s[white]", logLine)
		} else if strings.Contains(logLine, "[WARN]") {
			coloredLine = fmt.Sprintf("[yellow]%s[white]", logLine)
		} else if strings.Contains(logLine, "[DEBUG]") {
			coloredLine = fmt.Sprintf("[blue]%s[white]", logLine)
		} else if strings.Contains(logLine, "[INFO]") {
			coloredLine = fmt.Sprintf("[green]%s[white]", logLine)
		}

		coloredLogs = append(coloredLogs, coloredLine)
	}

	logsText.SetText(strings.Join(coloredLogs, "\n"))
}

// createProgressBar creates a visual progress bar
func (ui *UIManager) createProgressBar(progress float64, width int) string {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	filled := int(float64(width) * progress / 100)
	bar := ""

	// Filled part
	for i := 0; i < filled; i++ {
		bar += "█"
	}

	// Empty part
	for i := filled; i < width; i++ {
		bar += "░"
	}

	// Percentage
	return fmt.Sprintf("%s %.1f%%", bar, progress)
}

// formatSpeed formats bytes per second
func (ui *UIManager) formatSpeed(speed int64) string {
	if speed == 0 {
		return "0 B/s"
	}

	units := []string{"B/s", "KB/s", "MB/s", "GB/s"}
	unitIndex := 0
	speedFloat := float64(speed)

	for speedFloat >= 1024 && unitIndex < len(units)-1 {
		speedFloat /= 1024
		unitIndex++
	}

	// For small speeds, show more precision
	if unitIndex == 0 {
		if speedFloat < 1 {
			return fmt.Sprintf("%.2f %s", speedFloat, units[unitIndex])
		} else if speedFloat < 10 {
			return fmt.Sprintf("%.1f %s", speedFloat, units[unitIndex])
		}
		return fmt.Sprintf("%.0f %s", speedFloat, units[unitIndex])
	}

	// For larger units, show appropriate precision
	if speedFloat < 10 {
		return fmt.Sprintf("%.2f %s", speedFloat, units[unitIndex])
	} else if speedFloat < 100 {
		return fmt.Sprintf("%.1f %s", speedFloat, units[unitIndex])
	}
	return fmt.Sprintf("%.0f %s", speedFloat, units[unitIndex])
}

// formatSize formats file size
func (ui *UIManager) formatSize(size int64) string {
	if size == 0 {
		return "0 B"
	}

	units := []string{"B", "KB", "MB", "GB", "TB"}
	unitIndex := 0
	sizeFloat := float64(size)

	for sizeFloat >= 1024 && unitIndex < len(units)-1 {
		sizeFloat /= 1024
		unitIndex++
	}

	return fmt.Sprintf("%.1f %s", sizeFloat, units[unitIndex])
}

// formatDuration formats time duration
func (ui *UIManager) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	} else {
		return fmt.Sprintf("%.0fh", d.Hours())
	}
}

// calculateETA calculates estimated time of arrival
func (ui *UIManager) calculateETA(transfer *TransferStatus) string {
	if transfer.Status != "active" || transfer.Speed == 0 {
		return "-"
	}

	remaining := transfer.TotalBytes - transfer.Downloaded
	if remaining <= 0 {
		return "Done"
	}

	seconds := float64(remaining) / float64(transfer.Speed)
	duration := time.Duration(seconds) * time.Second

	if duration < time.Minute {
		return fmt.Sprintf("%.0fs", duration.Seconds())
	} else if duration < time.Hour {
		return fmt.Sprintf("%.0fm", duration.Minutes())
	} else {
		return fmt.Sprintf("%.0fh", duration.Hours())
	}
}

// UpdateTransferStatus updates the status of a file transfer
func (ui *UIManager) UpdateTransferStatus(filePath, serverName, transferType string, status TransferStatus) {
	ui.mutex.Lock()
	defer ui.mutex.Unlock()

	key := filePath + ":" + serverName
	status.Type = transferType
	status.LastUpdate = time.Now()

	// If status changed to completed or failed, move to history
	if status.Status == "completed" || status.Status == "failed" {
		// Add to history
		ui.transferHistory = append(ui.transferHistory, &status)

		// Keep only last 50 entries
		if len(ui.transferHistory) > 50 {
			ui.transferHistory = ui.transferHistory[len(ui.transferHistory)-50:]
		}

		// Remove from active transfers after a short delay
		go func() {
			time.Sleep(2 * time.Second)
			ui.mutex.Lock()
			delete(ui.transfers, key)
			ui.mutex.Unlock()
		}()
	} else {
		// Update active transfer
		ui.transfers[key] = &status
	}
}

// UpdateDownloadStatus updates the status of a download (backward compatibility)
func (ui *UIManager) UpdateDownloadStatus(filePath, serverName string, status TransferStatus) {
	status.Type = "download"
	ui.UpdateTransferStatus(filePath, serverName, "download", status)
}

// UpdateUploadStatus updates the status of an upload
func (ui *UIManager) UpdateUploadStatus(filePath, serverName string, status TransferStatus) {
	status.Type = "upload"
	ui.UpdateTransferStatus(filePath, serverName, "upload", status)
}

// UpdateServerStatus updates the status of a server
func (ui *UIManager) UpdateServerStatus(serverName string, status ServerStatus) {
	ui.mutex.Lock()
	defer ui.mutex.Unlock()

	status.LastSeen = time.Now()
	ui.servers[serverName] = &status
}

// RemoveDownload removes a download from tracking (backward compatibility)
func (ui *UIManager) RemoveDownload(filePath, serverName string) {
	ui.mutex.Lock()
	defer ui.mutex.Unlock()

	key := filePath + ":" + serverName
	delete(ui.transfers, key)
}

// Stop stops the UI
func (ui *UIManager) Stop() {
	logger.Info("Stopping UI...")
	close(ui.stopChan) // Signal updateLoop to stop
	if ui.app != nil {
		ui.app.Stop()
	}
}
