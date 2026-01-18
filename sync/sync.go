package sync

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"file-sync/config"
	"file-sync/db"
	"file-sync/p2p"
	"file-sync/ui"
)

type FileSync struct {
	config        *config.Config
	db            *db.Database
	p2p           *p2p.P2PNetwork
	previousFiles map[string]p2p.FileInfo // Track previous file state
	uiManager     *ui.UIManager           // Optional UI manager
}

func NewFileSync(cfg *config.Config, database *db.Database, p2pNet *p2p.P2PNetwork) *FileSync {
	return &FileSync{
		config:        cfg,
		db:            database,
		p2p:           p2pNet,
		previousFiles: make(map[string]p2p.FileInfo),
		uiManager:     nil, // Will be set later if UI mode is enabled
	}
}

// SetUIManager sets the UI manager for this sync service
func (fs *FileSync) SetUIManager(uiManager *ui.UIManager) {
	fs.uiManager = uiManager
}

func (fs *FileSync) Start() error {
	fmt.Printf("[SYNC] Starting file sync service for server %s\n", fs.config.Server.Name)
	fmt.Printf("[SYNC] Sync directory: %s\n", fs.config.Server.SyncDir)
	fmt.Printf("[SYNC] Sync interval: %d seconds\n", fs.config.Sync.Interval)

	// Ensure sync directory exists
	if err := os.MkdirAll(fs.config.Server.SyncDir, 0755); err != nil {
		return fmt.Errorf("failed to create sync directory: %w", err)
	}

	// Clean up orphaned temp files from previous failed downloads
	if err := fs.cleanupOrphanedTempFiles(); err != nil {
		fmt.Printf("[SYNC] WARNING: Failed to cleanup temp files: %v\n", err)
	}

	// Start periodic sync
	go fs.syncRoutine()

	fmt.Printf("[SYNC] File sync service started successfully\n")
	return nil
}

func (fs *FileSync) syncRoutine() {
	ticker := time.NewTicker(time.Duration(fs.config.Sync.Interval) * time.Second)
	defer ticker.Stop()

	fmt.Printf("[SYNC] Starting sync routine with %d second intervals\n", fs.config.Sync.Interval)

	for range ticker.C {
		fmt.Printf("[SYNC] Starting periodic sync at %s\n", time.Now().Format("15:04:05"))
		if err := fs.performSync(); err != nil {
			fmt.Printf("[SYNC] ERROR: Sync failed: %v\n", err)
		} else {
			fmt.Printf("[SYNC] Periodic sync completed successfully\n")
		}
	}
}

func (fs *FileSync) performSync() error {
	fmt.Printf("[SYNC] Performing sync operation...\n")

	// Get local files
	fmt.Printf("[SYNC] Scanning local directory: %s\n", fs.config.Server.SyncDir)
	localFiles, err := fs.scanDirectory(fs.config.Server.SyncDir)
	if err != nil {
		return fmt.Errorf("failed to scan local directory: %w", err)
	}
	fmt.Printf("[SYNC] Found %d local files\n", len(localFiles))

	// Log local files
	for _, file := range localFiles {
		fmt.Printf("[SYNC] Local file: %s (hash: %s, size: %d)\n", file.Path, file.Hash[:8]+"...", file.Size)
	}

	// Create current files map for quick lookup
	currentFiles := make(map[string]p2p.FileInfo)
	for _, file := range localFiles {
		currentFiles[file.Path] = file
	}

	// Detect deleted files
	deletedFiles := fs.detectDeletedFiles(currentFiles)
	if len(deletedFiles) > 0 {
		fmt.Printf("[SYNC] Detected %d deleted files\n", len(deletedFiles))
		for _, file := range deletedFiles {
			fmt.Printf("[SYNC] Deleted file: %s\n", file.Path)
		}
	}

	// Broadcast file list and deletions to peers
	fmt.Printf("[SYNC] Broadcasting file list to %d peers\n", len(fs.p2p.GetPeers()))
	if err := fs.p2p.BroadcastFileList(localFiles); err != nil {
		fmt.Printf("[SYNC] WARNING: Failed to broadcast file list: %v\n", err)
		// Continue anyway - this is not fatal
	}

	// Broadcast deletions to peers
	if len(deletedFiles) > 0 {
		fmt.Printf("[SYNC] Broadcasting deletions to peers\n")
		if err := fs.p2p.BroadcastDeletions(deletedFiles); err != nil {
			fmt.Printf("[SYNC] WARNING: Failed to broadcast deletions: %v\n", err)
		}
	}

	// Record local changes
	fmt.Printf("[SYNC] Recording local file changes to database\n")
	if err := fs.recordLocalChanges(localFiles); err != nil {
		return fmt.Errorf("failed to record local changes: %w", err)
	}

	// Record deletions
	if len(deletedFiles) > 0 {
		fmt.Printf("[SYNC] Recording file deletions to database\n")
		if err := fs.recordDeletions(deletedFiles); err != nil {
			fmt.Printf("[SYNC] WARNING: Failed to record deletions: %v\n", err)
		}
	}

	// Update previous files state
	fs.previousFiles = currentFiles

	// Check for files to download from peers
	fmt.Printf("[SYNC] Checking for files to download from peers\n")
	if err := fs.syncFromPeers(); err != nil {
		fmt.Printf("[SYNC] WARNING: Failed to sync from peers: %v\n", err)
	}

	fmt.Printf("[SYNC] Sync operation completed\n")
	return nil
}

func (fs *FileSync) scanDirectory(dir string) ([]p2p.FileInfo, error) {
	var files []p2p.FileInfo

	fmt.Printf("[SYNC] Scanning directory: %s\n", dir)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		fmt.Printf("[SYNC] Found file: %s\n", path)

		// Get relative path
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		// Skip certain files that shouldn't be synced
		fileName := filepath.Base(path)
		if fs.shouldSkipFile(fileName) {
			fmt.Printf("[SYNC] Skipping file: %s (filtered out)\n", fileName)
			return nil
		}

		// Calculate hash
		hash, err := fs.calculateFileHash(path)
		if err != nil {
			return err
		}

		fileInfo := p2p.FileInfo{
			Path:     relPath,
			Hash:     hash,
			Size:     info.Size(),
			Modified: info.ModTime(),
		}

		fmt.Printf("[SYNC] Added to sync list: %s (hash: %s, size: %d)\n", relPath, hash[:8]+"...", info.Size())
		files = append(files, fileInfo)
		return nil
	})

	fmt.Printf("[SYNC] Scan completed, found %d files\n", len(files))
	return files, err
}

// GetCurrentFiles returns the current local files
func (fs *FileSync) GetCurrentFiles() ([]p2p.FileInfo, error) {
	return fs.scanDirectory(fs.config.Server.SyncDir)
}

// GetP2PNetwork returns the P2P network instance
func (fs *FileSync) GetP2PNetwork() *p2p.P2PNetwork {
	return fs.p2p
}

func (fs *FileSync) cleanupOrphanedTempFiles() error {
	fmt.Printf("[SYNC] Cleaning up orphaned temp files...\n")

	return filepath.Walk(fs.config.Server.SyncDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		fileName := filepath.Base(path)
		if strings.HasSuffix(fileName, ".tmp") {
			// Check if corresponding final file exists
			finalPath := strings.TrimSuffix(path, ".tmp")
			if _, err := os.Stat(finalPath); os.IsNotExist(err) {
				// Final file doesn't exist, this is an orphaned temp file
				fmt.Printf("[SYNC] Removing orphaned temp file: %s\n", path)
				if err := os.Remove(path); err != nil {
					fmt.Printf("[SYNC] WARNING: Failed to remove orphaned temp file %s: %v\n", path, err)
				}
			} else {
				// Final file exists, check if temp file is newer (indicates failed rename)
				if finalInfo, err := os.Stat(finalPath); err == nil {
					if info.ModTime().After(finalInfo.ModTime()) {
						fmt.Printf("[SYNC] Removing stale temp file (newer than final): %s\n", path)
						if err := os.Remove(path); err != nil {
							fmt.Printf("[SYNC] WARNING: Failed to remove stale temp file %s: %v\n", path, err)
						}
					}
				}
			}
		}

		return nil
	})
}

func (fs *FileSync) shouldSkipFile(fileName string) bool {
	// Skip PID files
	if strings.HasSuffix(fileName, ".pid") {
		return true
	}

	// Skip hidden files
	if strings.HasPrefix(fileName, ".") {
		return true
	}

	// Skip temporary files (including .tmp files from downloads)
	if strings.HasSuffix(fileName, ".tmp") || strings.HasSuffix(fileName, ".temp") {
		fmt.Printf("[SYNC] Skipping temporary file: %s\n", fileName)
		return true
	}

	// Skip lock files
	if strings.HasSuffix(fileName, ".lock") {
		return true
	}

	return false
}

func (fs *FileSync) calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (fs *FileSync) recordLocalChanges(files []p2p.FileInfo) error {
	for _, file := range files {
		// Check if file exists in history
		history, err := fs.db.GetFileHistory(file.Path, 1)
		if err != nil {
			return err
		}

		action := "created"
		if len(history) > 0 {
			if history[0].Hash != file.Hash {
				action = "modified"
			} else {
				continue // No change
			}
		}

		if err := fs.db.RecordFileChange(file.Path, file.Hash, file.Size, file.Modified, fs.config.Server.Name, action); err != nil {
			return err
		}
	}

	return nil
}

func (fs *FileSync) DownloadFileParallel(serverName, filePath string) error {
	return fs.downloadFileWithConcurrency(serverName, filePath, fs.config.Sync.MaxParallelChunks)
}

func (fs *FileSync) downloadFileWithConcurrency(serverName, filePath string, maxConcurrency int) error {
	localPath := filepath.Join(fs.config.Server.SyncDir, filePath)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}

	// First, get file size by requesting a small range
	testReader, fileSize, err := fs.p2p.RequestFileRange(serverName, filePath, 0, 0)
	if err != nil {
		return fmt.Errorf("failed to get file size: %w", err)
	}
	testReader.Close()

	if fileSize == 0 {
		return fmt.Errorf("file size is 0")
	}

	// Calculate adaptive chunk size based on file size and concurrency
	chunkSize := fs.calculateAdaptiveChunkSize(fileSize, maxConcurrency)
	fmt.Printf("[SYNC] Starting parallel download of %s (size: %d bytes, chunk size: %d bytes, concurrency: %d)\n",
		filePath, fileSize, chunkSize, maxConcurrency)

	// Update UI with download start
	if fs.uiManager != nil {
		fs.uiManager.UpdateDownloadStatus(filePath, serverName, ui.DownloadStatus{
			FilePath:   filePath,
			ServerName: serverName,
			Status:     "downloading",
			Progress:   0,
			TotalBytes: fileSize,
			StartTime:  time.Now(),
		})
	}

	// Calculate chunk ranges
	var ranges []chunkRange

	if maxConcurrency <= 1 || fileSize <= chunkSize {
		// Single chunk download
		ranges = []chunkRange{{start: 0, end: fileSize - 1}}
	} else {
		// Multi-chunk parallel download
		for start := int64(0); start < fileSize; start += chunkSize {
			end := start + chunkSize - 1
			if end >= fileSize {
				end = fileSize - 1
			}
			ranges = append(ranges, chunkRange{start: start, end: end})
		}
	}

	fmt.Printf("[SYNC] File divided into %d chunks\n", len(ranges))

	// Create temporary file (atomic creation)
	tempPath := localPath + ".tmp"

	// Check if temp file already exists (another process might be downloading)
	if _, err := os.Stat(tempPath); err == nil {
		return fmt.Errorf("temp file already exists, another process is downloading this file")
	}

	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	// Pre-allocate file space
	if err := file.Truncate(fileSize); err != nil {
		file.Close()
		return fmt.Errorf("failed to pre-allocate file: %w", err)
	}

	// Channel for chunk results
	type chunkResult struct {
		range_ chunkRange
		data   []byte
		err    error
	}

	results := make(chan chunkResult, len(ranges))
	errorOccurred := make(chan struct{}) // Signal channel for errors

	// Semaphore to limit concurrent downloads
	sem := make(chan struct{}, maxConcurrency)

	// Start download workers
	var wg sync.WaitGroup
	for i, r := range ranges {
		wg.Add(1)
		go func(workerID int, cr chunkRange) {
			defer wg.Done()

			fmt.Printf("[SYNC] Worker %d starting chunk %d-%d (%d bytes)\n",
				workerID, cr.start, cr.end, cr.end-cr.start+1)

			// Check if an error already occurred in another worker
			select {
			case <-errorOccurred:
				// Another worker failed, abort this one too
				fmt.Printf("[SYNC] Worker %d aborted due to error in another worker\n", workerID)
				results <- chunkResult{range_: cr, err: fmt.Errorf("aborted due to error in another worker")}
				return
			default:
			}

			// Acquire semaphore
			fmt.Printf("[SYNC] Worker %d waiting for semaphore...\n", workerID)
			sem <- struct{}{}
			defer func() { <-sem }()
			fmt.Printf("[SYNC] Worker %d acquired semaphore\n", workerID)

			fmt.Printf("[SYNC] Worker %d downloading chunk %d-%d (%d bytes)\n",
				workerID, cr.start, cr.end, cr.end-cr.start+1)

			reader, _, err := fs.p2p.RequestFileRange(serverName, filePath, cr.start, cr.end)
			if err != nil {
				fmt.Printf("[SYNC] Worker %d failed to request range: %v\n", workerID, err)
				select {
				case errorOccurred <- struct{}{}: // Signal error to other workers
				default:
				}
				results <- chunkResult{range_: cr, err: err}
				return
			}
			defer reader.Close()

			// Pre-allocate buffer with exact size for better performance
			expectedSize := cr.end - cr.start + 1
			chunkData := make([]byte, expectedSize)

			fmt.Printf("[SYNC] Worker %d reading %d bytes...\n", workerID, expectedSize)

			// Read data with optimized buffering
			totalRead := int64(0)
			bufReader := bufio.NewReaderSize(reader, 1*1024*1024) // 1MB read buffer

			for totalRead < expectedSize {
				n, err := bufReader.Read(chunkData[totalRead:])
				if n > 0 {
					totalRead += int64(n)
					fmt.Printf("[SYNC] Worker %d read %d/%d bytes\n", workerID, totalRead, expectedSize)
				}
				if err != nil {
					if err == io.EOF {
						break
					}
					fmt.Printf("[SYNC] Worker %d read error: %v\n", workerID, err)
					select {
					case errorOccurred <- struct{}{}: // Signal error to other workers
					default:
					}
					results <- chunkResult{range_: cr, err: err}
					return
				}
			}

			if totalRead != expectedSize {
				err := fmt.Errorf("read size mismatch: expected %d, got %d", expectedSize, totalRead)
				fmt.Printf("[SYNC] Worker %d size mismatch: %v\n", workerID, err)
				select {
				case errorOccurred <- struct{}{}: // Signal error to other workers
				default:
				}
				results <- chunkResult{range_: cr, err: err}
				return
			}

			fmt.Printf("[SYNC] Worker %d completed chunk successfully\n", workerID)
			results <- chunkResult{range_: cr, data: chunkData}
		}(i, r)
	}

	// Close results channel when all workers are done
	go func() {
		fmt.Printf("[SYNC] Waiting for all workers to complete...\n")
		wg.Wait()
		fmt.Printf("[SYNC] All workers completed, closing results channel\n")
		close(results)
	}()

	// Collect and write results
	totalDownloaded := int64(0)
	chunksReceived := 0
	hasError := false
	errorDetails := make([]string, 0)

	fmt.Printf("[SYNC] Waiting for %d chunk results...\n", len(ranges))

	for result := range results {
		chunksReceived++

		fmt.Printf("[SYNC] Received result %d/%d for chunk %d-%d\n",
			chunksReceived, len(ranges), result.range_.start, result.range_.end)

		// Check for errors in chunks
		if result.err != nil {
			errorMsg := fmt.Sprintf("chunk %d-%d failed: %v", result.range_.start, result.range_.end, result.err)
			errorDetails = append(errorDetails, errorMsg)
			fmt.Printf("[SYNC] ERROR: %s\n", errorMsg)
			hasError = true
			continue // Continue to drain the channel
		}

		// Skip writing if we already had an error
		if hasError {
			fmt.Printf("[SYNC] Skipping chunk %d-%d due to previous error\n",
				result.range_.start, result.range_.end)
			continue
		}

		// Write chunk to file
		fmt.Printf("[SYNC] Writing chunk %d-%d (%d bytes) to file\n",
			result.range_.start, result.range_.end, len(result.data))
		if _, err := file.WriteAt(result.data, result.range_.start); err != nil {
			hasError = true
			file.Close()
			os.Remove(tempPath) // Clean up temp file on error
			return fmt.Errorf("failed to write chunk at offset %d: %w", result.range_.start, err)
		}

		totalDownloaded += int64(len(result.data))
		fmt.Printf("[SYNC] Chunk %d-%d written (%d/%d bytes total, %d/%d chunks)\n",
			result.range_.start, result.range_.end, totalDownloaded, fileSize, chunksReceived, len(ranges))

		// Update UI with progress
		if fs.uiManager != nil {
			progress := float64(totalDownloaded) / float64(fileSize) * 100
			fs.uiManager.UpdateDownloadStatus(filePath, serverName, ui.DownloadStatus{
				FilePath:   filePath,
				ServerName: serverName,
				Status:     "downloading",
				Progress:   progress,
				Speed:      0, // TODO: calculate actual speed
				TotalBytes: fileSize,
				Downloaded: totalDownloaded,
				StartTime:  time.Now(), // TODO: track actual start time
			})
		}
	}

	fmt.Printf("[SYNC] Finished receiving chunks: %d received, %d expected\n", chunksReceived, len(ranges))

	// If there was an error, clean up and return it with details
	if hasError {
		file.Close()
		os.Remove(tempPath) // Clean up temp file on error

		// Update UI with error
		if fs.uiManager != nil {
			fs.uiManager.UpdateDownloadStatus(filePath, serverName, ui.DownloadStatus{
				FilePath:   filePath,
				ServerName: serverName,
				Status:     "failed",
				Error:      fmt.Sprintf("Download failed: %v", errorDetails),
			})
		}

		return fmt.Errorf("download failed due to chunk errors: %v", errorDetails)
	}

	// Check that we wrote all expected bytes before syncing
	if totalDownloaded != fileSize {
		file.Close()
		os.Remove(tempPath) // Clean up temp file on error
		return fmt.Errorf("incomplete write: expected %d bytes, wrote %d bytes", fileSize, totalDownloaded)
	}

	fmt.Printf("[SYNC] Syncing file to disk...\n")
	// Sync file to disk before closing
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(tempPath) // Clean up temp file on error
		return fmt.Errorf("failed to sync file to disk: %w", err)
	}
	fmt.Printf("[SYNC] File synced to disk successfully\n")

	// Close file
	file.Close()

	// Verify that we received all chunks
	if chunksReceived != len(ranges) {
		os.Remove(tempPath) // Clean up temp file on error
		return fmt.Errorf("missing chunks: expected %d, received %d", len(ranges), chunksReceived)
	}

	// Verify that we downloaded all expected data
	if totalDownloaded != fileSize {
		os.Remove(tempPath) // Clean up temp file on error
		return fmt.Errorf("download incomplete: expected %d bytes, got %d bytes", fileSize, totalDownloaded)
	}

	// Final verification - check file size on disk
	fmt.Printf("[SYNC] Verifying downloaded file...\n")
	if info, err := os.Stat(tempPath); err != nil {
		fmt.Printf("[SYNC] ERROR: Failed to stat temp file: %v\n", err)
		os.Remove(tempPath) // Clean up temp file on error
		return fmt.Errorf("failed to verify temp file: %w", err)
	} else if info.Size() != fileSize {
		fmt.Printf("[SYNC] ERROR: File size mismatch on disk: expected %d, got %d\n", fileSize, info.Size())
		os.Remove(tempPath) // Clean up temp file on error
		return fmt.Errorf("file size mismatch on disk: expected %d, got %d", fileSize, info.Size())
	}

	fmt.Printf("[SYNC] File verification passed, renaming %s to %s\n", tempPath, localPath)
	if err := os.Rename(tempPath, localPath); err != nil {
		fmt.Printf("[SYNC] ERROR: Failed to rename temp file: %v\n", err)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	fmt.Printf("[SYNC] Parallel download completed successfully: %s (%d bytes)\n", filePath, totalDownloaded)

	// Update UI with completion
	if fs.uiManager != nil {
		fs.uiManager.UpdateDownloadStatus(filePath, serverName, ui.DownloadStatus{
			FilePath:   filePath,
			ServerName: serverName,
			Status:     "completed",
			Progress:   100,
			TotalBytes: fileSize,
			Downloaded: totalDownloaded,
		})
	}

	// Verify file size
	if info, err := os.Stat(localPath); err != nil {
		return err
	} else if info.Size() != fileSize {
		return fmt.Errorf("file size mismatch: expected %d, got %d", fileSize, info.Size())
	}

	// Calculate hash for verification
	hash, err := fs.calculateFileHash(localPath)
	if err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	// Record the download
	if err := fs.db.RecordFileChange(filePath, hash, fileSize, time.Now(), fs.config.Server.Name, "downloaded"); err != nil {
		fmt.Printf("[SYNC] WARNING: Failed to record download: %v\n", err)
	}

	return nil
}

type chunkRange struct {
	start, end int64
}

// calculateAdaptiveChunkSize determines optimal chunk size based on file size and concurrency
func (fs *FileSync) calculateAdaptiveChunkSize(fileSize int64, maxConcurrency int) int64 {
	baseChunkSize := int64(fs.config.Sync.ChunkSize)

	// For very small files, use the whole file as one chunk
	if fileSize <= baseChunkSize {
		return fileSize
	}

	// For large files with high concurrency, use larger chunks to reduce overhead
	if maxConcurrency >= 4 && fileSize > 100*1024*1024 { // > 100MB
		// Scale up chunk size for very large files
		scaledSize := baseChunkSize * 2
		if fileSize > 1024*1024*1024 { // > 1GB
			scaledSize = baseChunkSize * 4
		}
		// Don't make chunks too large (max 128MB)
		if scaledSize > 128*1024*1024 {
			scaledSize = 128 * 1024 * 1024
		}
		return scaledSize
	}

	// For small files with low concurrency, use smaller chunks for better progress reporting
	if maxConcurrency <= 2 && fileSize < 50*1024*1024 { // < 50MB
		smallChunkSize := baseChunkSize / 2
		// Don't make chunks too small (min 1MB)
		if smallChunkSize < 1024*1024 {
			smallChunkSize = 1024 * 1024
		}
		return smallChunkSize
	}

	return baseChunkSize
}

func (fs *FileSync) DownloadFile(serverName, filePath string) error {
	localPath := filepath.Join(fs.config.Server.SyncDir, filePath)
	tempPath := localPath + ".tmp"

	// Check if another process is already downloading this file
	if _, err := os.Stat(tempPath); err == nil {
		fmt.Printf("[SYNC] File %s is already being downloaded by another process, skipping\n", filePath)
		return nil // Skip download, another process is handling it
	}

	// Small delay to allow other servers to see the file if it's being downloaded
	time.Sleep(100 * time.Millisecond)

	// Check if file already exists and is complete
	if info, err := os.Stat(localPath); err == nil {
		fmt.Printf("[SYNC] File %s already exists (%d bytes), skipping download\n", filePath, info.Size())
		return nil
	}

	// First, get file size to decide download strategy
	reader, fileSize, err := fs.p2p.RequestFileWithOffset(serverName, filePath, 0)
	if err != nil {
		return err
	}
	reader.Close()

	// Use parallel download for large files and when enabled
	minParallelSize := int64(50 * 1024 * 1024) // 50MB minimum for parallel download
	maxConcurrency := fs.config.Sync.MaxParallelChunks

	if fileSize >= minParallelSize && maxConcurrency > 1 {
		fmt.Printf("[SYNC] Using parallel download for large file %s (%d bytes)\n", filePath, fileSize)
		err := fs.downloadFileWithConcurrency(serverName, filePath, maxConcurrency)
		if err != nil {
			fmt.Printf("[SYNC] Parallel download failed for %s, falling back to sequential: %v\n", filePath, err)
			return fs.downloadFileSequential(serverName, filePath)
		}
		return nil
	} else {
		fmt.Printf("[SYNC] Using sequential download for file %s (%d bytes)\n", filePath, fileSize)
		return fs.downloadFileSequential(serverName, filePath)
	}
}

func (fs *FileSync) downloadFileSequential(serverName, filePath string) error {
	localPath := filepath.Join(fs.config.Server.SyncDir, filePath)
	tempPath := localPath + ".tmp"

	// Check if another process is already downloading this file
	if _, err := os.Stat(tempPath); err == nil {
		fmt.Printf("[SYNC] Sequential download: File %s is already being downloaded by another process, skipping\n", filePath)
		return nil // Skip download, another process is handling it
	}

	// Check if file already exists and is complete
	if info, err := os.Stat(localPath); err == nil {
		fmt.Printf("[SYNC] Sequential download: File %s already exists (%d bytes), skipping\n", filePath, info.Size())
		return nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}

	// Check if temp file exists for resume
	var existingSize int64 = 0
	if info, err := os.Stat(tempPath); err == nil {
		existingSize = info.Size()
		fmt.Printf("[SYNC] Sequential download: Resuming from temp file, offset: %d bytes\n", existingSize)
		// Rename temp file to continue download
		if err := os.Rename(tempPath, localPath); err != nil {
			return fmt.Errorf("failed to resume from temp file: %w", err)
		}
	} else {
		fmt.Printf("[SYNC] Sequential download: Starting fresh download\n")
	}

	// Request file with offset support
	reader, fileSize, err := fs.p2p.RequestFileWithOffset(serverName, filePath, existingSize)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Create temp file for download
	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer file.Close()

	// If resuming, seek to the end
	if existingSize > 0 {
		if _, err := file.Seek(existingSize, 0); err != nil {
			file.Close()
			os.Remove(tempPath)
			return fmt.Errorf("failed to seek in temp file: %w", err)
		}
		fmt.Printf("[SYNC] Sequential download: Resumed at offset %d bytes\n", existingSize)
	}

	// Copy data with optimized buffering
	chunkSize := int64(fs.config.Sync.ChunkSize)
	totalDownloaded := existingSize

	// Use larger buffer for better performance (up to 64MB for large files)
	bufferSize := chunkSize
	if bufferSize > 64*1024*1024 { // 64MB max buffer
		bufferSize = 64 * 1024 * 1024
	}

	// Use buffered reader for better performance
	bufReader := io.LimitReader(reader, fileSize-existingSize)
	buffer := make([]byte, bufferSize)

	// Use buffered writer for better I/O performance
	bufWriter := bufio.NewWriterSize(file, 4*1024*1024) // 4MB buffer

	for {
		n, err := bufReader.Read(buffer)
		if n > 0 {
			if _, writeErr := bufWriter.Write(buffer[:n]); writeErr != nil {
				return fmt.Errorf("failed to write chunk: %w", writeErr)
			}
			totalDownloaded += int64(n)

			// Progress logging (every 10MB)
			if totalDownloaded%int64(10*1024*1024) == 0 {
				progress := float64(totalDownloaded) / float64(fileSize) * 100
				fmt.Printf("[SYNC] Download progress: %.1f%% (%d/%d bytes)\n",
					progress, totalDownloaded, fileSize)
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read chunk: %w", err)
		}
	}

	// Flush any remaining buffered data
	if err := bufWriter.Flush(); err != nil {
		return fmt.Errorf("failed to flush buffer: %w", err)
	}

	fmt.Printf("[SYNC] Sequential download completed: %s (%d bytes)\n", filePath, totalDownloaded)

	// Sync file to disk
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(tempPath)
		return fmt.Errorf("failed to sync file to disk: %w", err)
	}

	// Close file
	file.Close()

	// Verify file size
	if fileSize > 0 && totalDownloaded != fileSize {
		os.Remove(tempPath)
		fmt.Printf("[SYNC] ERROR: Sequential download size mismatch: expected %d, got %d\n", fileSize, totalDownloaded)
		return fmt.Errorf("file size mismatch: expected %d, got %d", fileSize, totalDownloaded)
	}

	// Verify file size on disk
	if info, err := os.Stat(tempPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to verify temp file: %w", err)
	} else if info.Size() != fileSize {
		os.Remove(tempPath)
		return fmt.Errorf("file size mismatch on disk: expected %d, got %d", fileSize, info.Size())
	}

	// Rename temp file to final file
	if err := os.Rename(tempPath, localPath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	fmt.Printf("[SYNC] Sequential download verification passed, calculating hash...\n")

	// Calculate hash for verification
	hash, err := fs.calculateFileHash(localPath)
	if err != nil {
		fmt.Printf("[SYNC] ERROR: Failed to calculate hash: %v\n", err)
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	fmt.Printf("[SYNC] Hash calculated: %s, recording to database...\n", hash[:16]+"...")

	// Record the download
	info, err := os.Stat(localPath)
	if err != nil {
		fmt.Printf("[SYNC] ERROR: Failed to stat final file: %v\n", err)
		return err
	}

	err = fs.db.RecordFileChange(filePath, hash, info.Size(), info.ModTime(), fs.config.Server.Name, "downloaded")
	if err != nil {
		fmt.Printf("[SYNC] ERROR: Failed to record to database: %v\n", err)
		return err
	}

	fmt.Printf("[SYNC] Sequential download fully completed for %s\n", filePath)
	return nil
}

func (fs *FileSync) GetLocalFiles() ([]p2p.FileInfo, error) {
	return fs.scanDirectory(fs.config.Server.SyncDir)
}

func (fs *FileSync) GetFileHistory(filePath string, limit int) ([]db.FileRecord, error) {
	return fs.db.GetFileHistory(filePath, limit)
}

func (fs *FileSync) GetRecentChanges(limit int) ([]db.FileRecord, error) {
	return fs.db.GetRecentChanges(fs.config.Server.Name, limit)
}

func (fs *FileSync) detectDeletedFiles(currentFiles map[string]p2p.FileInfo) []p2p.FileInfo {
	var deletedFiles []p2p.FileInfo

	for path, prevFile := range fs.previousFiles {
		if _, exists := currentFiles[path]; !exists {
			deletedFiles = append(deletedFiles, prevFile)
		}
	}

	return deletedFiles
}

func (fs *FileSync) recordDeletions(deletedFiles []p2p.FileInfo) error {
	for _, file := range deletedFiles {
		if err := fs.db.RecordFileChange(file.Path, file.Hash, file.Size, file.Modified, fs.config.Server.Name, "deleted"); err != nil {
			return err
		}
	}
	return nil
}

func (fs *FileSync) ApplyDeletion(serverName, filePath string) error {
	fmt.Printf("[SYNC] Applying deletion from %s: %s\n", serverName, filePath)

	localPath := filepath.Join(fs.config.Server.SyncDir, filePath)

	// Check if file exists
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		fmt.Printf("[SYNC] File %s already deleted or never existed\n", filePath)
		return nil
	}

	// Delete the file
	if err := os.Remove(localPath); err != nil {
		return fmt.Errorf("failed to delete file %s: %w", localPath, err)
	}

	fmt.Printf("[SYNC] Successfully deleted file: %s\n", filePath)

	// Record the deletion in our history
	if err := fs.db.RecordFileChange(filePath, "", 0, time.Now(), fs.config.Server.Name, "deleted_by_peer"); err != nil {
		fmt.Printf("[SYNC] WARNING: Failed to record deletion: %v\n", err)
	}

	return nil
}

func (fs *FileSync) syncFromPeers() error {
	peers := fs.p2p.GetPeers()
	if len(peers) == 0 {
		fmt.Printf("[SYNC] No peers available for syncing\n")
		return nil
	}

	fmt.Printf("[SYNC] Checking %d peers for new files\n", len(peers))

	// Query each peer for their file lists
	for peerName := range peers {
		if err := fs.requestFileListFromPeer(peerName); err != nil {
			fmt.Printf("[SYNC] WARNING: Failed to get file list from peer %s: %v\n", peerName, err)
			// Continue with other peers
		}
	}

	return nil
}

func (fs *FileSync) requestFileListFromPeer(peerName string) error {
	fmt.Printf("[SYNC] Requesting file list from peer: %s\n", peerName)

	// Send a request for file list to the peer
	// This would trigger the peer to send us their current file list
	// For now, we'll implement this by sending a special sync message

	syncMsg := p2p.SyncMessage{
		Server: fs.config.Server.Name,
		Type:   "request_file_list",
		Data:   map[string]interface{}{},
	}

	return fs.p2p.SendSyncMessage(peerName, syncMsg)
}

func (fs *FileSync) RequestSyncFromPeer(peerName string) error {
	fmt.Printf("[SYNC] Requesting sync from peer: %s\n", peerName)

	// This would be called when receiving a file list from a peer
	// For now, we'll implement basic logic to request missing files

	// Get peer's file list (this would come from the sync message)
	// Compare with local files
	// Download missing files

	return nil
}
