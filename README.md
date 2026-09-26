# Nanolimiter 🛡️

**Nanolimiter** is an ultra-low RAM, high-performance HTTP reverse proxy and DDoS protection shield written in Go. It acts as an intelligent security gateway positioned in front of your backend web applications (Node.js, Python, PHP, Apache, etc.), filtering out malicious spikes, rate limit abuse, and DoS attacks before they hit your core servers.

Designed and maintained by **[mesutozansoftware](https://github.com/mesutozansoftware)**.

---

## ⚡ Key Features

- **Ultra-Low Memory Footprint**: Runs on ~8MB of RAM in idle state and around 15MB under heavy traffic load (no JVM, node_modules, or virtual machines required).
- **Cross-Platform Portability**: Compiles to a single standalone executable binary with zero external dependencies, natively supporting **Linux**, **Windows**, and **macOS**.
- **Dual Rate-Limiting Engine**: Combines Token-Bucket and Sliding Window algorithms to apply individual client request-per-second (RPS) and request-per-minute (RPM) limits.
- **Dynamic Auto-Ban Shield**: Automatically quarantines and bans abusive IP addresses for a configurable duration.
- **Embedded Glassmorphic Dashboard**: A gorgeous real-time web interface (`http://localhost:8090/dashboard`) showing traffic graphs, active bans, system stats, and live terminal logs.
- **Interactive REPL Terminal Console**: Run commands directly in the server's terminal console (`status`, `ban`, `unban`, `whitelist`, `blacklist`, `limit`) to manage security rules in real-time.
- **Built-in DoS Attack Simulator**: Includes a web-controlled client-spoofing DoS simulation module to stress test rates and verify auto-banning in local networks.

---

## 🏗️ Architecture

```
Client/Attacker ──> HTTP Requests ──> [ Nanolimiter Proxy ] ── (Clean traffic) ──> Backend Server
                                             │
                       ├── Rate Limiting & IP Checking
                       ├── Interactive Terminal REPL Console
                       └── Web Dashboard (WebSockets & APIs)
```

---

## 🚀 Getting Started

### Prerequisites
- **Go 1.16 or higher** installed.

### Installation & Build

1. Clone the repository or navigate to the project directory:
   ```bash
   git clone https://github.com/mesutozan/nanolimiter.git
   cd nanolimiter
   ```

2. Download Go dependencies:
   ```bash
   go mod tidy
   ```

3. Build Nanolimiter for your local system:
   ```bash
   go build -o nanolimiter
   ```

4. Run the executable:
   ```bash
   ./nanolimiter -port 8090 -backend http://localhost:8080
   ```
   *Note: If no `-backend` URL is specified, Nanolimiter automatically starts in **Mock Backend Mode**, serving a built-in lightweight page to facilitate testing.*

---

## 💻 Interactive Terminal Commands

When running, Nanolimiter provides an interactive shell prompt (`nanolimiter> `). You can type the following commands at any time:

| Command | Description | Example |
| :--- | :--- | :--- |
| `status` | Shows system status, configuration, and stats. | `status` |
| `ban <ip> [duration]` | Blocks an IP address (duration in seconds). | `ban 192.168.1.55 300` |
| `unban <ip>` | Unblocks a banned IP address. | `unban 192.168.1.55` |
| `whitelist <add/remove/list> [ip]` | Manages whitelisted IPs (exempt from limits). | `whitelist add 127.0.0.1` |
| `blacklist <add/remove/list> [ip]` | Manages permanently blocked IP addresses. | `blacklist add 8.8.8.8` |
| `limit <rps> [rpm]` | Configures RPS and RPM thresholds on the fly. | `limit 25 300` |
| `list-bans` | Lists all active banned IPs and remaining time. | `list-bans` |
| `clear-bans` | Unbans all currently banned IP addresses. | `clear-bans` |
| `autoban <on/off>` | Toggles auto-ban of abusive clients. | `autoban off` |
| `logs <on/off>` | Toggles live stdout request logs stream. | `logs off` |
| `exit` / `quit` | Gracefully saves config state and shuts down. | `exit` |

---

## 🛠️ Cross-Compilation (Linux & Windows)

You can cross-compile Nanolimiter from your current system to other operating systems using Go's built-in toolchain:

### Compile for Linux
```bash
GOOS=linux GOARCH=amd64 go build -o nanolimiter-linux
```

### Compile for Windows
```bash
GOOS=windows GOARCH=amd64 go build -o nanolimiter-windows.exe
```

---

## 🌐 Publishing to GitHub

To publish this project to your own GitHub repository:

1. Create a new repository on your GitHub account (named `nanolimiter`).
2. Run the following Git commands in your project folder:
   ```bash
   git init
   git add .
   git commit -m "Initial release of Nanolimiter"
   git branch -M main
   git remote add origin https://github.com/YOUR_GITHUB_USERNAME/nanolimiter.git
   git push -u origin main
   ```
   *(Be sure to replace `YOUR_GITHUB_USERNAME` with your actual username or organization name, such as `mesutozan` or `mesutozansoftware`)*.

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
