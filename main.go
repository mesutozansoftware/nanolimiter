package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"nanolimiter/config"
	"nanolimiter/console"
	"nanolimiter/dashboard"
	"nanolimiter/limiter"
	"nanolimiter/proxy"
)

func main() {
	// Parse CLI flags
	portFlag := flag.Int("port", 0, "Nanolimiter listening port (default: 8090)")
	backendFlag := flag.String("backend", "", "Target backend server URL to protect (e.g., http://localhost:8080)")
	configFlag := flag.String("config", "./config.json", "Path to config file")
	flag.Parse()

	// Initialize Configuration
	cm, err := config.NewConfigManager(*configFlag)
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Override config with CLI flags if provided
	err = cm.Update(func(cfg *config.Config) {
		if *portFlag != 0 {
			cfg.Port = *portFlag
		}
		if *backendFlag != "" {
			cfg.BackendURL = *backendFlag
		}
	})
	if err != nil {
		fmt.Printf("Failed to update configuration: %v\n", err)
		os.Exit(1)
	}

	cfg := cm.Get()

	// Initialize Rate Limiter
	l := limiter.NewLimiter(cm)

	// Initialize Reverse Proxy
	ps, err := proxy.NewProxyServer(cm, l)
	if err != nil {
		fmt.Printf("Failed to initialize reverse proxy: %v\n", err)
		os.Exit(1)
	}

	// Initialize Web Dashboard
	db := dashboard.NewDashboard(cm, l, ps)

	// Initialize Console REPL
	c := console.NewConsole(cm, l)

	// Setup Unified Route Multiplexer
	// All /dashboard and /api routes are routed to the dashboard, other routes to the proxy shield
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/dashboard") || strings.HasPrefix(r.URL.Path, "/api") {
			db.Handler().ServeHTTP(w, r)
		} else {
			ps.ServeHTTP(w, r)
		}
	})

	// Start Console & Dashboard
	db.Start()
	c.Start()

	// Connection Log Broker (multiplexes logs to Dashboard websocket and stdout console REPL)
	go func() {
		for ev := range ps.LogChan {
			// 1. Publish to dashboard WebSocket clients
			db.PublishLog(ev)

			// 2. Print to stdout if console logs are enabled
			if c.LogsEnabled() {
				timeStr := ev.Timestamp.Format("15:04:05")
				if !ev.Allowed {
					if ev.Banned {
						fmt.Printf("\r\033[1;31m[%s] [BANNED] IP: %-15s | %-6s %-25s | %s\033[0m\n", timeStr, ev.IP, ev.Method, ev.Path, ev.Reason)
					} else {
						fmt.Printf("\r\033[0;31m[%s] [BLOCK]  IP: %-15s | %-6s %-25s | %s\033[0m\n", timeStr, ev.IP, ev.Method, ev.Path, ev.Reason)
					}
				} else {
					fmt.Printf("\r\033[0;32m[%s] [ALLOW]  IP: %-15s | %-6s %-25s\033[0m\n", timeStr, ev.IP, ev.Method, ev.Path)
				}
				// Re-print prompt
				fmt.Print("\033[35mnanolimiter> \033[0m")
			}
		}
	}()

	// Start HTTP Server
	serverAddr := fmt.Sprintf(":%d", cfg.Port)
	go func() {
		if err := http.ListenAndServe(serverAddr, nil); err != nil {
			fmt.Printf("\n[✗] Failed to start server: %v\n", err)
			os.Exit(1)
		}
	}()

	// Show startup status in console log broker
	time.Sleep(100 * time.Millisecond) // brief sleep to print after console greetings
	fmt.Printf("\033[32m[✓] Nanolimiter Security Shield active on port %s!\033[0m\n", serverAddr)
	if cfg.BackendURL != "" {
		fmt.Printf("\033[32m[✓] Protecting & forwarding traffic to: %s\033[0m\n", cfg.BackendURL)
	} else {
		fmt.Println("\033[33m[!] No target backend server URL specified. Running in Mock Mode.\033[0m")
		fmt.Println("\033[33m[!] Manage and test the security shield here: http://localhost:8090/dashboard\033[0m")
	}
	fmt.Print("\033[35mnanolimiter> \033[0m")

	// Graceful Shutdown Listening
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\n\033[31m[-] Stopping server, saving configuration...\033[0m")
	// Save current config state
	_ = cm.Save()
	fmt.Println("\033[32m[✓] Nanolimiter stopped successfully.\033[0m")
}
