package sync

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"file-sync/config"
	"file-sync/db"
	"file-sync/p2p"
)

type FileSync struct {
	config        *config.Config
	db            *db.Database
	p2p           *p2p.P2PNetwork
	previousFiles map[string]p2p.FileInfo // Track previous file state
}

func NewFileSync(cfg *config.Config, database *db.Database, p2pNet *p2p.P2PNetwork) *FileSync {
	return &FileSync{
		config:        cfg,
		db:            database,
		p2p:           p2pNet,
		previousFiles: make(map[string]p2p.FileInfo),
	}
}

func (fs *FileSync) Start() error {
	fmt.Printf("[SYNC] Starting file sync service for server %s\n", fs.config.Server.Name)
	fmt.Printf("[SYNC] Sync directory: %s\n", fs.config.Server.SyncDir)
	fmt.Printf("[SYNC] Sync interval: %d seconds\n", fs.config.Sync.Interval)

	// Ensure sync directory exists
	if err := os.MkdirAll(fs.config.Server.SyncDir, 0755); err != nil {
		return fmt.Errorf("failed to create sync directory: %w", err)
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

func (fs *FileSync) shouldSkipFile(fileName string) bool {
	// Skip PID files
	if strings.HasSuffix(fileName, ".pid") {
		return true
	}

	// Skip hidden files
	if strings.HasPrefix(fileName, ".") {
		return true
	}

	// Skip temporary files
	if strings.HasSuffix(fileName, ".tmp") || strings.HasSuffix(fileName, ".temp") {
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

func (fs *FileSync) DownloadFile(serverName, filePath string) error {
	localPath := filepath.Join(fs.config.Server.SyncDir, filePath)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}

	// Check if file already exists and get its size for resume
	var existingSize int64 = 0
	if info, err := os.Stat(localPath); err == nil {
		existingSize = info.Size()
		fmt.Printf("[SYNC] Resuming download from offset: %d bytes\n", existingSize)
	}

	// Request file with offset support
	reader, fileSize, err := fs.p2p.RequestFileWithOffset(serverName, filePath, existingSize)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Open file for writing (append if resuming)
	var file *os.File
	if existingSize > 0 {
		file, err = os.OpenFile(localPath, os.O_APPEND|os.O_WRONLY, 0644)
	} else {
		file, err = os.Create(localPath)
	}
	if err != nil {
		return err
	}
	defer file.Close()

	// Copy data in chunks
	chunkSize := int64(fs.config.Sync.ChunkSize)
	totalDownloaded := existingSize
	buffer := make([]byte, chunkSize)

	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			if _, writeErr := file.Write(buffer[:n]); writeErr != nil {
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

	fmt.Printf("[SYNC] Download completed: %s (%d bytes)\n", filePath, totalDownloaded)

	// Verify file size
	if fileSize > 0 && totalDownloaded != fileSize {
		return fmt.Errorf("file size mismatch: expected %d, got %d", fileSize, totalDownloaded)
	}

	// Calculate hash for verification
	hash, err := fs.calculateFileHash(localPath)
	if err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	// Record the download
	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}

	return fs.db.RecordFileChange(filePath, hash, info.Size(), info.ModTime(), fs.config.Server.Name, "downloaded")
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
