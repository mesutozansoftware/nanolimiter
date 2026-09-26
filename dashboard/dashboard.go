package dashboard

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"runtime"
	"sync"
	"time"

	"nanolimiter/config"
	"nanolimiter/limiter"
	"nanolimiter/proxy"

	"github.com/gorilla/websocket"
)

//go:embed assets/*
var embeddedAssets embed.FS

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all connections for simplicity in local setups
	},
}

type Client struct {
	conn *websocket.Conn
	send chan []byte
}

type Dashboard struct {
	configManager *config.ConfigManager
	limiter       *limiter.Limiter
	proxyServer   *proxy.ProxyServer

	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

func NewDashboard(cm *config.ConfigManager, l *limiter.Limiter, ps *proxy.ProxyServer) *Dashboard {
	return &Dashboard{
		configManager: cm,
		limiter:       l,
		proxyServer:   ps,
		clients:       make(map[*Client]bool),
		register:      make(chan *Client),
		unregister:    make(chan *Client),
	}
}

// Start runs the WebSocket hub broadcasting loops.
func (d *Dashboard) Start() {
	go d.runHub()
	go d.systemStatsBroadcaster()
}

// Handler returns the HTTP handler serving dashboard pages and APIs.
func (d *Dashboard) Handler() http.Handler {
	mux := http.NewServeMux()

	// 1. Serve embedded static files
	assetsFS, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(assetsFS))
	
	// Handle /dashboard/ and redirect /dashboard to /dashboard/
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/", http.StatusMovedPermanently)
	})
	mux.Handle("/dashboard/", http.StripPrefix("/dashboard/", fileServer))

	// 2. REST API endpoints
	mux.HandleFunc("/api/config", d.handleConfig)
	mux.HandleFunc("/api/bans", d.handleBans)
	mux.HandleFunc("/api/ban", d.handleBan)
	mux.HandleFunc("/api/unban", d.handleUnban)
	mux.HandleFunc("/api/stats", d.handleStats)
	mux.HandleFunc("/api/ws", d.handleWS)

	// Custom simulation tool triggers (DoS test inside panel)
	mux.HandleFunc("/api/simulate", d.handleSimulate)

	return mux
}

func (d *Dashboard) runHub() {
	for {
		select {
		case client := <-d.register:
			d.mu.Lock()
			d.clients[client] = true
			d.mu.Unlock()
		case client := <-d.unregister:
			d.mu.Lock()
			if _, ok := d.clients[client]; ok {
				delete(d.clients, client)
				close(client.send)
			}
			d.mu.Unlock()
		}
	}
}

func (d *Dashboard) broadcast(message []byte) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for client := range d.clients {
		select {
		case client.send <- message:
		default:
			// Client queue full, unregister client
			go func(c *Client) {
				d.unregister <- c
			}(client)
		}
	}
}

// PublishLog formats and broadcasts log events to the dashboard clients.
func (d *Dashboard) PublishLog(ev proxy.LogEvent) {
	data, err := json.Marshal(map[string]interface{}{
		"type": "log",
		"data": ev,
	})
	if err == nil {
		d.broadcast(data)
	}
}

// systemStatsBroadcaster periodic updates for CPU, RAM, active limits.
func (d *Dashboard) systemStatsBroadcaster() {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		allowed, blocked, activeClients := d.limiter.GetStats()
		activeBans := len(d.limiter.GetBans())

		stats := map[string]interface{}{
			"type": "stats",
			"data": map[string]interface{}{
				"timestamp":      time.Now().Unix(),
				"memory_mb":      float64(m.Alloc) / 1024.0 / 1024.0,
				"goroutines":     runtime.NumGoroutine(),
				"allowed_total":  allowed,
				"blocked_total":  blocked,
				"active_clients": activeClients,
				"active_bans":    activeBans,
			},
		}

		data, err := json.Marshal(stats)
		if err == nil {
			d.broadcast(data)
		}
	}
}

// API Handlers
func (d *Dashboard) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(d.configManager.Get())
		return
	}

	if r.Method == http.MethodPost {
		var req config.Config
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err := d.configManager.Update(func(cfg *config.Config) {
			cfg.RateLimitRPS = req.RateLimitRPS
			cfg.RateLimitRPM = req.RateLimitRPM
			cfg.BanDurationSeconds = req.BanDurationSeconds
			cfg.AutoBanEnabled = req.AutoBanEnabled
			cfg.Whitelist = req.Whitelist
			cfg.Blacklist = req.Blacklist
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (d *Dashboard) handleBans(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d.limiter.GetBans())
}

func (d *Dashboard) handleBan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		IP       string `json:"ip"`
		Duration int    `json:"duration"`
		Reason   string `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Duration <= 0 {
		req.Duration = d.configManager.Get().BanDurationSeconds
	}
	if req.Reason == "" {
		req.Reason = "Manually banned from dashboard panel"
	}

	d.limiter.Ban(req.IP, req.Duration, req.Reason)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (d *Dashboard) handleUnban(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		IP string `json:"ip"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	d.limiter.Unban(req.IP)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (d *Dashboard) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	allowed, blocked, activeClients := d.limiter.GetStats()
	activeBans := len(d.limiter.GetBans())

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"allowed_total":  allowed,
		"blocked_total":  blocked,
		"active_clients": activeClients,
		"active_bans":    activeBans,
		"memory_mb":      float64(m.Alloc) / 1024.0 / 1024.0,
		"goroutines":     runtime.NumGoroutine(),
	})
}

// handleWS upgrades connection to WebSocket and registers client.
func (d *Dashboard) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
	}

	d.register <- client

	// Read loop (to keep socket alive and handle client closing connection)
	go func() {
		defer func() {
			d.unregister <- client
			conn.Close()
		}()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()

	// Write loop (delivering messages to client)
	go func() {
		for msg := range client.send {
			err := conn.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				break
			}
		}
	}()
}

// handleSimulate handles internal DoS test trigger
func (d *Dashboard) handleSimulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RPS      int  `json:"rps"`
		Duration int  `json:"duration"`
		Stop     bool `json:"stop"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// We'll manage simulation inside the web dashboard to make it easy.
	// If stop is requested, stop simulation
	if req.Stop {
		stopSimulation()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
		return
	}

	// Start simulation in background
	go startSimulation(req.RPS, req.Duration, d.configManager.Get().Port)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

// Simulated Attack Control Variables
var (
	simCancelChan chan struct{}
	simMu         sync.Mutex
	isSimulating  bool
)

func startSimulation(rps int, durationSec int, port int) {
	simMu.Lock()
	if isSimulating {
		if simCancelChan != nil {
			close(simCancelChan)
		}
	}
	isSimulating = true
	simCancelChan = make(chan struct{})
	simMu.Unlock()

	tickerInterval := time.Second / time.Duration(rps)
	if rps <= 0 {
		return
	}
	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()

	timeout := time.After(time.Duration(durationSec) * time.Second)
	client := &http.Client{Timeout: 500 * time.Millisecond}

	// Attack from a simulated custom IP
	// In local networks, we can pass X-Real-IP to simulate different bots!
	// This is perfect because we can show multiple IPs attacking!
	botIPs := []string{
		"192.168.10.15", "192.168.10.16", "192.168.10.17",
		"10.0.0.12", "10.0.0.13", "10.0.0.14",
		"172.16.5.99", "172.16.5.100", "8.8.8.8",
	}

	for {
		select {
		case <-simCancelChan:
			simMu.Lock()
			isSimulating = false
			simMu.Unlock()
			return
		case <-timeout:
			simMu.Lock()
			isSimulating = false
			simMu.Unlock()
			return
		case <-ticker.C:
			// Trigger HTTP request in parallel so we don't slow down simulation
			go func() {
				// Pick a random bot IP to simulate a Distributed DoS (DDoS)
				botIP := botIPs[time.Now().UnixNano()%int64(len(botIPs))]
				req, err := http.NewRequest("GET", "http://localhost:8090/", nil) // Point to proxy port
				if err == nil {
					// Nanolimiter reads X-Real-IP, so we simulate the bots easily
					req.Header.Set("X-Real-IP", botIP)
					resp, err := client.Do(req)
					if err == nil {
						resp.Body.Close()
					}
				}
			}()
		}
	}
}

func stopSimulation() {
	simMu.Lock()
	if isSimulating && simCancelChan != nil {
		close(simCancelChan)
		isSimulating = false
	}
	simMu.Unlock()
}
