// Package ports decides, on every ./run, which host port each service really uses. Settings hold
// the preferred port; when it is busy (another program, another workspace) the service moves to a
// free one for this run, and every URL handed to the processes follows it (see Rewriter).
package ports

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
)

// Move records one port that changed for this run.
type Move struct {
	Key       string `json:"key"`
	Preferred int    `json:"preferred"`
	Actual    int    `json:"actual"`
	Holder    string `json:"holder,omitempty"`
}

// Plan is the resolved port of every key for one run.
type Plan struct {
	Actual    map[string]int `json:"actual"`
	Preferred map[string]int `json:"preferred"`
	Moves     []Move         `json:"moves,omitempty"`
}

// Probe abstracts the machine (tests replace it).
type Probe struct {
	Busy   func(port int) bool
	Ours   func(key string, port int) bool // the port is held by this workspace (its own container)
	Holder func(port int) string
	// Current returns the host port this workspace's running container already publishes for key
	// (0 when none), so a moved infra port stays put across runs instead of drifting.
	Current func(key string) int
}

// Resolve keeps every free (or already ours) preferred port and moves the others to the first free
// port that is neither a catalog default nor already taken in this plan.
func Resolve(preferred map[string]int, keys []string, probe Probe) Plan {
	plan := Plan{Actual: map[string]int{}, Preferred: map[string]int{}}
	taken := map[int]bool{}
	for _, k := range keys {
		plan.Preferred[k] = preferred[k]
	}
	// First pass: keep what can be kept, so a moved port never steals another key's preferred one.
	for _, k := range keys {
		p := preferred[k]
		if p > 0 && !taken[p] && (probe.Ours != nil && probe.Ours(k, p) || !probe.Busy(p)) {
			plan.Actual[k] = p
			taken[p] = true
		}
	}
	reserved := map[int]bool{}
	for _, k := range keys {
		reserved[preferred[k]] = true
	}
	for _, k := range keys {
		if _, ok := plan.Actual[k]; ok {
			continue
		}
		p := preferred[k]
		if probe.Current != nil {
			if cur := probe.Current(k); cur > 0 && !taken[cur] {
				plan.Actual[k] = cur
				taken[cur] = true
				plan.Moves = append(plan.Moves, Move{Key: k, Preferred: p, Actual: cur})
				continue
			}
		}
		cand := p + 1
		for ; cand < 65535; cand++ {
			if taken[cand] || reserved[cand] || catalog.IsDefaultPort(cand) {
				continue
			}
			if !probe.Busy(cand) {
				break
			}
		}
		plan.Actual[k] = cand
		taken[cand] = true
		m := Move{Key: k, Preferred: p, Actual: cand}
		if probe.Holder != nil {
			m.Holder = probe.Holder(p)
		}
		plan.Moves = append(plan.Moves, m)
	}
	sort.Slice(plan.Moves, func(i, j int) bool { return plan.Moves[i].Key < plan.Moves[j].Key })
	return plan
}

// Port of a key in this run.
func (p Plan) Port(key string) int {
	if v := p.Actual[key]; v > 0 {
		return v
	}
	return catalog.DefaultPorts()[key]
}

// Save writes the plan to .ipalpha/.state/ports.json (read by `ipalpha status` and the panel).
func (p Plan) Save(stateDir string) error {
	data, _ := json.MarshalIndent(p, "", "  ")
	return envfile.WriteAtomic(filepath.Join(stateDir, "ports.json"), data, 0o644)
}

// Load reads the last run's plan.
func Load(stateDir string) (Plan, bool) {
	var p Plan
	data, err := os.ReadFile(filepath.Join(stateDir, "ports.json"))
	if err != nil || json.Unmarshal(data, &p) != nil || p.Actual == nil {
		return Plan{}, false
	}
	return p, true
}

// Rewriter maps every port a .env may mention (catalog default or preferred) to the port of this run.
type Rewriter struct{ from map[int]int }

// NewRewriter builds the mapping of a plan. Replacement ports never equal a default, so the mapping
// is unambiguous even when a preferred port of one key equals the default of another.
func NewRewriter(plan Plan) Rewriter {
	r := Rewriter{from: map[int]int{}}
	defaults := catalog.DefaultPorts()
	for k, actual := range plan.Actual {
		if d := defaults[k]; d > 0 {
			r.from[d] = actual
		}
	}
	for k, pref := range plan.Preferred {
		if pref > 0 && pref != defaults[k] {
			r.from[pref] = plan.Actual[k]
		}
	}
	return r
}

var loopbackPort = regexp.MustCompile(`(localhost|127\.0\.0\.1|\[::1\]|0\.0\.0\.0):(\d{2,5})\b`)

// Value rewrites loopback host:port pairs inside a value.
func (r Rewriter) Value(v string) string {
	return loopbackPort.ReplaceAllStringFunc(v, func(s string) string {
		m := loopbackPort.FindStringSubmatch(s)
		p, _ := strconv.Atoi(m[2])
		if to, ok := r.from[p]; ok {
			return m[1] + ":" + strconv.Itoa(to)
		}
		return s
	})
}

// Port maps a bare port value (PORT=3001).
func (r Rewriter) Port(v string) string {
	p, err := strconv.Atoi(v)
	if err != nil {
		return v
	}
	if to, ok := r.from[p]; ok {
		return strconv.Itoa(to)
	}
	return v
}

// Env rewrites a whole env map in place and returns the ports it referenced (after mapping).
func (r Rewriter) Env(env map[string]string) map[int]bool {
	refs := map[int]bool{}
	for k, v := range env {
		nv := r.Value(v)
		if k == "PORT" {
			nv = r.Port(v)
		}
		env[k] = nv
		for _, m := range loopbackPort.FindAllStringSubmatch(nv, -1) {
			if p, err := strconv.Atoi(m[2]); err == nil {
				refs[p] = true
			}
		}
	}
	return refs
}
