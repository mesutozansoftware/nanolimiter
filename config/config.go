package config

import (
	"encoding/json"
	"os"
	"sync"
)

// Config holds the configuration settings for Nanolimiter.
type Config struct {
	Port               int      `json:"port"`
	BackendURL         string   `json:"backend_url"`
	RateLimitRPS       float64  `json:"rate_limit_rps"`
	RateLimitRPM       float64  `json:"rate_limit_rpm"`
	BanDurationSeconds int      `json:"ban_duration_seconds"`
	AutoBanEnabled     bool     `json:"auto_ban_enabled"`
	Whitelist          []string `json:"whitelist"`
	Blacklist          []string `json:"blacklist"`
}

// ConfigManager handles thread-safe access to configuration.
type ConfigManager struct {
	mu       sync.RWMutex
	config   *Config
	filePath string
}

// NewConfigManager creates a new ConfigManager loading configuration from filePath.
// If the file doesn't exist, it creates a default configuration.
func NewConfigManager(filePath string) (*ConfigManager, error) {
	cm := &ConfigManager{
		filePath: filePath,
	}
	err := cm.Load()
	if err != nil {
		// If file doesn't exist, initialize default config
		if os.IsNotExist(err) {
			cm.config = DefaultConfig()
			err = cm.Save()
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return cm, nil
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Port:               8090,
		BackendURL:         "", // Empty means Mock Backend Mode is enabled
		RateLimitRPS:       10,
		RateLimitRPM:       150,
		BanDurationSeconds: 60, // 1 minute default ban
		AutoBanEnabled:     true,
		Whitelist:          []string{"127.0.0.1"},
		Blacklist:          []string{},
	}
}

// Load loads the configuration from file.
func (cm *ConfigManager) Load() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	file, err := os.Open(cm.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var cfg Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return err
	}

	cm.config = &cfg
	return nil
}

// Save saves the current configuration to file.
func (cm *ConfigManager) Save() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	file, err := os.Create(cm.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(cm.config)
}

// Get returns a copy of the current configuration.
func (cm *ConfigManager) Get() Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Deep copy to prevent race conditions when slicing Whitelist/Blacklist
	wl := make([]string, len(cm.config.Whitelist))
	copy(wl, cm.config.Whitelist)
	bl := make([]string, len(cm.config.Blacklist))
	copy(bl, cm.config.Blacklist)

	return Config{
		Port:               cm.config.Port,
		BackendURL:         cm.config.BackendURL,
		RateLimitRPS:       cm.config.RateLimitRPS,
		RateLimitRPM:       cm.config.RateLimitRPM,
		BanDurationSeconds: cm.config.BanDurationSeconds,
		AutoBanEnabled:     cm.config.AutoBanEnabled,
		Whitelist:          wl,
		Blacklist:          bl,
	}
}

// Update updates configuration settings.
func (cm *ConfigManager) Update(fn func(*Config)) error {
	cm.mu.Lock()
	fn(cm.config)
	cm.mu.Unlock()

	return cm.Save()
}
