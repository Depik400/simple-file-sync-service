package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"file-sync/config"
	"file-sync/db"
	"file-sync/p2p"
	"file-sync/sync"

	"github.com/gorilla/mux"
)

type downloadTask struct {
	peerName string
	filePath string
}

type Server struct {
	config *config.Config
	db     *db.Database
	p2p    *p2p.P2PNetwork
	sync   *sync.FileSync
	router *mux.Router

	// Queue for pending file downloads
	downloadQueue chan downloadTask
}

func NewServer(cfg *config.Config, database *db.Database, p2pNet *p2p.P2PNetwork, syncService *sync.FileSync) *Server {
	s := &Server{
		config:        cfg,
		db:            database,
		p2p:           p2pNet,
		sync:          syncService,
		router:        mux.NewRouter(),
		downloadQueue: make(chan downloadTask, 100), // Buffer for 100 pending downloads
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	// Static files
	s.router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("web/static/"))))

	// API routes
	s.router.HandleFunc("/api/files", s.handleGetFiles).Methods("GET")
	s.router.HandleFunc("/api/files/{server}/{filePath:.*}", s.handleDownloadFile).Methods("GET")
	s.router.HandleFunc("/api/history/{filePath:.*}", s.handleGetFileHistory).Methods("GET")
	s.router.HandleFunc("/api/changes", s.handleGetRecentChanges).Methods("GET")
	s.router.HandleFunc("/api/servers", s.handleGetServers).Methods("GET")
	s.router.HandleFunc("/api/peers", s.handleGetPeers).Methods("GET")

	// P2P routes
	s.router.HandleFunc("/sync", s.handleSyncMessage).Methods("POST")
	s.router.HandleFunc("/files/{filePath:.*}", s.handleServeFile).Methods("GET")
	s.router.HandleFunc("/files/{filePath:.*}", s.handleReceiveFile).Methods("PUT")
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")

	// Web UI routes
	s.router.HandleFunc("/", s.handleIndex).Methods("GET")
	s.router.HandleFunc("/files", s.handleFilesPage).Methods("GET")
	s.router.HandleFunc("/history", s.handleHistoryPage).Methods("GET")
}

func (s *Server) Start() error {
	fmt.Printf("Starting server on %s (web + P2P API)\n", s.config.GetServerAddr())

	// Start concurrent download workers
	maxConcurrentDownloads := s.config.Sync.MaxConcurrentTransfers
	if maxConcurrentDownloads <= 0 {
		maxConcurrentDownloads = 3 // Default to 3 concurrent downloads
	}

	fmt.Printf("[SERVER] Starting %d concurrent download workers\n", maxConcurrentDownloads)
	for i := 0; i < maxConcurrentDownloads; i++ {
		go s.downloadWorker(i)
	}

	return http.ListenAndServe(s.config.GetServerAddr(), s.router)
}

func (s *Server) downloadWorker(workerID int) {
	fmt.Printf("[SERVER] Download worker %d started\n", workerID)

	for task := range s.downloadQueue {
		fmt.Printf("[SERVER] Worker %d processing download: %s from %s\n",
			workerID, task.filePath, task.peerName)

		if err := s.sync.DownloadFile(task.peerName, task.filePath); err != nil {
			fmt.Printf("[SERVER] ERROR: Worker %d failed to download %s: %v\n",
				workerID, task.filePath, err)
		} else {
			fmt.Printf("[SERVER] Worker %d successfully downloaded: %s\n",
				workerID, task.filePath)
		}
	}

	fmt.Printf("[SERVER] Download worker %d stopped\n", workerID)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("web/templates/index.html"))
	tmpl.Execute(w, s.config.Server.Name)
}

func (s *Server) handleFilesPage(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("web/templates/files.html"))
	data := struct {
		ServerName string
		Peers      map[string]*config.PeerConfig
	}{
		ServerName: s.config.Server.Name,
		Peers:      s.p2p.GetPeers(),
	}
	tmpl.Execute(w, data)
}

func (s *Server) handleHistoryPage(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("web/templates/history.html"))
	tmpl.Execute(w, s.config.Server.Name)
}

func (s *Server) handleGetFiles(w http.ResponseWriter, r *http.Request) {
	files, err := s.sync.GetLocalFiles()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func (s *Server) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverName := vars["server"]
	filePath := vars["filePath"]

	if serverName == s.config.Server.Name {
		// Local file
		localPath := filepath.Join(s.config.Server.SyncDir, filePath)
		http.ServeFile(w, r, localPath)
		return
	}

	// Remote file
	reader, err := s.p2p.RequestFile(serverName, filePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(filePath)))
	io.Copy(w, reader)
}

func (s *Server) handleGetFileHistory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	filePath := vars["filePath"]

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	history, err := s.sync.GetFileHistory(filePath, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

func (s *Server) handleGetRecentChanges(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	changes, err := s.sync.GetRecentChanges(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(changes)
}

func (s *Server) handleGetServers(w http.ResponseWriter, r *http.Request) {
	servers, err := s.db.GetAllServers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(servers)
}

func (s *Server) handleGetPeers(w http.ResponseWriter, r *http.Request) {
	peers := s.p2p.GetPeers()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peers)
}

func (s *Server) handleSyncMessage(w http.ResponseWriter, r *http.Request) {
	var msg p2p.SyncMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		fmt.Printf("[SERVER] ERROR: Failed to decode sync message: %v\n", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fmt.Printf("[SERVER] Received sync message from %s: type=%s\n", msg.Server, msg.Type)

	// Handle sync message based on type
	switch msg.Type {
	case "file_list":
		fmt.Printf("[SERVER] Processing file list from peer %s\n", msg.Server)

		// Extract file list from message
		filesData, ok := msg.Data["files"]
		if !ok {
			fmt.Printf("[SERVER] ERROR: No files data in message from %s\n", msg.Server)
			break
		}

		// Convert to proper format
		filesJson, err := json.Marshal(filesData)
		if err != nil {
			fmt.Printf("[SERVER] ERROR: Failed to marshal files data: %v\n", err)
			break
		}

		var remoteFiles []p2p.FileInfo
		if err := json.Unmarshal(filesJson, &remoteFiles); err != nil {
			fmt.Printf("[SERVER] ERROR: Failed to unmarshal files data: %v\n", err)
			break
		}

		fmt.Printf("[SERVER] Received %d files from peer %s\n", len(remoteFiles), msg.Server)

		// Check which files we need to download
		if err := s.checkAndDownloadMissingFiles(msg.Server, remoteFiles); err != nil {
			fmt.Printf("[SERVER] ERROR: Failed to sync files from %s: %v\n", msg.Server, err)
		}

	case "request_file_list":
		fmt.Printf("[SERVER] Processing file list request from peer %s\n", msg.Server)

		// Send our current file list back to the requesting peer
		if err := s.sendFileListToPeer(msg.Server); err != nil {
			fmt.Printf("[SERVER] ERROR: Failed to send file list to %s: %v\n", msg.Server, err)
		}

	case "file_deletions":
		fmt.Printf("[SERVER] Processing file deletions from peer %s\n", msg.Server)

		// Extract deletions list from message
		deletionsData, ok := msg.Data["deletions"]
		if !ok {
			fmt.Printf("[SERVER] ERROR: No deletions data in message from %s\n", msg.Server)
			break
		}

		// Convert to proper format
		deletionsJson, err := json.Marshal(deletionsData)
		if err != nil {
			fmt.Printf("[SERVER] ERROR: Failed to marshal deletions data: %v\n", err)
			break
		}

		var deletedFiles []p2p.FileInfo
		if err := json.Unmarshal(deletionsJson, &deletedFiles); err != nil {
			fmt.Printf("[SERVER] ERROR: Failed to unmarshal deletions data: %v\n", err)
			break
		}

		fmt.Printf("[SERVER] Received %d deletions from peer %s\n", len(deletedFiles), msg.Server)

		// Apply deletions
		if err := s.applyDeletionsFromPeer(msg.Server, deletedFiles); err != nil {
			fmt.Printf("[SERVER] ERROR: Failed to apply deletions from %s: %v\n", msg.Server, err)
		}

	case "request_sync":
		fmt.Printf("[SERVER] Received sync request from %s\n", msg.Server)
		// Send our file list back
		localFiles, err := s.sync.GetLocalFiles()
		if err != nil {
			fmt.Printf("[SERVER] ERROR: Failed to get local files: %v\n", err)
			break
		}

		response := p2p.SyncMessage{
			Type:      "file_list",
			Server:    s.config.Server.Name,
			Data:      map[string]interface{}{"files": localFiles},
			Timestamp: time.Now(),
		}

		if err := s.p2p.SendMessageToPeer(msg.Server, response); err != nil {
			fmt.Printf("[SERVER] ERROR: Failed to send file list to %s: %v\n", msg.Server, err)
		}

	default:
		fmt.Printf("[SERVER] WARNING: Unknown message type: %s from %s\n", msg.Type, msg.Server)
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) sendFileListToPeer(peerName string) error {
	fmt.Printf("[SERVER] Sending file list to peer: %s\n", peerName)

	// Get current local files
	localFiles, err := s.sync.GetCurrentFiles()
	if err != nil {
		return fmt.Errorf("failed to scan local directory: %w", err)
	}

	// Send file list to the specific peer
	return s.sync.GetP2PNetwork().SendFileList(peerName, localFiles)
}

func (s *Server) handleServeFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	filePath := vars["filePath"]

	// Get current working directory
	cwd, _ := os.Getwd()
	fmt.Printf("[SERVER] CWD: %s, SyncDir: %s\n", cwd, s.config.Server.SyncDir)

	localPath := filepath.Join(s.config.Server.SyncDir, filePath)
	fmt.Printf("[SERVER] Serving file: %s (local path: %s, sync_dir: %s)\n", filePath, localPath, s.config.Server.SyncDir)

	// Check if file exists
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		fmt.Printf("[SERVER] ERROR: File not found: %s (sync_dir: %s, filePath: %s)\n", localPath, s.config.Server.SyncDir, filePath)
		http.NotFound(w, r)
		return
	}

	fmt.Printf("[SERVER] File exists, serving: %s\n", localPath)
	http.ServeFile(w, r, localPath)
}

func (s *Server) handleReceiveFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	filePath := vars["filePath"]

	localPath := filepath.Join(s.config.Server.SyncDir, filePath)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	file, err := os.Create(localPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	if _, err := io.Copy(file, r.Body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) checkAndDownloadMissingFiles(peerName string, remoteFiles []p2p.FileInfo) error {
	fmt.Printf("[SERVER] Checking for missing files from peer %s\n", peerName)

	localFiles, err := s.sync.GetLocalFiles()
	if err != nil {
		return fmt.Errorf("failed to get local files: %w", err)
	}

	// Create map of local files for quick lookup
	localFileMap := make(map[string]p2p.FileInfo)
	for _, file := range localFiles {
		localFileMap[file.Path] = file
	}

	filesToDownload := 0

	// Check each remote file
	for _, remoteFile := range remoteFiles {
		localFile, exists := localFileMap[remoteFile.Path]

		if !exists {
			// File doesn't exist locally - queue it for download
			fmt.Printf("[SERVER] File missing locally: %s - queuing download from %s\n", remoteFile.Path, peerName)
			select {
			case s.downloadQueue <- downloadTask{peerName: peerName, filePath: remoteFile.Path}:
				filesToDownload++
			default:
				fmt.Printf("[SERVER] WARNING: Download queue full, skipping %s\n", remoteFile.Path)
			}
		} else if localFile.Hash != remoteFile.Hash {
			// File exists but hash differs - could be newer version
			fmt.Printf("[SERVER] File hash mismatch for %s (local: %s, remote: %s)\n",
				remoteFile.Path, localFile.Hash[:8]+"...", remoteFile.Hash[:8]+"...")

			// For simplicity, we'll download the remote version if it's newer
			if remoteFile.Modified.After(localFile.Modified) {
				fmt.Printf("[SERVER] Remote file is newer, queuing download: %s\n", remoteFile.Path)
				select {
				case s.downloadQueue <- downloadTask{peerName: peerName, filePath: remoteFile.Path}:
					filesToDownload++
				default:
					fmt.Printf("[SERVER] WARNING: Download queue full, skipping %s\n", remoteFile.Path)
				}
			}
		}
	}

	if filesToDownload == 0 {
		fmt.Printf("[SERVER] No files to download from peer %s\n", peerName)
	} else {
		fmt.Printf("[SERVER] Queued %d files for download from peer %s\n", filesToDownload, peerName)
	}

	return nil
}

func (s *Server) applyDeletionsFromPeer(peerName string, deletedFiles []p2p.FileInfo) error {
	fmt.Printf("[SERVER] Applying %d deletions from peer %s\n", len(deletedFiles), peerName)

	deletionsApplied := 0

	for _, deletedFile := range deletedFiles {
		fmt.Printf("[SERVER] Applying deletion: %s\n", deletedFile.Path)
		if err := s.sync.ApplyDeletion(peerName, deletedFile.Path); err != nil {
			fmt.Printf("[SERVER] ERROR: Failed to apply deletion of %s: %v\n", deletedFile.Path, err)
		} else {
			fmt.Printf("[SERVER] Successfully applied deletion: %s\n", deletedFile.Path)
			deletionsApplied++
		}
	}

	if deletionsApplied == 0 {
		fmt.Printf("[SERVER] No deletions applied from peer %s\n", peerName)
	} else {
		fmt.Printf("[SERVER] Applied %d deletions from peer %s\n", deletionsApplied, peerName)
	}

	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}
