package limiter

import (
	"sync"
	"time"

	"nanolimiter/config"
)

type clientState struct {
	lastSeen   time.Time
	rpsTokens  float64
	rpmTokens  float64
	violations int
}

type BanInfo struct {
	IP        string    `json:"ip"`
	ExpiresAt time.Time `json:"expires_at"`
	Reason    string    `json:"reason"`
}

type Limiter struct {
	mu            sync.RWMutex
	clients       map[string]*clientState
	bans          map[string]BanInfo
	configManager *config.ConfigManager

	// Global statistics
	allowedCount uint64
	blockedCount uint64
}

func NewLimiter(cm *config.ConfigManager) *Limiter {
	l := &Limiter{
		clients:       make(map[string]*clientState),
		bans:          make(map[string]BanInfo),
		configManager: cm,
	}

	// Start background cleanup routine to keep RAM usage extremely low
	go l.startCleanupLoop(30 * time.Second)

	return l
}

// Allow checks if an IP is allowed to make a request.
// Returns (allowed, banned, reason)
func (l *Limiter) Allow(ip string) (bool, bool, string) {
	cfg := l.configManager.Get()

	// 1. Check Whitelist
	for _, wIP := range cfg.Whitelist {
		if wIP == ip {
			l.incrementAllowed()
			return true, false, ""
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// 2. Check Blacklist
	for _, bIP := range cfg.Blacklist {
		if bIP == ip {
			l.blockedCount++
			return false, false, "IP is manually blacklisted"
		}
	}

	now := time.Now()

	// 3. Check Active Bans
	if ban, exists := l.bans[ip]; exists {
		if now.Before(ban.ExpiresAt) {
			l.blockedCount++
			return false, true, "IP is banned: " + ban.Reason
		}
		// Ban expired, remove it
		delete(l.bans, ip)
	}

	// 4. Token Bucket Rate Limiting
	state, exists := l.clients[ip]
	if !exists {
		state = &clientState{
			lastSeen:  now,
			rpsTokens: cfg.RateLimitRPS,
			rpmTokens: cfg.RateLimitRPM,
		}
		l.clients[ip] = state
	}

	// Calculate elapsed time and replenish tokens
	elapsed := now.Sub(state.lastSeen).Seconds()
	state.lastSeen = now

	// Replenish RPS bucket
	state.rpsTokens += elapsed * cfg.RateLimitRPS
	if state.rpsTokens > cfg.RateLimitRPS {
		state.rpsTokens = cfg.RateLimitRPS
	}

	// Replenish RPM bucket
	state.rpmTokens += elapsed * (cfg.RateLimitRPM / 60.0)
	if state.rpmTokens > cfg.RateLimitRPM {
		state.rpmTokens = cfg.RateLimitRPM
	}

	// Check limits
	if state.rpsTokens >= 1.0 && state.rpmTokens >= 1.0 {
		state.rpsTokens -= 1.0
		state.rpmTokens -= 1.0
		state.violations = 0 // Reset violations on successful request
		l.allowedCount++
		return true, false, ""
	}

	// Blocked!
	l.blockedCount++
	state.violations++

	// 5. Auto-Ban Check
	if cfg.AutoBanEnabled && state.violations >= 5 {
		expiresAt := now.Add(time.Duration(cfg.BanDurationSeconds) * time.Second)
		l.bans[ip] = BanInfo{
			IP:        ip,
			ExpiresAt: expiresAt,
			Reason:    "Rate limit exceeded repeatedly (5+ violations)",
		}
		// Reset violations for this client since they are banned now
		state.violations = 0
		return false, true, "IP auto-banned for rate limit abuse"
	}

	return false, false, "Rate limit exceeded"
}

// Ban manually bans an IP address.
func (l *Limiter) Ban(ip string, durationSeconds int, reason string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	expiresAt := time.Now().Add(time.Duration(durationSeconds) * time.Second)
	l.bans[ip] = BanInfo{
		IP:        ip,
		ExpiresAt: expiresAt,
		Reason:    reason,
	}
}

// Unban manually removes a ban.
func (l *Limiter) Unban(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.bans, ip)
}

// GetBans returns a list of active bans.
func (l *Limiter) GetBans() []BanInfo {
	l.mu.RLock()
	defer l.mu.RUnlock()

	bansList := make([]BanInfo, 0, len(l.bans))
	now := time.Now()
	for _, ban := range l.bans {
		if now.Before(ban.ExpiresAt) {
			bansList = append(bansList, ban)
		}
	}
	return bansList
}

// ClearBans removes all bans.
func (l *Limiter) ClearBans() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.bans = make(map[string]BanInfo)
}

// GetStats returns current allowed and blocked counts.
func (l *Limiter) GetStats() (uint64, uint64, int) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.allowedCount, l.blockedCount, len(l.clients)
}

func (l *Limiter) incrementAllowed() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.allowedCount++
}

// startCleanupLoop runs in the background and cleans up inactive clients.
func (l *Limiter) startCleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		l.mu.Lock()
		now := time.Now()
		// Clean up clients inactive for more than 5 minutes
		for ip, state := range l.clients {
			if now.Sub(state.lastSeen) > 5*time.Minute {
				delete(l.clients, ip)
			}
		}
		// Clean up expired bans
		for ip, ban := range l.bans {
			if now.After(ban.ExpiresAt) {
				delete(l.bans, ip)
			}
		}
		l.mu.Unlock()
	}
}
