package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"nanolimiter/config"
	"nanolimiter/limiter"
)

type LogEvent struct {
	Timestamp time.Time `json:"timestamp"`
	IP        string    `json:"ip"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	Allowed   bool      `json:"allowed"`
	Banned    bool      `json:"banned"`
	Reason    string    `json:"reason"`
}

type ProxyServer struct {
	configManager *config.ConfigManager
	limiter       *limiter.Limiter
	revProxy      *httputil.ReverseProxy
	targetURL     *url.URL
	LogChan       chan LogEvent
}

func NewProxyServer(cm *config.ConfigManager, l *limiter.Limiter) (*ProxyServer, error) {
	ps := &ProxyServer{
		configManager: cm,
		limiter:       l,
		LogChan:       make(chan LogEvent, 2000), // Buffer size to handle spike traffic without blocking Go threads
	}

	cfg := cm.Get()
	if cfg.BackendURL != "" {
		target, err := url.Parse(cfg.BackendURL)
		if err != nil {
			return nil, fmt.Errorf("invalid backend URL: %w", err)
		}
		ps.targetURL = target
		ps.revProxy = httputil.NewSingleHostReverseProxy(target)

		// Customize Director to pass X-Real-IP and X-Forwarded-For headers correctly
		originalDirector := ps.revProxy.Director
		ps.revProxy.Director = func(req *http.Request) {
			originalDirector(req)
			ip := ps.ExtractIP(req)
			req.Header.Set("X-Real-IP", ip)
			req.Header.Set("X-Forwarded-For", ip)
			req.Host = target.Host
		}
	}

	return ps, nil
}

// ServeHTTP acts as the entrypoint for all incoming client traffic.
func (ps *ProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Skip rate-limiting for the dashboard static assets and API (dashboard uses different port or path)
	// We handle this separation in the main routing
	ip := ps.ExtractIP(r)

	// Check rate limit / ban status
	allowed, banned, reason := ps.limiter.Allow(ip)

	// Create and queue log event
	logEv := LogEvent{
		Timestamp: time.Now(),
		IP:        ip,
		Method:    r.Method,
		Path:      r.URL.Path,
		Allowed:   allowed,
		Banned:    banned,
		Reason:    reason,
	}

	if !allowed {
		status := http.StatusTooManyRequests
		if banned {
			status = http.StatusForbidden
			logEv.Status = status
			ps.sendBlockResponse(w, ip, status, "Banned: " + reason)
		} else {
			logEv.Status = status
			ps.sendBlockResponse(w, ip, status, "Rate Limited: " + reason)
		}
		ps.queueLog(logEv)
		return
	}

	// Request is allowed
	logEv.Status = http.StatusOK // Default for mock, proxy will set its own status but we log as OK for success entry
	ps.queueLog(logEv)

	if ps.revProxy != nil {
		// Forward request to actual backend
		ps.revProxy.ServeHTTP(w, r)
	} else {
		// Serve Mock Backend Response (extremely light, zero external calls)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>Nanolimiter Mock Backend</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0b0e14; color: #e2e8f0; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; }
        .card { background: rgba(30, 41, 59, 0.5); backdrop-filter: blur(10px); border: 1px solid rgba(255, 255, 255, 0.1); border-radius: 16px; padding: 40px; text-align: center; box-shadow: 0 10px 30px rgba(0,0,0,0.5); max-width: 500px; }
        h1 { color: #8b5cf6; margin-top: 0; }
        p { line-height: 1.6; color: #94a3b8; }
        .ip { font-family: monospace; background: #1e293b; padding: 4px 8px; border-radius: 4px; color: #38bdf8; }
        .footer { font-size: 0.8em; color: #475569; margin-top: 25px; border-top: 1px solid rgba(255,255,255,0.05); padding-top: 15px; }
    </style>
</head>
<body>
    <div class="card">
        <h1>🛡️ Nanolimiter Shield Active</h1>
        <p>Congratulations! This page is successfully protected and proxied by <b>Nanolimiter</b>.</p>
        <p>Your requesting IP address: <span class="ip">%s</span></p>
        <p style="font-size: 0.9em; color: #64748b;">(Note: Nanolimiter is currently running in <i>Mock Backend Mode</i> and has not forwarded this request to an external server.)</p>
        <div class="footer">Powered by mesutozansoftware</div>
    </div>
</body>
</html>`, ip)
	}
}

func (ps *ProxyServer) ExtractIP(r *http.Request) string {
	// 1. Check X-Real-IP
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	// 2. Check X-Forwarded-For
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	// 3. Fallback to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func (ps *ProxyServer) sendBlockResponse(w http.ResponseWriter, ip string, status int, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>Access Denied - Nanolimiter</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f0b15; color: #f8fafc; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; }
        .card { background: rgba(30, 41, 59, 0.4); backdrop-filter: blur(12px); border: 1px solid rgba(239, 68, 68, 0.2); border-radius: 16px; padding: 40px; text-align: center; box-shadow: 0 10px 40px rgba(0,0,0,0.6); max-width: 480px; }
        .icon { font-size: 4rem; margin-bottom: 20px; }
        h1 { color: #f87171; margin-top: 0; font-size: 1.8rem; }
        p { line-height: 1.6; color: #cbd5e1; }
        .details { margin-top: 25px; padding: 15px; background: rgba(15, 11, 21, 0.6); border-radius: 8px; font-family: monospace; font-size: 0.95rem; border: 1px solid rgba(255,255,255,0.05); }
        .label { color: #94a3b8; }
        .val { color: #fca5a5; font-weight: bold; }
        .footer { font-size: 0.8em; color: #475569; margin-top: 25px; border-top: 1px solid rgba(255,255,255,0.05); padding-top: 15px; }
    </style>
</head>
<body>
    <div class="card">
        <div class="icon">🛑</div>
        <h1>Connection Blocked</h1>
        <p>The Nanolimiter Security Shield blocked requests from this IP address. Your request rate may have exceeded the configured thresholds or your IP has been blacklisted.</p>
        <div class="details">
            <div><span class="label">IP:</span> <span style="color: #38bdf8;">%s</span></div>
            <div style="margin-top: 5px;"><span class="label">Status:</span> <span class="val">%s</span></div>
            <div style="margin-top: 5px;"><span class="label">Time:</span> <span style="color: #a78bfa;">%s</span></div>
        </div>
        <div class="footer">Security Shield by mesutozansoftware</div>
    </div>
</body>
</html>`, ip, msg, time.Now().Format("15:04:05"))
}

func (ps *ProxyServer) queueLog(ev LogEvent) {
	select {
	case ps.LogChan <- ev:
	default:
		// If queue is full, discard to prevent blocking main proxy server processing
	}
}
