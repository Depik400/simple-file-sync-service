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

// DownloadStatus represents the status of a file download
type DownloadStatus struct {
	FilePath   string
	ServerName string
	Status     string  // "downloading", "completed", "failed", "paused"
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
	app            *tview.Application
	downloadsTable *tview.Table
	serversTable   *tview.Table
	statusText     *tview.TextView
	logsText       *tview.TextView
	downloads      map[string]*DownloadStatus
	servers        map[string]*ServerStatus
	mutex          sync.RWMutex
	stopChan       chan struct{} // Channel to signal stopping
}

// NewUIManager creates a new UI manager
func NewUIManager() *UIManager {
	return &UIManager{
		downloads: make(map[string]*DownloadStatus),
		servers:   make(map[string]*ServerStatus),
		stopChan:  make(chan struct{}),
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
		SetText("File Sync Monitor - Press 'q' or 'Esc' to quit")
	header.SetBorder(true).SetTitle("File Sync v1.0")

	// Downloads table
	ui.downloadsTable = tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false)
	ui.downloadsTable.SetBorder(true).SetTitle("Active Downloads")

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
		AddItem(ui.downloadsTable, 0, 2, false).
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

	ui.updateDownloadsTable()
	ui.updateServersTable()
	ui.updateStatusText()
	ui.updateLogsText()
}

// updateDownloadsTable updates the downloads table
func (ui *UIManager) updateDownloadsTable() {
	table := ui.downloadsTable
	table.Clear()

	// Headers
	headers := []string{"File", "Server", "Progress", "Speed", "Status", "ETA"}
	for i, header := range headers {
		table.SetCell(0, i, tview.NewTableCell(header).
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetSelectable(false).
			SetExpansion(1))
	}

	// Data rows
	row := 1
	for _, download := range ui.downloads {
		// File name (truncated)
		fileName := download.FilePath
		if len(fileName) > 30 {
			fileName = "..." + fileName[len(fileName)-27:]
		}

		// Progress bar
		progressBar := ui.createProgressBar(download.Progress, 20)

		// Speed
		speedStr := ui.formatSpeed(download.Speed)

		// ETA
		etaStr := ui.calculateETA(download)

		// Status with color
		statusCell := tview.NewTableCell(download.Status)
		switch download.Status {
		case "downloading":
			statusCell.SetTextColor(tview.Styles.PrimaryTextColor)
		case "completed":
			statusCell.SetTextColor(tview.Styles.SecondaryTextColor)
		case "failed":
			statusCell.SetTextColor(tview.Styles.TertiaryTextColor)
		case "paused":
			statusCell.SetTextColor(tview.Styles.ContrastSecondaryTextColor)
		}

		cells := []string{fileName, download.ServerName, progressBar, speedStr, download.Status, etaStr}
		for col, cellText := range cells {
			cell := tview.NewTableCell(cellText).SetExpansion(1)
			if col == 4 { // Status column
				cell = statusCell
			}
			table.SetCell(row, col, cell)
		}
		row++
	}

	// If no downloads, show message
	if row == 1 {
		table.SetCell(1, 0, tview.NewTableCell("No active downloads").
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetSelectable(false))
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

	for _, download := range ui.downloads {
		if download.Status == "downloading" {
			activeDownloads++
			totalSpeed += download.Speed
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

	if unitIndex == 0 {
		return fmt.Sprintf("%.0f %s", speedFloat, units[unitIndex])
	}
	return fmt.Sprintf("%.1f %s", speedFloat, units[unitIndex])
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
func (ui *UIManager) calculateETA(download *DownloadStatus) string {
	if download.Status != "downloading" || download.Speed == 0 {
		return "-"
	}

	remaining := download.TotalBytes - download.Downloaded
	if remaining <= 0 {
		return "Done"
	}

	seconds := float64(remaining) / float64(download.Speed)
	duration := time.Duration(seconds) * time.Second

	if duration < time.Minute {
		return fmt.Sprintf("%.0fs", duration.Seconds())
	} else if duration < time.Hour {
		return fmt.Sprintf("%.0fm", duration.Minutes())
	} else {
		return fmt.Sprintf("%.0fh", duration.Hours())
	}
}

// UpdateDownloadStatus updates the status of a download
func (ui *UIManager) UpdateDownloadStatus(filePath, serverName string, status DownloadStatus) {
	ui.mutex.Lock()
	defer ui.mutex.Unlock()

	key := filePath + ":" + serverName
	status.LastUpdate = time.Now()

	if status.Status == "completed" || status.Status == "failed" {
		// Remove completed/failed downloads after a delay
		go func() {
			time.Sleep(5 * time.Second)
			ui.mutex.Lock()
			delete(ui.downloads, key)
			ui.mutex.Unlock()
		}()
	} else {
		ui.downloads[key] = &status
	}
}

// UpdateServerStatus updates the status of a server
func (ui *UIManager) UpdateServerStatus(serverName string, status ServerStatus) {
	ui.mutex.Lock()
	defer ui.mutex.Unlock()

	status.LastSeen = time.Now()
	ui.servers[serverName] = &status
}

// RemoveDownload removes a download from tracking
func (ui *UIManager) RemoveDownload(filePath, serverName string) {
	ui.mutex.Lock()
	defer ui.mutex.Unlock()

	key := filePath + ":" + serverName
	delete(ui.downloads, key)
}

// Stop stops the UI
func (ui *UIManager) Stop() {
	logger.Info("Stopping UI...")
	close(ui.stopChan) // Signal updateLoop to stop
	if ui.app != nil {
		ui.app.Stop()
	}
}
