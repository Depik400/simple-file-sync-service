package p2p

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"file-sync/config"
)

type FileInfo struct {
	Path     string    `json:"path"`
	Hash     string    `json:"hash"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

type SyncMessage struct {
	Type      string                 `json:"type"`
	Server    string                 `json:"server"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

type P2PNetwork struct {
	config     *config.Config
	httpClient *http.Client
	mu         sync.RWMutex
	peers      map[string]*config.PeerConfig
	demoMode   bool
}

func NewP2PNetwork(cfg *config.Config) *P2PNetwork {
	// Create optimized HTTP transport for better performance
	transport := &http.Transport{
		MaxIdleConns:        100,              // Keep up to 100 idle connections
		MaxIdleConnsPerHost: 10,               // Up to 10 idle connections per host
		IdleConnTimeout:     90 * time.Second, // Keep connections alive for 90 seconds
		DisableCompression:  false,            // Enable compression
		ForceAttemptHTTP2:   true,             // Try HTTP/2 when available
	}

	return &P2PNetwork{
		config: cfg,
		httpClient: &http.Client{
			Timeout:   24 * time.Hour, // Allow long transfers for large files
			Transport: transport,
		},
		peers: make(map[string]*config.PeerConfig),
	}
}

func (p *P2PNetwork) Start() error {
	fmt.Printf("[P2P] Starting P2P network for server %s\n", p.config.Server.Name)
	fmt.Printf("[P2P] HTTP timeout: %v\n", p.httpClient.Timeout)

	// Initialize peers from config
	fmt.Printf("[P2P] Initializing %d peers from config\n", len(p.config.Peers))
	for name, peer := range p.config.Peers {
		peerConfig := peer
		peerConfig.Name = name // Set name from map key
		p.peers[name] = &peerConfig
		fmt.Printf("[P2P] Added peer: %s (%s:%d)\n", name, peer.Host, peer.Port)
	}

	// Start health check routine
	go p.healthCheckRoutine()

	fmt.Printf("[P2P] P2P network started successfully\n")
	return nil
}

func (p *P2PNetwork) healthCheckRoutine() {
	fmt.Printf("[P2P] Starting health check routine (every 30 seconds)\n")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		fmt.Printf("[P2P] Running health checks for %d peers\n", len(p.peers))
		p.checkPeerHealth()
	}
}

func (p *P2PNetwork) checkPeerHealth() {
	for name, peer := range p.peers {
		go func(name string, peer *config.PeerConfig) {
			if p.demoMode {
				fmt.Printf("[P2P] DEMO: Would check health of peer %s at %s\n", name, peer.GetAddr())
				return
			}

			url := fmt.Sprintf("http://%s/health", peer.GetAddr())
			start := time.Now()
			resp, err := p.httpClient.Get(url)
			duration := time.Since(start)

			if err != nil {
				fmt.Printf("[P2P] Peer %s health check FAILED (%v) - took %v\n", name, err, duration)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				fmt.Printf("[P2P] Peer %s health check OK - took %v\n", name, duration)
			} else {
				fmt.Printf("[P2P] Peer %s health check returned status %d - took %v\n", name, resp.StatusCode, duration)
			}
		}(name, peer)
	}
}

func (p *P2PNetwork) BroadcastFileList(files []FileInfo) error {
	fmt.Printf("[P2P] Broadcasting file list with %d files to %d peers\n", len(files), len(p.peers))

	message := SyncMessage{
		Type:      "file_list",
		Server:    p.config.Server.Name,
		Data:      map[string]interface{}{"files": files},
		Timestamp: time.Now(),
	}

	return p.broadcastMessage(message)
}

func (p *P2PNetwork) SendFileList(peerName string, files []FileInfo) error {
	fmt.Printf("[P2P] Sending file list with %d files to peer %s\n", len(files), peerName)

	message := SyncMessage{
		Type:      "file_list",
		Server:    p.config.Server.Name,
		Data:      map[string]interface{}{"files": files},
		Timestamp: time.Now(),
	}

	return p.sendMessage(peerName, message)
}

func (p *P2PNetwork) BroadcastDeletions(deletedFiles []FileInfo) error {
	if p.demoMode {
		fmt.Printf("[P2P] DEMO: Would broadcast %d deletions\n", len(deletedFiles))
		return nil
	}

	fmt.Printf("[P2P] Broadcasting %d deletions to %d peers\n", len(deletedFiles), len(p.peers))

	message := SyncMessage{
		Type:      "file_deletions",
		Server:    p.config.Server.Name,
		Data:      map[string]interface{}{"deletions": deletedFiles},
		Timestamp: time.Now(),
	}

	return p.broadcastMessage(message)
}

func (p *P2PNetwork) RequestFile(serverName, filePath string) (io.ReadCloser, error) {
	reader, _, err := p.RequestFileWithOffset(serverName, filePath, 0)
	return reader, err
}

func (p *P2PNetwork) RequestFileRange(serverName, filePath string, start, end int64) (io.ReadCloser, int64, error) {
	if p.demoMode {
		return nil, 0, fmt.Errorf("file requests not supported in demo mode")
	}

	peer, exists := p.peers[serverName]
	if !exists {
		return nil, 0, fmt.Errorf("peer %s not found", serverName)
	}

	url := fmt.Sprintf("http://%s/files/%s", peer.GetAddr(), filePath)
	fmt.Printf("[P2P] Requesting file range %s from peer %s (%d-%d)\n", filePath, serverName, start, end)

	// Create request with Range header for specific byte range
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	startTime := time.Now()
	resp, err := p.httpClient.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		fmt.Printf("[P2P] ERROR: Failed to request file range %s from %s: %v (took %v)\n",
			filePath, serverName, err, duration)
		return nil, 0, fmt.Errorf("failed to request file range: %w", err)
	}

	fmt.Printf("[P2P] Range request response: status=%d, content-length=%d\n",
		resp.StatusCode, resp.ContentLength)

	if resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		fmt.Printf("[P2P] ERROR: Server %s returned status %d for range request %s (took %v)\n",
			serverName, resp.StatusCode, filePath, duration)
		return nil, 0, fmt.Errorf("server returned status: %d", resp.StatusCode)
	}

	// Get total file size from Content-Range header
	var fileSize int64
	contentRange := resp.Header.Get("Content-Range")
	if contentRange != "" {
		parts := strings.Split(contentRange, "/")
		if len(parts) == 2 {
			if size, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				fileSize = size
			}
		}
	}

	// Calculate expected range size
	expectedSize := end - start + 1
	fmt.Printf("[P2P] Successfully requested file range %s from %s (%d-%d, expected size: %d, took %v)\n",
		filePath, serverName, start, end, expectedSize, duration)

	return resp.Body, fileSize, nil
}

func (p *P2PNetwork) RequestFileWithOffset(serverName, filePath string, offset int64) (io.ReadCloser, int64, error) {
	if p.demoMode {
		return nil, 0, fmt.Errorf("file requests not supported in demo mode")
	}

	peer, exists := p.peers[serverName]
	if !exists {
		return nil, 0, fmt.Errorf("peer %s not found", serverName)
	}

	url := fmt.Sprintf("http://%s/files/%s", peer.GetAddr(), filePath)
	fmt.Printf("[P2P] Requesting file %s from peer %s (offset: %d)\n", filePath, serverName, offset)

	// Create request with Range header if offset > 0
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	start := time.Now()
	resp, err := p.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("[P2P] ERROR: Failed to request file %s from %s: %v (took %v)\n",
			filePath, serverName, err, duration)
		return nil, 0, fmt.Errorf("failed to request file: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		fmt.Printf("[P2P] ERROR: Server %s req %s returned status %d for file %s (took %v)\n",
			serverName, url, resp.StatusCode, filePath, duration)
		return nil, 0, fmt.Errorf("server returned status: %d", resp.StatusCode)
	}

	// Get total file size from Content-Length or Content-Range
	var fileSize int64
	if resp.StatusCode == http.StatusPartialContent {
		// Parse Content-Range header: "bytes 100-199/200"
		contentRange := resp.Header.Get("Content-Range")
		if contentRange != "" {
			parts := strings.Split(contentRange, "/")
			if len(parts) == 2 {
				if size, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
					fileSize = size
				}
			}
		}
	} else {
		fileSize = resp.ContentLength
	}

	fmt.Printf("[P2P] Successfully requested file %s from %s (size: %d, took %v)\n",
		filePath, serverName, fileSize, duration)
	return resp.Body, fileSize, nil
}

func (p *P2PNetwork) SendFile(serverName, filePath string, content io.Reader) error {
	peer, exists := p.peers[serverName]
	if !exists {
		return fmt.Errorf("peer %s not found", serverName)
	}

	url := fmt.Sprintf("http://%s/files/%s", peer.GetAddr(), filePath)
	req, err := http.NewRequest("PUT", url, content)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status: %d", resp.StatusCode)
	}

	return nil
}

func (p *P2PNetwork) broadcastMessage(message SyncMessage) error {
	if p.demoMode {
		fmt.Printf("[P2P] DEMO: Would broadcast %s message to %d peers\n", message.Type, len(p.peers))
		for name := range p.peers {
			fmt.Printf("[P2P] DEMO: Would send to peer %s\n", name)
		}
		return nil
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	fmt.Printf("[P2P] Sending %s message to %d peers\n", message.Type, len(p.peers))

	for name, peer := range p.peers {
		go func(name string, peer *config.PeerConfig) {
			url := fmt.Sprintf("http://%s/sync", peer.GetAddr())
			start := time.Now()
			resp, err := p.httpClient.Post(url, "application/json", bytes.NewReader(data))
			duration := time.Since(start)

			if err != nil {
				fmt.Printf("[P2P] ERROR: Failed to send %s message to %s: %v (took %v)\n",
					message.Type, name, err, duration)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				fmt.Printf("[P2P] Successfully sent %s message to %s (took %v)\n",
					message.Type, name, duration)
			} else {
				fmt.Printf("[P2P] WARNING: Peer %s returned status %d for %s message (took %v)\n",
					name, resp.StatusCode, message.Type, duration)
			}
		}(name, peer)
	}

	return nil
}

func (p *P2PNetwork) sendMessage(peerName string, message SyncMessage) error {
	if p.demoMode {
		fmt.Printf("[P2P] DEMO: Would send %s message to peer %s\n", message.Type, peerName)
		return nil
	}

	peer, exists := p.peers[peerName]
	if !exists {
		return fmt.Errorf("peer %s not found", peerName)
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	fmt.Printf("[P2P] Sending %s message to peer %s\n", message.Type, peerName)

	url := fmt.Sprintf("http://%s/sync", peer.GetAddr())
	start := time.Now()
	resp, err := p.httpClient.Post(url, "application/json", bytes.NewReader(data))
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("[P2P] ERROR: Failed to send %s message to %s: %v (took %v)\n",
			message.Type, peerName, err, duration)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("[P2P] WARNING: Peer %s returned status %d for %s message (took %v)\n",
			peerName, resp.StatusCode, message.Type, duration)
	} else {
		fmt.Printf("[P2P] Successfully sent %s message to %s (took %v)\n",
			message.Type, peerName, duration)
	}

	return nil
}

func (p *P2PNetwork) SendSyncMessage(peerName string, message SyncMessage) error {
	return p.sendMessage(peerName, message)
}

func (p *P2PNetwork) AddPeer(peer config.PeerConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.peers[peer.Name] = &peer
}

func (p *P2PNetwork) RemovePeer(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.peers, name)
}

func (p *P2PNetwork) GetPeers() map[string]*config.PeerConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()

	peers := make(map[string]*config.PeerConfig)
	for k, v := range p.peers {
		peers[k] = v
	}
	return peers
}

func (p *P2PNetwork) SetDemoMode(demo bool) {
	p.demoMode = demo
	fmt.Printf("[P2P] Demo mode set to: %v\n", demo)
}

func (p *P2PNetwork) SendMessageToPeer(peerName string, message SyncMessage) error {
	peer, exists := p.peers[peerName]
	if !exists {
		return fmt.Errorf("peer %s not found", peerName)
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	url := fmt.Sprintf("http://%s/sync", peer.GetAddr())
	resp, err := p.httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to send message to %s: %w", peerName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("peer %s returned status: %d", peerName, resp.StatusCode)
	}

	fmt.Printf("[P2P] Successfully sent %s message to peer %s\n", message.Type, peerName)
	return nil
}
