# 🌐 Reverse Proxy HTTP/HTTPS + TCP (SSH) in Go

This is a flexible reverse proxy written in Go that supports:
- **HTTP/HTTPS** routing by `path`, `subdomain`, and `domain`
- **Generic TCP (SSH, MySQL, etc)** routing by DNS subdomains
- Dynamic configuration via the `proxies.json` file

---

## 📁 Project Structure
```
proxy/
├── proxserver.go # Main server
├── proxsize.go # Script to manage proxies.json
├── proxies.json # Configuration file
└── README.md # This file
```

---

## 🔧 Installation

1. Clone the repository:

```bash
git clone https://github.com/srliath/ProxSize 
cd ProxSize
```

2. build files:

```bash
go build proxserver.go 
go build proxsize.go 
```

3. example:

```bash
proxsize.exe -h #or (./proxsize -h) for help, allow ports and property proxys

# Add path rule
proxsize.exe -path "/api=http://localhost:3000"

# Add subdomain rule
proxsize.exe -subdomain "admin=http://localhost:3001"

# Add domain rule
proxsize.exe -domain "example.com=http://192.168.1.10:8080"

# Add allowed port
proxsize.exe -port 8080

# Remove allowed port
proxsize.exe -remove "port=8080"

# Remove rules
proxsize.exe  -remove "path=/api"
proxsize.exe  -remove "subdomain=admin"
proxsize.exe  -remove "domain=example.com"

# List All Rules:
proxsize.exe -list

# Example tcp, ssh, mysql, rdp, etc
proxsize.exe -subdomain "ssh=tcp://192.168.1.10:22"

# Server your proxies
proxserver.exe 
```

---

## 🛠️ Manager Commands (manager/main.go)

### Add Rules

| Type         | Command                                                                 |
|--------------|-------------------------------------------------------------------------|
| Path         | proxsize.exe -path "/api=http://localhost:3000"            |
| Subdomain    | proxsize.exe -subdomain "app=https://localhost:5000"        |
| TCP Subdomain (SSH)| proxsize.exe -subdomain "git=tcp://192.168.1.10:22" |
| Domain       | proxsize.exe -domain "example.com=http://localhost:3001"   |

### List Rules

```bash
go run main.go -list
```

### Add Allowed Port

```bash
go run main.go -port 2222
```

### Remove Port

```bash
go run main.go -remove "port=2222"
```

### Remove Specific Rule

```bash
go run main.go -remove "subdomain=git"
```

---

## 🧾 Example `proxies.json`

```json
{
  "allowed_ports": [8080, 4040, 2222],
  "path": {
    "/api": {
      "target": "http://localhost:3000"
    }
  },
  "subdomain": {
    "app": {
      "target": "https://localhost:5000" 
    },
    "git": {
      "target": "tcp://192.168.1.10:22"
    },
    "db": {
      "target": "tcp://192.168.1.20:3306"
    }
  },
  "domain": {
    "example.com": {
      "target": "http://localhost:3001"
    }
  }
}
```

---

## 🚀 Key Features

| Protocol | Routing Basis              | Supported |
|----------|----------------------------|-----------|
| HTTP     | path, subdomain, domain    | ✅        |
| HTTPS    | path, subdomain, domain    | ✅        |
| SSH      | subdomain                  | ✅        |
| MySQL    | subdomain                  | ✅        |
| SMTP     | subdomain                  | ✅        |

> All TCP services share the same port (e.g., `2222`) and are routed by subdomain.

---

## 🧪 How to Test SSH

```bash
ssh user@git.example.com -p 2222
```

It will be forwarded to `192.168.1.10:22`.

> Make sure you have a wildcard DNS pointing to the proxy's IP: `*.example.com -> PROXY_IP`

---

## 📝 Important Notes

- The TCP proxy does not handle TLS termination.
- Use SSL/TLS certificates if you want to secure HTTPS connections.
- The TCP proxy tries to detect hostname from the first TCP packet (useful for HTTP).
- For SSH, the hostname is extracted via DNS lookup only (client must use `git.example.com`).

---

## 🧩 Future Improvements (suggestions)

- Add ALPN/SNI support to better distinguish HTTPS protocols
- Build a web UI to manage rules (is really necessary?)
- Add basic authentication support
- Add detailed connection logging

---

## 💡 Questions?

Open an issue or contact me!
