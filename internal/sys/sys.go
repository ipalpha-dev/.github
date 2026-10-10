// Package sys hides the differences between macOS, Linux, WSL and native Windows: process trees,
// port probes, opening URLs, the clipboard and the shell npm scripts run in.
package sys

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Windows reports a native Windows build (not WSL).
const Windows = runtime.GOOS == "windows"

// ExeSuffix is ".exe" on Windows.
func ExeSuffix() string {
	if Windows {
		return ".exe"
	}
	return ""
}

// Has reports whether a command is on PATH.
func Has(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func extraDirs() []string {
	switch runtime.GOOS {
	case "windows":
		pf := os.Getenv("ProgramFiles")
		local := os.Getenv("LOCALAPPDATA")
		app := os.Getenv("APPDATA")
		return []string{
			filepath.Join(pf, "Git", "cmd"), filepath.Join(pf, "nodejs"),
			filepath.Join(pf, "Docker", "Docker", "resources", "bin"), filepath.Join(pf, "GitHub CLI"),
			filepath.Join(local, "Programs", "Git", "cmd"), filepath.Join(app, "npm"),
			filepath.Join(local, "Microsoft", "WinGet", "Links"),
		}
	case "darwin":
		return []string{"/opt/homebrew/bin", "/usr/local/bin", "/Applications/Docker.app/Contents/Resources/bin"}
	default:
		return []string{"/usr/local/bin", "/snap/bin", "/home/linuxbrew/.linuxbrew/bin"}
	}
}

// AddToPath appends the folders of known tools to PATH for this process and its children, so a
// tool installed a moment ago (winget, brew) works without opening a new terminal.
func AddToPath() {
	path := os.Getenv("PATH")
	have := map[string]bool{}
	for _, p := range filepath.SplitList(path) {
		have[strings.ToLower(filepath.Clean(p))] = true
	}
	var add []string
	for _, dir := range extraDirs() {
		if dir == "" || have[strings.ToLower(filepath.Clean(dir))] {
			continue
		}
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			add = append(add, dir)
		}
	}
	if len(add) > 0 {
		sep := string(os.PathListSeparator)
		os.Setenv("PATH", path+sep+strings.Join(add, sep))
	}
}

// IsWSL reports a Linux build running inside Windows Subsystem for Linux.
func IsWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	data, _ := os.ReadFile("/proc/version")
	return strings.Contains(strings.ToLower(string(data)), "microsoft")
}

// OSName is a friendly platform label for messages and doctor.
func OSName() string {
	switch {
	case runtime.GOOS == "darwin":
		return "macOS"
	case Windows:
		return "Windows"
	case IsWSL():
		return "Linux (WSL)"
	default:
		return "Linux"
	}
}

// OpenURL opens a URL in the default browser. argv only, never an interpolated shell string.
func OpenURL(url string) error {
	var name string
	var args []string
	switch {
	case runtime.GOOS == "darwin":
		name, args = "open", []string{url}
	case Windows:
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case IsWSL() && Has("wslview"):
		name, args = "wslview", []string{url}
	case IsWSL():
		name, args = "explorer.exe", []string{url}
	default:
		name, args = "xdg-open", []string{url}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := exec.CommandContext(ctx, name, args...).Run()
	if IsWSL() && name == "explorer.exe" {
		return nil // explorer.exe exits 1 even when it opened the page
	}
	return err
}

// Clipboard copies text; falls back to the OSC 52 terminal escape.
func Clipboard(text string) error {
	var c *exec.Cmd
	switch {
	case runtime.GOOS == "darwin":
		c = exec.Command("pbcopy")
	case Windows:
		c = exec.Command("clip")
	case IsWSL() && Has("clip.exe"):
		c = exec.Command("clip.exe")
	case Has("wl-copy"):
		c = exec.Command("wl-copy")
	case Has("xclip"):
		c = exec.Command("xclip", "-selection", "clipboard")
	case Has("xsel"):
		c = exec.Command("xsel", "--clipboard", "--input")
	default:
		fmt.Fprintf(os.Stderr, "\x1b]52;c;%s\a", b64(text))
		return nil
	}
	c.Stdin = strings.NewReader(text)
	return c.Run()
}

func b64(s string) string {
	const enc = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	src := []byte(s)
	var out strings.Builder
	for i := 0; i < len(src); i += 3 {
		var b [3]byte
		n := copy(b[:], src[i:])
		v := uint(b[0])<<16 | uint(b[1])<<8 | uint(b[2])
		for j := 0; j < 4; j++ {
			if j <= n {
				out.WriteByte(enc[(v>>(18-6*j))&63])
			} else {
				out.WriteByte('=')
			}
		}
	}
	return out.String()
}

// PortBusy reports whether anything accepts connections on, or holds, the TCP port.
func PortBusy(port int) bool {
	for _, host := range []string{"127.0.0.1", "[::1]"} {
		conn, err := net.DialTimeout("tcp", host+":"+strconv.Itoa(port), 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
	}
	for _, addr := range []string{"127.0.0.1:", ":"} {
		ln, err := net.Listen("tcp", addr+strconv.Itoa(port))
		if err != nil {
			return true
		}
		ln.Close()
	}
	return false
}

// Holder is a process listening on a port.
type Holder struct {
	PID     int
	Command string
}

// PortHolders lists the processes listening on a TCP port (best effort; empty when unknown).
func PortHolders(port int) []Holder {
	p := strconv.Itoa(port)
	switch {
	case Windows:
		out, err := exec.Command("netstat", "-ano", "-p", "TCP").Output()
		if err != nil {
			return nil
		}
		hs := ParseNetstat(string(out), p)
		for i := range hs {
			hs[i].Command = processName(hs[i].PID)
		}
		return hs
	case Has("lsof"):
		out, _ := exec.Command("lsof", "-nP", "-iTCP:"+p, "-sTCP:LISTEN", "-Fpc").Output()
		return ParseLsof(string(out))
	case Has("ss"):
		out, _ := exec.Command("ss", "-ltnpH", "sport = :"+p).Output()
		return ParseSS(string(out))
	}
	return nil
}

// ParseLsof reads `lsof -Fpc` output.
func ParseLsof(raw string) []Holder {
	var out []Holder
	var cur Holder
	seen := map[int]bool{}
	flush := func() {
		if cur.PID != 0 && !seen[cur.PID] {
			seen[cur.PID] = true
			out = append(out, cur)
		}
		cur = Holder{}
	}
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			flush()
			cur.PID, _ = strconv.Atoi(line[1:])
		case 'c':
			cur.Command = line[1:]
		}
	}
	flush()
	return out
}

// ParseSS reads `ss -ltnpH` output: LISTEN 0 511 *:3001 *:* users:(("node",pid=1234,fd=20)).
func ParseSS(raw string) []Holder {
	var out []Holder
	seen := map[int]bool{}
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.Split(line, "((")
		if len(parts) < 2 {
			continue
		}
		for _, entry := range strings.Split(strings.TrimSuffix(strings.TrimSpace(parts[1]), "))"), "),(") {
			var h Holder
			for _, f := range strings.Split(entry, ",") {
				switch {
				case strings.HasPrefix(f, "pid="):
					h.PID, _ = strconv.Atoi(strings.TrimPrefix(f, "pid="))
				case strings.HasPrefix(f, "\""):
					h.Command = strings.Trim(f, "\"")
				}
			}
			if h.PID != 0 && !seen[h.PID] {
				seen[h.PID] = true
				out = append(out, h)
			}
		}
	}
	return out
}

// ParseNetstat reads `netstat -ano -p TCP`: "  TCP    0.0.0.0:3001   0.0.0.0:0   LISTENING   1234".
func ParseNetstat(raw, port string) []Holder {
	var res []Holder
	seen := map[int]bool{}
	sc := bufio.NewScanner(strings.NewReader(raw))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 5 || !strings.EqualFold(f[0], "TCP") || !strings.EqualFold(f[3], "LISTENING") {
			continue
		}
		local := f[1]
		if i := strings.LastIndex(local, ":"); i < 0 || local[i+1:] != port {
			continue
		}
		pid, _ := strconv.Atoi(f[4])
		if pid > 0 && !seen[pid] {
			seen[pid] = true
			res = append(res, Holder{PID: pid})
		}
	}
	return res
}

// FreePort finds the first free port at or after start that is not excluded.
func FreePort(start int, excluded func(int) bool) (int, error) {
	for p := start; p < start+3000 && p < 65535; p++ {
		if excluded != nil && excluded(p) {
			continue
		}
		if !PortBusy(p) {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free port after %d", start)
}

// KillHolders terminates the listeners of a port except skip; returns how many were signaled.
func KillHolders(port, skip int) (int, error) {
	holders := PortHolders(port)
	n := 0
	for _, h := range holders {
		if h.PID == skip || h.PID == os.Getpid() {
			continue
		}
		KillPID(h.PID)
		n++
	}
	if n == 0 && len(holders) == 0 && PortBusy(port) {
		return 0, errors.New("could not identify the process holding the port")
	}
	return n, nil
}

// RunTimeout runs a command with a deadline (killing its whole tree on timeout) and returns stdout/stderr.
func RunTimeout(d time.Duration, dir, stdin, name string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	c := exec.CommandContext(ctx, name, args...)
	c.Dir = dir
	if stdin != "" {
		c.Stdin = strings.NewReader(stdin)
	}
	var out, errb safeBuffer
	c.Stdout = &out
	c.Stderr = &errb
	Detach(c)
	c.Cancel = func() error { KillTree(c); return nil }
	c.WaitDelay = 3 * time.Second
	err := c.Run()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("timed out after %s", d)
	}
	return out.String(), errb.String(), err
}

type safeBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// Output runs a command and returns trimmed stdout (the error carries stderr).
func Output(dir, name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	c.Dir = dir
	var errb strings.Builder
	c.Stderr = &errb
	out, err := c.Output()
	if err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return strings.TrimSpace(string(out)), errors.New(msg)
	}
	return strings.TrimSpace(string(out)), nil
}

// ScriptShell is the shell npm must run package scripts with. The services' scripts use POSIX
// syntax ("tsc && (tsc --watch & node --watch …)"); cmd.exe would run "&" sequentially and never
// start node, so on Windows npm gets Git Bash.
func ScriptShell() string {
	if !Windows {
		return ""
	}
	var roots []string
	if git, err := exec.LookPath("git"); err == nil {
		roots = append(roots, filepath.Dir(filepath.Dir(git))) // <Git>\cmd\git.exe → <Git>
	}
	roots = append(roots, filepath.Join(os.Getenv("ProgramFiles"), "Git"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Git"))
	for _, root := range roots {
		for _, p := range []string{filepath.Join(root, "bin", "bash.exe"), filepath.Join(root, "usr", "bin", "bash.exe")} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// NodeEnv returns the environment additions every npm child needs on this platform.
func NodeEnv() []string {
	if sh := ScriptShell(); sh != "" {
		return []string{"npm_config_script_shell=" + sh}
	}
	return nil
}

// Npm is the npm executable name for exec (npm.cmd on Windows).
func Npm() string {
	if Windows {
		return "npm.cmd"
	}
	return "npm"
}

// Npx likewise.
func Npx() string {
	if Windows {
		return "npx.cmd"
	}
	return "npx"
}
