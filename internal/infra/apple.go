package infra

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/ports"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/ui"
)

// appleUp runs the same four services with Apple's `container` CLI (no compose there).
func (e *Engine) appleUp(plan ports.Plan) error {
	c := credentials(e.W)
	n := e.Name
	_, _ = sys.Output("", "container", "network", "create", n)
	for _, v := range []string{"mongo-data", "redis-data", "rabbitmq-data", "rustfs-data", "face-models"} {
		_, _ = sys.Output("", "container", "volume", "create", n+"-"+v)
	}
	specs := []struct {
		name  string
		args  []string
		ready []string
	}{
		{"cpamc", []string{"--network", n, "--publish", fmt.Sprintf("127.0.0.1:%d:8317", plan.Port("cpamc")),
			"--cpus", "1", "--memory", "256M", catalog.CPAMCSimulatorImage},
			[]string{"/cpamc-simulator", "healthcheck"}},
		{"mongo", []string{"--network", n, "--publish", fmt.Sprintf("%d:27017", plan.Port("mongo")),
			"--env", "MONGO_INITDB_ROOT_USERNAME=" + c["MONGO_USERNAME"], "--env", "MONGO_INITDB_ROOT_PASSWORD=" + c["MONGO_PASSWORD"],
			"--volume", n + "-mongo-data:/data/db", "mongo:8"},
			[]string{"mongosh", "--quiet", "--username", c["MONGO_USERNAME"], "--password", c["MONGO_PASSWORD"], "--authenticationDatabase", "admin", "--eval", "db.adminCommand({ ping: 1 })"}},
		{"redis", []string{"--network", n, "--publish", fmt.Sprintf("%d:6379", plan.Port("redis")),
			"--volume", n + "-redis-data:/data", "redis:7-alpine", "redis-server", "--appendonly", "yes"},
			[]string{"redis-cli", "ping"}},
		{"rabbitmq", []string{"--network", n, "--publish", fmt.Sprintf("%d:5672", plan.Port("rabbitmq")),
			"--publish", fmt.Sprintf("%d:15672", plan.Port("rabbitmq-mgmt")),
			"--env", "RABBITMQ_DEFAULT_USER=" + c["RABBITMQ_USERNAME"], "--env", "RABBITMQ_DEFAULT_PASS=" + c["RABBITMQ_PASSWORD"],
			"--volume", n + "-rabbitmq-data:/var/lib/rabbitmq", "rabbitmq:4-management"},
			[]string{"rabbitmq-diagnostics", "-q", "ping"}},
		// Local capture only: HTTP Send API + inbox on loopback, no SMTP relay. Apple's VM needs ≥200 MiB.
		{"mailpit", []string{"--network", n, "--publish", fmt.Sprintf("127.0.0.1:%d:8025", plan.Port("mailpit")),
			"--cpus", "1", "--memory", "256M", "--env", "MP_MAX_MESSAGES=200", "--env", "MP_MAX_AGE=24h",
			"--env", "MP_MAX_MESSAGE_SIZE=1", "--env", "MP_DISABLE_VERSION_CHECK=true", "--env", "MP_SMTP_DISABLE_RDNS=true",
			"axllent/mailpit:v1.31.3"},
			[]string{"/mailpit", "readyz"}},
		{"rustfs", []string{"--network", n, "--publish", fmt.Sprintf("127.0.0.1:%d:9000", plan.Port("rustfs")),
			"--env", "RUSTFS_VOLUMES=/data", "--env", "RUSTFS_ADDRESS=:9000", "--env", "RUSTFS_CONSOLE_ENABLE=false",
			"--env", "RUSTFS_ACCESS_KEY=" + c["RUSTFS_ACCESS_KEY"], "--env", "RUSTFS_SECRET_KEY=" + c["RUSTFS_SECRET_KEY"],
			"--volume", n + "-rustfs-data:/data", "rustfs/rustfs:1.0.1", "/data"},
			[]string{"curl", "-fsS", "http://127.0.0.1:9000/health/ready"}},
	}
	if e.W.Settings.Face {
		specs = append(specs, struct {
			name  string
			args  []string
			ready []string
		}{"face", []string{"--network", n, "--platform", "linux/amd64", "--publish", fmt.Sprintf("127.0.0.1:%d:8000", plan.Port("face")),
			"--env", "FACE_MODEL=buffalo_l", "--env", "FACE_MODEL_ROOT=/app/data/insightface", "--env", "FACE_DET_SIZE=640", "--env", "FACE_MAX_BYTES=8388608",
			"--volume", n + "-face-models:/app/data/insightface", catalog.FaceImage},
			[]string{"python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8000/health', timeout=4)"}})
	}
	status := map[string]string{}
	for _, ac := range appleList() {
		status[ac.Configuration.ID] = ac.Status
	}
	_ = os.MkdirAll(e.W.StateDir(), 0o755)
	for _, s := range specs {
		name := n + "-" + s.name
		sum := sha256.Sum256([]byte(strings.Join(s.args, "\x00")))
		hash := hex.EncodeToString(sum[:])
		hashFile := filepath.Join(e.W.StateDir(), name+".spec")
		prev, _ := os.ReadFile(hashFile)
		st := status[name]
		if st != "" && strings.TrimSpace(string(prev)) != hash {
			_, _ = sys.Output("", "container", "stop", name)
			_, _ = sys.Output("", "container", "delete", "--force", name)
			st = ""
		}
		var out string
		var err error
		switch st {
		case "running":
		case "":
			out, err = sys.Output("", "container", append([]string{"run", "--detach", "--name", name}, s.args...)...)
			if err == nil {
				_ = os.WriteFile(hashFile, []byte(hash+"\n"), 0o644)
			}
		default:
			out, err = sys.Output("", "container", "start", name)
		}
		if err != nil {
			return &ui.Problem{Step: i18n.T("infra_step"), Cause: i18n.T("infra_up_failed") + " (" + s.name + ")",
				Tail: ui.Tail(out+"\n"+err.Error(), 10), Fix: []string{i18n.T("infra_fix_use_docker")}}
		}
	}
	for _, s := range specs {
		name := n + "-" + s.name
		ok := false
		for i := 0; i < 90; i++ {
			if _, err := sys.Output("", "container", append([]string{"exec", name}, s.ready...)...); err == nil {
				ok = true
				break
			}
			time.Sleep(time.Second)
		}
		if !ok {
			ui.Warning(i18n.T("infra_not_healthy_yet", s.name))
		}
	}
	// One-shot job: create the buckets (photos, church e-mail images, documents) when missing (idempotent).
	endpoint := "http://" + n + "-rustfs:9000"
	if out, err := sys.Output("", "container", "run", "--rm", "--network", n,
		"--env", "AWS_ACCESS_KEY_ID="+c["RUSTFS_ACCESS_KEY"], "--env", "AWS_SECRET_ACCESS_KEY="+c["RUSTFS_SECRET_KEY"], "--env", "AWS_DEFAULT_REGION=us-east-1",
		"--entrypoint", "sh", "amazon/aws-cli:2.37.9", "-c",
		"for bucket in photos church-email-images documents; do aws --endpoint-url "+endpoint+" s3api head-bucket --bucket \"$bucket\" || aws --endpoint-url "+endpoint+" s3api create-bucket --bucket \"$bucket\"; done"); err != nil {
		return &ui.Problem{Step: i18n.T("infra_step"), Cause: i18n.T("infra_bucket_failed"), Tail: ui.Tail(out+"\n"+err.Error(), 8), Fix: []string{i18n.T("infra_fix_use_docker")}}
	}
	return nil
}
