package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	reEnvLine   = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$`)
	reLocalPort = regexp.MustCompile(`(?i)(?:https?|ws|wss)://(?:127\.0\.0\.1|localhost|0\.0\.0\.0|\[::1\]):(\d+)`)
	reHostPort  = regexp.MustCompile(`(?i)(?:127\.0\.0\.1|localhost|0\.0\.0\.0):(\d+)`)
)

func discoverEnvDeps(root string, specs []projectSpec) map[string][]string {
	type meta struct {
		name string
		path string
		port string
	}
	metas := make([]meta, 0, len(specs))
	portOwner := map[string]string{}
	nameSet := map[string]bool{}

	for _, s := range specs {
		if s.Name == "" {
			continue
		}
		nameSet[s.Name] = true
		m := meta{name: s.Name, path: s.Path, port: strings.TrimSpace(s.Port)}
		if m.port == "" {
			m.port = readPortFromEnvFiles(filepath.Join(root, s.Path))
		}
		if m.port != "" {
			portOwner[m.port] = s.Name
		}
		metas = append(metas, m)
	}
	names := make([]string, 0, len(metas))
	for _, m := range metas {
		names = append(names, m.name)
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })

	deps := map[string][]string{}
	add := func(from, to string) {
		if from == "" || to == "" || from == to {
			return
		}
		if !nameSet[to] {
			return
		}
		for _, d := range deps[from] {
			if d == to {
				return
			}
		}
		deps[from] = append(deps[from], to)
	}

	infraPorts := map[string]bool{
		"27017": true, "5432": true, "5672": true, "15672": true, "6379": true,
	}

	for _, m := range metas {
		envText := readEnvFiles(filepath.Join(root, m.path))
		if envText == "" {
			continue
		}
		for _, port := range extractLocalPorts(envText) {
			if infraPorts[port] || port == m.port {
				continue
			}
			if owner, ok := portOwner[port]; ok {
				add(m.name, owner)
			}
		}
		sc := bufio.NewScanner(strings.NewReader(envText))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			mline := reEnvLine.FindStringSubmatch(line)
			if mline == nil {
				continue
			}
			key := strings.ToUpper(mline[1])
			val := strings.Trim(mline[2], `"' `)
			if val == "" {
				continue
			}
			if !strings.Contains(key, "URL") && !strings.Contains(key, "API") &&
				!strings.Contains(key, "HOST") && !strings.Contains(key, "BACKEND") &&
				!strings.Contains(key, "FRONTEND") && !strings.Contains(key, "WS") {
				if !strings.Contains(strings.ToLower(val), "http") && !strings.Contains(val, "127.0.0.1") && !strings.Contains(strings.ToLower(val), "localhost") {
					continue
				}
			}
			low := strings.ToLower(val)
			for _, other := range names {
				if other == m.name {
					continue
				}
				oLow := strings.ToLower(other)
				oSnake := strings.ReplaceAll(oLow, "-", "_")
				if strings.Contains(low, oLow) || strings.Contains(strings.ToLower(key), oSnake) || strings.Contains(strings.ToLower(key), oLow) {
					if strings.Contains(low, oLow) || strings.Contains(strings.ToLower(key), oSnake) {
						add(m.name, other)
					}
				}
			}
		}
	}

	for k := range deps {
		sort.Strings(deps[k])
	}
	return deps
}

func readPortFromEnvFiles(dir string) string {
	env := readEnvFiles(dir)
	sc := bufio.NewScanner(strings.NewReader(env))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := reEnvLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := strings.ToUpper(m[1])
		if key == "PORT" || key == "HTTP_PORT" {
			return strings.Trim(m[2], `"' `)
		}
	}
	return ""
}

func readEnvFiles(dir string) string {
	var b strings.Builder
	for _, name := range []string{".env", ".env.development", ".env.local"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}

func extractLocalPorts(envText string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, m := range reLocalPort.FindAllStringSubmatch(envText, -1) {
		add(m[1])
	}
	for _, m := range reHostPort.FindAllStringSubmatch(envText, -1) {
		add(m[1])
	}
	return out
}
func topoSortServices(names []string, deps map[string][]string) []string {
	index := map[string]int{}
	for i, n := range names {
		index[n] = i
	}
	visited := map[string]int{}
	var out []string
	var visit func(string)
	visit = func(n string) {
		if visited[n] == 2 {
			return
		}
		if visited[n] == 1 {
			return
		}
		visited[n] = 1
		for _, d := range deps[n] {
			if _, ok := index[d]; ok {
				visit(d)
			}
		}
		visited[n] = 2
		out = append(out, n)
	}
	for _, n := range names {
		visit(n)
	}
	return out
}
func sccID(names []string, deps map[string][]string) map[string]int {
	indexOf := map[string]int{}
	for i, n := range names {
		indexOf[n] = i
	}
	index := 0
	stack := []string{}
	onStack := map[string]bool{}
	idx := map[string]int{}
	low := map[string]int{}
	comp := map[string]int{}
	compN := 0

	var strongconnect func(string)
	strongconnect = func(v string) {
		idx[v] = index
		low[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range deps[v] {
			if _, ok := indexOf[w]; !ok {
				continue
			}
			if _, seen := idx[w]; !seen {
				strongconnect(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] && idx[w] < low[v] {
				low[v] = idx[w]
			}
		}

		if low[v] == idx[v] {
			for {
				n := len(stack) - 1
				w := stack[n]
				stack = stack[:n]
				onStack[w] = false
				comp[w] = compN
				if w == v {
					break
				}
			}
			compN++
		}
	}

	for _, n := range names {
		if _, seen := idx[n]; !seen {
			strongconnect(n)
		}
	}
	return comp
}
func cycleMembers(names []string, deps map[string][]string) map[string]bool {
	comp := sccID(names, deps)
	size := map[int]int{}
	for _, id := range comp {
		size[id]++
	}
	self := map[string]bool{}
	for _, n := range names {
		for _, d := range deps[n] {
			if d == n {
				self[n] = true
			}
		}
	}
	out := map[string]bool{}
	for _, n := range names {
		if size[comp[n]] > 1 || self[n] {
			out[n] = true
		}
	}
	return out
}
func waitableDeps(from string, deps []string, scc map[string]int, inCycle map[string]bool) (wait []string, skipped []string) {
	my := scc[from]
	for _, d := range deps {
		if inCycle[from] && inCycle[d] && scc[d] == my {
			skipped = append(skipped, d)
			continue
		}
		wait = append(wait, d)
	}
	return wait, skipped
}

func waitForPort(port string, timeout time.Duration) bool {
	return waitForPortCancel(port, timeout, nil)
}

func waitForPortCancel(port string, timeout time.Duration, cancel <-chan struct{}) bool {
	if port == "" {
		return true
	}
	deadline := time.Now().Add(timeout)
	addr := net.JoinHostPort("127.0.0.1", port)
	for time.Now().Before(deadline) {
		if cancel != nil {
			select {
			case <-cancel:
				return false
			default:
			}
		}
		conn, err := net.DialTimeout("tcp", addr, 400*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			client := &http.Client{Timeout: 400 * time.Millisecond}
			resp, err := client.Get("http://" + addr + "/")
			if err == nil {
				_ = resp.Body.Close()
			}
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

func waitForHTTP(rawURL string, timeout time.Duration) bool {
	if rawURL == "" {
		return true
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return waitForPort(rawURL, timeout)
	}
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(rawURL)
		if err == nil {
			_ = resp.Body.Close()
			return true
		}
		host := u.Hostname()
		port := u.Port()
		if port == "" {
			if u.Scheme == "https" {
				port = "443"
			} else {
				port = "80"
			}
		}
		if conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 300*time.Millisecond); err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

func formatDeps(deps []string) string {
	if len(deps) == 0 {
		return ""
	}
	return fmt.Sprintf("needs %s", strings.Join(deps, ", "))
}
