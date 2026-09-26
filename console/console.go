package console

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"nanolimiter/config"
	"nanolimiter/limiter"
)

type Console struct {
	configManager *config.ConfigManager
	limiter       *limiter.Limiter
	logsEnabled   bool
}

func NewConsole(cm *config.ConfigManager, l *limiter.Limiter) *Console {
	return &Console{
		configManager: cm,
		limiter:       l,
		logsEnabled:   true,
	}
}

// SetLogsEnabled allows dynamically enabling/disabling console logs.
func (c *Console) SetLogsEnabled(enabled bool) {
	c.logsEnabled = enabled
}

// LogsEnabled returns whether console logs are enabled.
func (c *Console) LogsEnabled() bool {
	return c.logsEnabled
}

// Start launches the interactive shell (REPL) in the background.
func (c *Console) Start() {
	fmt.Println("\033[35m")
	fmt.Println("    _  __                 _     _           _ _            ")
	fmt.Println("   | |/ /__ _ _ __   ___ | |   (_)_ __ ___ (_) |_ ___ _ __ ")
	fmt.Println("   | ' // _` | '_ \\ / _ \\| |   | | '_ ` _ \\| | __/ _ \\ '__|")
	fmt.Println("   | . \\ (_| | | | | (_) | |___| | | | | | | | |_|  __/ |   ")
	fmt.Println("   |_|\\_\\__,_|_| |_|\\___/|_____|_|_| |_| |_|_|\\__\\___|_|   ")
	fmt.Println("\033[0m")
	fmt.Println("   \033[1;36mNanolimiter Terminal Console Started!\033[0m")
	fmt.Println("   Created by mesutozansoftware")
	fmt.Println("   Type \033[33mhelp\033[0m or \033[33m?\033[0m for instructions.")
	fmt.Println("   Type \033[31mexit\033[0m to close the server.")
	fmt.Println()

	go c.readLoop()
}

func (c *Console) readLoop() {
	scanner := bufio.NewScanner(os.Stdin)
	c.printPrompt()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			c.handleCommand(line)
		}
		c.printPrompt()
	}
}

func (c *Console) printPrompt() {
	// Color prompt in magenta
	fmt.Print("\033[35mnanolimiter> \033[0m")
}

func (c *Console) handleCommand(line string) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "help", "?":
		c.cmdHelp()
	case "status":
		c.cmdStatus()
	case "ban":
		c.cmdBan(args)
	case "unban":
		c.cmdUnban(args)
	case "whitelist":
		c.cmdWhitelist(args)
	case "blacklist":
		c.cmdBlacklist(args)
	case "limit":
		c.cmdLimit(args)
	case "list-bans":
		c.cmdListBans()
	case "clear-bans":
		c.cmdClearBans()
	case "autoban":
		c.cmdAutoban(args)
	case "logs":
		c.cmdLogs(args)
	case "exit", "quit":
		fmt.Println("\033[31m[-] Stopping Nanolimiter...\033[0m")
		os.Exit(0)
	default:
		fmt.Printf("\033[31m[✗] Unknown command: %s. Type 'help' for instructions.\033[0m\n", cmd)
	}
}

func (c *Console) cmdHelp() {
	fmt.Println("\033[1;34m=== Available Commands ===\033[0m")
	fmt.Println("  \033[33mstatus\033[0m                      : Shows system status, configuration, and stats.")
	fmt.Println("  \033[33mban <ip> [duration]\033[0m         : Blocks an IP address (duration in seconds, defaults if omitted).")
	fmt.Println("  \033[33munban <ip>\033[0m                  : Unblocks a banned IP address.")
	fmt.Println("  \033[33mwhitelist <add/remove/list> [ip]\033[0m: Manages whitelist IPs.")
	fmt.Println("  \033[33mblacklist <add/remove/list> [ip]\033[0m: Manages blacklist IPs.")
	fmt.Println("  \033[33mlimit <rps> [rpm]\033[0m           : Configures RPS (req/sec) and RPM (req/min) limits.")
	fmt.Println("  \033[33mlist-bans\033[0m                   : Lists all active banned IPs and remaining durations.")
	fmt.Println("  \033[33mclear-bans\033[0m                  : Unbans all currently banned IP addresses.")
	fmt.Println("  \033[33mautoban <on/off>\033[0m            : Toggles automatic ban of abusive clients.")
	fmt.Println("  \033[33mlogs <on/off>\033[0m               : Toggles live stdout requests log stream.")
	fmt.Println("  \033[33mexit / quit\033[0m                 : Shuts down Nanolimiter.")
}

func (c *Console) cmdStatus() {
	cfg := c.configManager.Get()
	allowed, blocked, clientCount := c.limiter.GetStats()

	fmt.Println("\033[1;36m=== Nanolimiter System Status ===\033[0m")
	fmt.Printf("  • Created By           : mesutozansoftware\n")
	fmt.Printf("  • Listening Port       : %d\n", cfg.Port)
	if cfg.BackendURL != "" {
		fmt.Printf("  • Target Backend       : %s\n", cfg.BackendURL)
	} else {
		fmt.Println("  • Target Backend       : \033[33m[Mock Mode] (No external proxying)\033[0m")
	}
	fmt.Printf("  • Threshold Limits     : %.1f RPS | %.1f RPM\n", cfg.RateLimitRPS, cfg.RateLimitRPM)
	fmt.Printf("  • Auto Ban             : %s\n", formatBoolColor(cfg.AutoBanEnabled))
	fmt.Printf("  • Ban Duration         : %d seconds\n", cfg.BanDurationSeconds)
	fmt.Printf("  • Whitelisted IPs      : %d\n", len(cfg.Whitelist))
	fmt.Printf("  • Blacklisted IPs      : %d\n", len(cfg.Blacklist))
	fmt.Printf("  • Active Banned IPs    : %d\n", len(c.limiter.GetBans()))
	fmt.Printf("  • Monitored Clients    : %d\n", clientCount)
	fmt.Println("\033[1;34m=== Traffic Statistics ===\033[0m")
	fmt.Printf("  • Allowed Requests     : \033[32m%d\033[0m\n", allowed)
	fmt.Printf("  • Blocked Requests     : \033[31m%d\033[0m\n", blocked)
}

func (c *Console) cmdBan(args []string) {
	if len(args) == 0 {
		fmt.Println("\033[31m[✗] Error: IP address is required. Usage: ban <ip> [duration]\033[0m")
		return
	}

	ip := args[0]
	if net.ParseIP(ip) == nil {
		fmt.Printf("\033[31m[✗] Error: Invalid IP address: %s\033[0m\n", ip)
		return
	}

	duration := c.configManager.Get().BanDurationSeconds
	if len(args) > 1 {
		d, err := strconv.Atoi(args[1])
		if err != nil || d <= 0 {
			fmt.Println("\033[31m[✗] Error: Invalid duration. Must be a positive integer.\033[0m")
			return
		}
		duration = d
	}

	c.limiter.Ban(ip, duration, "Manually banned from terminal console")
	fmt.Printf("\033[32m[✓] IP %s banned for %d seconds.\033[0m\n", ip, duration)
}

func (c *Console) cmdUnban(args []string) {
	if len(args) == 0 {
		fmt.Println("\033[31m[✗] Error: IP address is required. Usage: unban <ip>\033[0m")
		return
	}

	ip := args[0]
	c.limiter.Unban(ip)
	fmt.Printf("\033[32m[✓] IP %s unbanned.\033[0m\n", ip)
}

func (c *Console) cmdWhitelist(args []string) {
	if len(args) == 0 {
		fmt.Println("\033[31m[✗] Error: Subcommand required. Usage: whitelist <add/remove/list> [ip]\033[0m")
		return
	}

	action := strings.ToLower(args[0])
	switch action {
	case "list":
		cfg := c.configManager.Get()
		fmt.Println("\033[1;34m=== Whitelisted IP Addresses ===\033[0m")
		if len(cfg.Whitelist) == 0 {
			fmt.Println("  (No whitelisted IPs)")
		} else {
			for _, ip := range cfg.Whitelist {
				fmt.Printf("  • %s\n", ip)
			}
		}
	case "add":
		if len(args) < 2 {
			fmt.Println("\033[31m[✗] Error: IP address is required.\033[0m")
			return
		}
		ip := args[1]
		if net.ParseIP(ip) == nil {
			fmt.Printf("\033[31m[✗] Error: Invalid IP address: %s\033[0m\n", ip)
			return
		}
		err := c.configManager.Update(func(cfg *config.Config) {
			for _, item := range cfg.Whitelist {
				if item == ip {
					return // Already exists
				}
			}
			cfg.Whitelist = append(cfg.Whitelist, ip)
		})
		if err != nil {
			fmt.Printf("\033[31m[✗] Failed to save config: %v\033[0m\n", err)
		} else {
			fmt.Printf("\033[32m[✓] IP %s added to whitelist.\033[0m\n", ip)
		}
	case "remove":
		if len(args) < 2 {
			fmt.Println("\033[31m[✗] Error: IP address is required.\033[0m")
			return
		}
		ip := args[1]
		err := c.configManager.Update(func(cfg *config.Config) {
			var newList []string
			for _, item := range cfg.Whitelist {
				if item != ip {
					newList = append(newList, item)
				}
			}
			cfg.Whitelist = newList
		})
		if err != nil {
			fmt.Printf("\033[31m[✗] Failed to save config: %v\033[0m\n", err)
		} else {
			fmt.Printf("\033[32m[✓] IP %s removed from whitelist.\033[0m\n", ip)
		}
	default:
		fmt.Println("\033[31m[✗] Invalid action. Usage: whitelist <add/remove/list> [ip]\033[0m")
	}
}

func (c *Console) cmdBlacklist(args []string) {
	if len(args) == 0 {
		fmt.Println("\033[31m[✗] Error: Subcommand required. Usage: blacklist <add/remove/list> [ip]\033[0m")
		return
	}

	action := strings.ToLower(args[0])
	switch action {
	case "list":
		cfg := c.configManager.Get()
		fmt.Println("\033[1;34m=== Blacklisted IP Addresses ===\033[0m")
		if len(cfg.Blacklist) == 0 {
			fmt.Println("  (No blacklisted IPs)")
		} else {
			for _, ip := range cfg.Blacklist {
				fmt.Printf("  • %s\n", ip)
			}
		}
	case "add":
		if len(args) < 2 {
			fmt.Println("\033[31m[✗] Error: IP address is required.\033[0m")
			return
		}
		ip := args[1]
		if net.ParseIP(ip) == nil {
			fmt.Printf("\033[31m[✗] Error: Invalid IP address: %s\033[0m\n", ip)
			return
		}
		err := c.configManager.Update(func(cfg *config.Config) {
			for _, item := range cfg.Blacklist {
				if item == ip {
					return // Already exists
				}
			}
			cfg.Blacklist = append(cfg.Blacklist, ip)
		})
		if err != nil {
			fmt.Printf("\033[31m[✗] Failed to save config: %v\033[0m\n", err)
		} else {
			fmt.Printf("\033[32m[✓] IP %s blacklisted (requests will be permanently rejected).\033[0m\n", ip)
		}
	case "remove":
		if len(args) < 2 {
			fmt.Println("\033[31m[✗] Error: IP address is required.\033[0m")
			return
		}
		ip := args[1]
		err := c.configManager.Update(func(cfg *config.Config) {
			var newList []string
			for _, item := range cfg.Blacklist {
				if item != ip {
					newList = append(newList, item)
				}
			}
			cfg.Blacklist = newList
		})
		if err != nil {
			fmt.Printf("\033[31m[✗] Failed to save config: %v\033[0m\n", err)
		} else {
			fmt.Printf("\033[32m[✓] IP %s removed from blacklist.\033[0m\n", ip)
		}
	default:
		fmt.Println("\033[31m[✗] Invalid action. Usage: blacklist <add/remove/list> [ip]\033[0m")
	}
}

func (c *Console) cmdLimit(args []string) {
	if len(args) == 0 {
		fmt.Println("\033[31m[✗] Error: RPS limit is required. Usage: limit <rps> [rpm]\033[0m")
		return
	}

	rps, err := strconv.ParseFloat(args[0], 64)
	if err != nil || rps <= 0 {
		fmt.Println("\033[31m[✗] Error: Invalid RPS value. Must be a positive number.\033[0m")
		return
	}

	rpm := rps * 15 // Default RPM proportional to RPS
	if len(args) > 1 {
		val, err := strconv.ParseFloat(args[1], 64)
		if err != nil || val <= 0 {
			fmt.Println("\033[31m[✗] Error: Invalid RPM value. Must be a positive number.\033[0m")
			return
		}
		rpm = val
	}

	err = c.configManager.Update(func(cfg *config.Config) {
		cfg.RateLimitRPS = rps
		cfg.RateLimitRPM = rpm
	})
	if err != nil {
		fmt.Printf("\033[31m[✗] Failed to save config: %v\033[0m\n", err)
	} else {
		fmt.Printf("\033[32m[✓] Limits updated to %.1f RPS and %.1f RPM.\033[0m\n", rps, rpm)
	}
}

func (c *Console) cmdListBans() {
	activeBans := c.limiter.GetBans()
	fmt.Println("\033[1;31m=== Active Banned IP Addresses ===\033[0m")
	if len(activeBans) == 0 {
		fmt.Println("  (No active banned IPs)")
		return
	}

	now := time.Now()
	for _, ban := range activeBans {
		remaining := ban.ExpiresAt.Sub(now).Round(time.Second)
		fmt.Printf("  • \033[1;31m%-16s\033[0m | Remaining: \033[33m%-6s\033[0m | Reason: %s\n", ban.IP, remaining.String(), ban.Reason)
	}
}

func (c *Console) cmdClearBans() {
	c.limiter.ClearBans()
	fmt.Println("\033[32m[✓] All active bans cleared successfully.\033[0m")
}

func (c *Console) cmdAutoban(args []string) {
	if len(args) == 0 {
		fmt.Println("\033[31m[✗] Error: State is required. Usage: autoban <on/off>\033[0m")
		return
	}

	state := strings.ToLower(args[0])
	var enabled bool
	if state == "on" || state == "1" || state == "true" {
		enabled = true
	} else if state == "off" || state == "0" || state == "false" {
		enabled = false
	} else {
		fmt.Println("\033[31m[✗] Invalid value. Use 'on' or 'off'.\033[0m")
		return
	}

	err := c.configManager.Update(func(cfg *config.Config) {
		cfg.AutoBanEnabled = enabled
	})
	if err != nil {
		fmt.Printf("\033[31m[✗] Failed to save config: %v\033[0m\n", err)
	} else {
		fmt.Printf("\033[32m[✓] Auto-ban setting updated to %s.\033[0m\n", formatOnOff(enabled))
	}
}

func (c *Console) cmdLogs(args []string) {
	if len(args) == 0 {
		fmt.Println("\033[31m[✗] Error: State is required. Usage: logs <on/off>\033[0m")
		return
	}

	state := strings.ToLower(args[0])
	var enabled bool
	if state == "on" || state == "1" || state == "true" {
		enabled = true
	} else if state == "off" || state == "0" || state == "false" {
		enabled = false
	} else {
		fmt.Println("\033[31m[✗] Invalid value. Use 'on' or 'off'.\033[0m")
		return
	}

	c.SetLogsEnabled(enabled)
	fmt.Printf("\033[32m[✓] Console logs stream turned %s.\033[0m\n", formatOnOff(enabled))
}

func formatBoolColor(b bool) string {
	if b {
		return "\033[32mEnabled\033[0m"
	}
	return "\033[31mDisabled\033[0m"
}

func formatOnOff(b bool) string {
	if b {
		return "\033[32mON\033[0m"
	}
	return "\033[31mOFF\033[0m"
}
