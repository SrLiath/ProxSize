package main

import (
    "bufio"
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net"
    "net/http"
    "net/http/httputil"
    "net/url"
    "os"
    "strings"
    "time"
    "github.com/fsnotify/fsnotify"
    "path/filepath"
)

type RouteEntry struct {
    Target string `json:"target"`
    Port   int    `json:"port,omitempty"`
}

type RawRules map[string]json.RawMessage

type ProxyRules struct {
    Path      map[string]RouteEntry `json:"path"`
    Subdomain map[string]RouteEntry `json:"subdomain"`
    Domain    map[string]RouteEntry `json:"domain"`
    TCP       map[string]RouteEntry `json:"tcp"`
}

type RawConfig struct {
    Path         RawRules `json:"path"`
    Subdomain    RawRules `json:"subdomain"`
    Domain       RawRules `json:"domain"`
    TCP          RawRules `json:"tcp"`
    AllowedPorts []int    `json:"allowed_ports,omitempty"`
}

type FullRoute struct {
    Type   string
    Key    string
    Target string
}

var configFile string
func initConfigFile() {
    exePath, err := os.Executable()
    if err != nil {
        log.Fatalf("Error getting executable path: %v", err)
    }
    dir := filepath.Dir(exePath)
    configFile = filepath.Join(dir, "proxies.json")
}
type ServerInstance struct {
    port   int
    server *http.Server
    cancel context.CancelFunc
}

func main() {
    initConfigFile()
    instances := make(map[int]*ServerInstance)
    reloadCh := make(chan struct{}, 1)
    go watchConfig(reloadCh)
    reloadCh <- struct{}{}
    for range reloadCh {
        log.Println("🔄 Reloading configuration...")
        rules, allowedPorts := loadRules()
        portMap := map[int][]FullRoute{}
        for kind, entries := range map[string]map[string]RouteEntry{
            "path":      rules.Path,
            "subdomain": rules.Subdomain,
            "domain":    rules.Domain,
        } {
            for key, entry := range entries {

                port := entry.Port
                if port == 0 {
                    port = -1
                }
                route := FullRoute{Type: kind, Key: key, Target: entry.Target}
                portMap[port] = append(portMap[port], route)
            }
        }

        tcpSubdomains := make(map[string]string)
        for key, entry := range rules.Subdomain {
            if strings.HasPrefix(entry.Target, "tcp://") {
                tcpSubdomains[key] = entry.Target
            }
        }

        if len(tcpSubdomains) > 0 {
            if inst, ok := instances[2222]; ok {
                stopServer(inst)
            }
            inst := startTCPServer(2222, tcpSubdomains)
            instances[2222] = inst
        }

        for _, port := range allowedPorts {
            routes := append(portMap[port], portMap[-1]...)
            if len(routes) == 0 {
                if inst, ok := instances[port]; ok {
                    stopServer(inst)
                    delete(instances, port)
                }
                continue
            }
            if inst, ok := instances[port]; ok {
                stopServer(inst)
            }
            inst := startServer(port, routes)
            instances[port] = inst
        }

        for port, inst := range instances {
            if !contains(allowedPorts, port) && port != 2222 {
                stopServer(inst)
                delete(instances, port)
            }
        }
    }
}

func watchConfig(ch chan struct{}) {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        log.Fatal(err)
    }
    defer watcher.Close()
    err = watcher.Add(configFile)
    if err != nil {
        log.Fatal(err)
    }

    debounce := time.NewTimer(0)
    <-debounce.C

    for {
        select {
        case event, ok := <-watcher.Events:
            if !ok {
                return
            }
            if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
                debounce.Reset(300 * time.Millisecond)
            }
        case <-debounce.C:
            ch <- struct{}{}
        case err, ok := <-watcher.Errors:
            if !ok {
                return
            }
            log.Println("🚨 Watcher error:", err)
        }
    }
}

func stopServer(inst *ServerInstance) {
    log.Printf("🛑 Stopping server on port %d", inst.port)
    inst.cancel()
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    inst.server.Shutdown(ctx)
}

func startServer(port int, routes []FullRoute) *ServerInstance {
    addr := fmt.Sprintf(":%d", port)
    mux := http.NewServeMux()
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        host, _, _ := net.SplitHostPort(r.Host)
        if host == "" {
            host = r.Host
        }
        path := r.URL.Path
        log.Printf("📨 [%s] %s | Host: %q | Path: %q", r.Method, r.URL.String(), host, path)

        for _, route := range routes {
            switch route.Type {
            case "domain":
                if host == route.Key {
                    log.Printf("🎯 Match domain: %q -> %q", route.Key, route.Target)
                    proxyTo(w, r, route.Target, "")
                    return
                }
            case "subdomain":
                parts := strings.Split(host, ".")
                if len(parts) >= 2 && parts[0] == route.Key {
                    log.Printf("🎯 Match subdomain: %q -> %q", route.Key, route.Target)
                    proxyTo(w, r, route.Target, "")
                    return
                }
            case "path":
                if strings.HasPrefix(path, route.Key) {
                    log.Printf("🎯 Match path: %q -> %q", route.Key, route.Target)
                    proxyTo(w, r, route.Target, route.Key)
                    return
                }
            }
        }

        log.Printf("❌ No rule found for %q", r.URL.Path)
        http.NotFound(w, r)
    })

    server := &http.Server{
        Addr:    addr,
        Handler: mux,
    }

    ctx, cancel := context.WithCancel(context.Background())
    go func() {
        log.Printf("🔌 Server listening on port %d with %d routes", port, len(routes))
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("❌ Error on port %d: %v", port, err)
        }
    }()

    go func() {
        <-ctx.Done()
        shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
        defer shutdownCancel()
        server.Shutdown(shutdownCtx)
    }()

    return &ServerInstance{port: port, server: server, cancel: cancel}
}

func proxyTo(w http.ResponseWriter, r *http.Request, target, trim string) {
    remote, err := url.Parse(target)
    if err != nil {
        http.Error(w, "Invalid target", http.StatusBadGateway)
        return
    }

    hostPort := r.Host
    _, rPort, err := net.SplitHostPort(hostPort)
    if err != nil {
        if remote.Scheme == "https" {
            rPort = "443"
        } else {
            rPort = "80"
        }
    }

    host, hPort, err := net.SplitHostPort(remote.Host)
    if err != nil || hPort == "" {
        remote.Host = net.JoinHostPort(remote.Host, rPort)
    } else if host == "" {
        remote.Host = net.JoinHostPort(remote.Host, rPort)
    }

    proxy := httputil.NewSingleHostReverseProxy(remote)
    proxy.Director = func(r *http.Request) {
        r.Host = remote.Host
        r.URL.Scheme = remote.Scheme
        r.URL.Host = remote.Host
        r.URL.Path = strings.TrimPrefix(r.URL.Path, trim)
    }

    proxy.ServeHTTP(w, r)
}

func parseRouteEntry(value any) RouteEntry {
    switch v := value.(type) {
    case string:
        return RouteEntry{Target: v}
    case map[string]any:
        var target string
        targetInterface := v["target"]
        switch t := targetInterface.(type) {
        case string:
            target = t
        case []byte:
            target = string(t)
        case fmt.Stringer:
            target = t.String()
        default:
            if t != nil {
                target = fmt.Sprintf("%v", t)
            }
        }

        port := 0
        if p, ok := v["port"].(float64); ok {
            port = int(p)
        }

        if target != "" {
            return RouteEntry{Target: target, Port: port}
        }
    }
    return RouteEntry{}
}

func loadRules() (ProxyRules, []int) {
    content, err := os.ReadFile(configFile)
    if err != nil {
        log.Fatalf("❌ Error reading %s: %v", configFile, err)
    }

    var raw RawConfig
    if err := json.Unmarshal(content, &raw); err != nil {
        log.Fatalf("❌ Error decoding JSON: %v", err)
    }

    result := ProxyRules{
        Path:      make(map[string]RouteEntry),
        Subdomain: make(map[string]RouteEntry),
        Domain:    make(map[string]RouteEntry),
        TCP:       make(map[string]RouteEntry),
    }

    parseAndAdd := func(src RawRules, dst map[string]RouteEntry) {
        for k, v := range src {
            var entry RouteEntry
            if err := json.Unmarshal(v, &entry); err != nil {
                log.Printf("⚠️ Failed to parse rule %q: %v", k, err)
                continue
            }
            log.Printf("Key: %q | Target: %q | Port: %d", k, entry.Target, entry.Port)
            if entry.Target != "" {
                dst[k] = entry
            }
        }
    }

    parseAndAdd(raw.Path, result.Path)
    parseAndAdd(raw.Subdomain, result.Subdomain)
    parseAndAdd(raw.Domain, result.TCP) // ou result.Domain dependendo da intenção
    parseAndAdd(raw.TCP, result.TCP)

    return result, raw.AllowedPorts
}
func contains(slice []int, val int) bool {
    for _, item := range slice {
        if item == val {
            return true
        }
    }
    return false
}

func startTCPServer(port int, subdomains map[string]string) *ServerInstance {
    addr := fmt.Sprintf(":%d", port)
    listener, err := net.Listen("tcp", addr)
    if err != nil {
        log.Fatalf("❌ Error starting TCP server on port %d: %v", port, err)
    }

    ctx, cancel := context.WithCancel(context.Background())
    go func() {
        log.Printf("🔌 TCP Server listening on port %d for subdomains", port)
        for {
            select {
            case <-ctx.Done():
                return
            default:
                conn, err := listener.Accept()
                if err != nil {
                    log.Printf("❌ Error accepting TCP connection: %v", err)
                    continue
                }
                go handleTCPWithSubdomain(conn, subdomains)
            }
        }
    }()

    return &ServerInstance{
        port:   port,
        server: &http.Server{Addr: addr},
        cancel: cancel,
    }
}

func handleTCPWithSubdomain(client net.Conn, subdomains map[string]string) {
    defer client.Close()
    buffer := make([]byte, 1024)
    n, err := client.Read(buffer)
    if err != nil {
        log.Printf("❌ Error reading initial data: %v", err)
        return
    }

    host := extractHostnameFromData(buffer[:n], client.RemoteAddr().String())
    if host == "" {
        log.Printf("❌ Could not identify hostname from TCP connection")
        return
    }

    sub := strings.SplitN(host, ".", 2)[0]
    if target, found := subdomains[sub]; found {
        targetAddr := strings.TrimPrefix(target, "tcp://")
        log.Printf("🎯 TCP Subdomain match: %s -> %s", sub, targetAddr)
        backend, err := net.Dial("tcp", targetAddr)
        if err != nil {
            log.Printf("❌ Error connecting to TCP target (%s): %v", targetAddr, err)
            return
        }

        _, _ = backend.Write(buffer[:n])
        go io.Copy(backend, client)
        go io.Copy(client, backend)
        return
    }

    log.Printf("❌ No destination found for TCP subdomain: %s", host)
}

func extractHostnameFromData(data []byte, remoteAddr string) string {
    s := string(data)
    if strings.HasPrefix(s, "GET ") || strings.HasPrefix(s, "POST ") {
        scanner := bufio.NewScanner(bytes.NewReader(data))
        for scanner.Scan() {
            line := scanner.Text()
            if strings.HasPrefix(line, "Host:") {
                return strings.TrimSpace(strings.TrimPrefix(line, "Host:"))
            }
            if line == "" {
                break
            }
        }
    }
    return ""
}