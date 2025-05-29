package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "os"
    "strings"
    "path/filepath"
)

type RouteEntry struct {
    Target string `json:"target"`
    Port   int    `json:"port,omitempty"`
}

type ProxyRules struct {
    Path         map[string]RouteEntry `json:"path"`
    Subdomain    map[string]RouteEntry `json:"subdomain"`
    Domain       map[string]RouteEntry `json:"domain"`
    TCP          map[string]RouteEntry `json:"tcp"`
    AllowedPorts []int                 `json:"allowed_ports,omitempty"`
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
func main() {
    initConfigFile()
    pathArg := flag.String("path", "", "Add path rule in key=value format")
    subdomainArg := flag.String("subdomain", "", "Add subdomain rule in key=value format")
    domainArg := flag.String("domain", "", "Add domain rule in key=value format")
    removeArg := flag.String("remove", "", "Remove rule in type=key format")
    listArg := flag.Bool("list", false, "List all rules")
    portArg := flag.Int("port", -1, "Add a port to the allowed ports list")

    flag.Parse()

    if *listArg {
        rules := loadOrCreateRules()
        printRules(rules)
        return
    }

    if *removeArg != "" {
        tipo, chave := parseRule(*removeArg)
        rules := loadOrCreateRules()
        switch tipo {
        case "path":
            if _, ok := rules.Path[chave]; ok {
                delete(rules.Path, chave)
                saveRules(rules)
                fmt.Printf("Removed path '%s'.\n", chave)
            } else {
                fmt.Printf("Path '%s' not found.\n", chave)
            }
        case "subdomain":
            if _, ok := rules.Subdomain[chave]; ok {
                delete(rules.Subdomain, chave)
                saveRules(rules)
                fmt.Printf("Removed subdomain '%s'.\n", chave)
            } else {
                fmt.Printf("Subdomain '%s' not found.\n", chave)
            }
        case "domain":
            if _, ok := rules.Domain[chave]; ok {
                delete(rules.Domain, chave)
                saveRules(rules)
                fmt.Printf("Removed domain '%s'.\n", chave)
            } else {
                fmt.Printf("Domain '%s' not found.\n", chave)
            }
        case "port":
            port := 0
            _, err := fmt.Sscanf(chave, "%d", &port)
            if err != nil || port <= 0 || port > 65535 {
                fmt.Println("Invalid or out-of-range port (1-65535).")
                return
            }
            found := false
            for i, p := range rules.AllowedPorts {
                if p == port {
                    rules.AllowedPorts = append(rules.AllowedPorts[:i], rules.AllowedPorts[i+1:]...)
                    found = true
                    break
                }
            }
            if found {
                saveRules(rules)
                fmt.Printf("Port %d removed successfully.\n", port)
            } else {
                fmt.Printf("Port %d not found in the list.\n", port)
            }
        default:
            fmt.Println("Invalid type. Use path, subdomain, domain, or port.")
        }
        return
    }

    if *portArg != -1 {
        port := *portArg
        if port <= 0 || port > 65535 {
            fmt.Println("Invalid port. Must be between 1 and 65535.")
            return
        }
        rules := loadOrCreateRules()
        exists := false
        for _, p := range rules.AllowedPorts {
            if p == port {
                exists = true
                break
            }
        }
        if !exists {
            rules.AllowedPorts = append(rules.AllowedPorts, port)
            saveRules(rules)
            fmt.Printf("Port %d added successfully.\n", port)
        } else {
            fmt.Printf("Port %d is already in the list.\n", port)
        }
        return
    }

    if *pathArg == "" && *subdomainArg == "" && *domainArg == "" {
        interactiveMenu()
        return
    }

    rules := loadOrCreateRules()

    if *pathArg != "" {
        key, value := parseRule(*pathArg)
        if _, exists := rules.Path[key]; exists {
            fmt.Printf("Path '%s' already exists. Ignoring.\n", key)
        } else {
            rules.Path[key] = RouteEntry{Target: value}
        }
    }

    if *subdomainArg != "" {
        key, value := parseRule(*subdomainArg)
        if _, exists := rules.Subdomain[key]; exists {
            fmt.Printf("Subdomain '%s' already exists. Ignoring.\n", key)
        } else {
            rules.Subdomain[key] = RouteEntry{Target: value}
        }
    }

    if *domainArg != "" {
        key, value := parseRule(*domainArg)
        if _, exists := rules.Domain[key]; exists {
            fmt.Printf("Domain '%s' already exists. Ignoring.\n", key)
        } else {
            rules.Domain[key] = RouteEntry{Target: value}
        }
    }

    saveRules(rules)
    fmt.Println("Rule added successfully.")
}

func interactiveMenu() {
    var tipo string
    var entrada string
    fmt.Println("=== Manage Rules ===")
    fmt.Println("1 - Path")
    fmt.Println("2 - Subdomain")
    fmt.Println("3 - Domain")
    fmt.Println("4 - Allowed Port")
    fmt.Println("5 - Remove Port")
    fmt.Print("Choose (1-5): ")
    fmt.Scanln(&tipo)

    switch tipo {
    case "1":
        fmt.Print("Enter path in key=value format (e.g., /api=http://localhost:3000): ")
        fmt.Scanln(&entrada)
        key, value := parseRule(entrada)
        rules := loadOrCreateRules()
        rules.Path[key] = RouteEntry{Target: value}
        saveRules(rules)
    case "2":
        fmt.Print("Enter subdomain in key=value format (e.g., admin=https://localhost:3001  or git=tcp://192.168.1.10:22): ")
        fmt.Scanln(&entrada)
        key, value := parseRule(entrada)
        rules := loadOrCreateRules()
        rules.Subdomain[key] = RouteEntry{Target: value}
        saveRules(rules)
    case "3":
        fmt.Print("Enter domain in key=value format (e.g., example.com=http://localhost:3002): ")
        fmt.Scanln(&entrada)
        key, value := parseRule(entrada)
        rules := loadOrCreateRules()
        rules.Domain[key] = RouteEntry{Target: value}
        saveRules(rules)
    case "4":
        var port int
        fmt.Print("Enter port (e.g., 8080): ")
        _, err := fmt.Scanln(&port)
        if err != nil || port <= 0 || port > 65535 {
            fmt.Println("Invalid port.")
            return
        }
        rules := loadOrCreateRules()
        exists := false
        for _, p := range rules.AllowedPorts {
            if p == port {
                exists = true
                break
            }
        }
        if !exists {
            rules.AllowedPorts = append(rules.AllowedPorts, port)
            saveRules(rules)
            fmt.Printf("Port %d added successfully.\n", port)
        } else {
            fmt.Printf("Port %d is already in the list.\n", port)
        }
    case "5":
        var port int
        fmt.Print("Enter the port you want to remove (e.g., 8080): ")
        _, err := fmt.Scanln(&port)
        if err != nil || port <= 0 || port > 65535 {
            fmt.Println("Invalid port.")
            return
        }
        rules := loadOrCreateRules()
        found := false
        for i, p := range rules.AllowedPorts {
            if p == port {
                rules.AllowedPorts = append(rules.AllowedPorts[:i], rules.AllowedPorts[i+1:]...)
                found = true
                break
            }
        }
        if found {
            saveRules(rules)
            fmt.Printf("Port %d removed successfully.\n", port)
        } else {
            fmt.Printf("Port %d not found.\n", port)
        }
    default:
        fmt.Println("Invalid option.")
        return
    }

    fmt.Println("Rule added successfully.")
}

func parseRule(input string) (string, string) {
    parts := strings.SplitN(input, "=", 2)
    if len(parts) != 2 {
        log.Fatalf("Invalid format: %s. Use key=value", input)
    }
    return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func loadOrCreateRules() ProxyRules {
    rules := ProxyRules{
        Path:         make(map[string]RouteEntry),
        Subdomain:    make(map[string]RouteEntry),
        Domain:       make(map[string]RouteEntry),
        AllowedPorts: []int{},
    }

    if _, err := os.Stat(configFile); os.IsNotExist(err) {
        return rules
    }

    file, err := os.Open(configFile)
    if err != nil {
        log.Fatalf("Error opening %s: %v", configFile, err)
    }
    defer file.Close()

    if err := json.NewDecoder(file).Decode(&rules); err != nil {
        log.Fatalf("Error decoding JSON: %v", err)
    }

    return rules
}

func saveRules(rules ProxyRules) {
    file, err := os.OpenFile(configFile, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
    if err != nil {
        log.Fatalf("Error saving %s: %v", configFile, err)
    }
    defer file.Close()

    encoder := json.NewEncoder(file)
    encoder.SetIndent("", "  ")

    if err := encoder.Encode(rules); err != nil {
        log.Fatalf("Error writing JSON: %v", err)
    }
}

func printRules(rules ProxyRules) {
    fmt.Println("=== Current Rules ===")

    fmt.Println("\n[Path]")
    for k, v := range rules.Path {
        fmt.Printf("  %s => %s\n", k, v.Target)
    }

    fmt.Println("\n[Subdomain]")
    for k, v := range rules.Subdomain {
        fmt.Printf("  %s => %s\n", k, v.Target)
    }

    fmt.Println("\n[Domain]")
    for k, v := range rules.Domain {
        fmt.Printf("  %s => %s\n", k, v.Target)
    }

    fmt.Println("\n[Allowed Ports]")
    for _, port := range rules.AllowedPorts {
        fmt.Printf("  %d\n", port)
    }
}