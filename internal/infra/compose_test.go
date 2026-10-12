package infra

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/assets"
)

func TestRustFSBucketVariablesAreEscapedForCompose(t *testing.T) {
	compose := string(assets.MustRead("compose.yaml"))
	if strings.Contains(compose, `--bucket "$bucket"`) || strings.Count(compose, `--bucket "$$bucket"`) != 2 {
		t.Fatal("Compose must not interpolate the shell loop's bucket variable")
	}
	if !strings.Contains(compose, "for bucket in photos church-email-images documents drive virtual-assistant; do") {
		t.Fatal("missing local runtime buckets")
	}
}

// Compose config needs the CLI plugin only, not a Docker daemon. Exercise its real
// interpolation whenever available, including with a conflicting host variable.
func TestComposePreservesRustFSBucketLoop(t *testing.T) {
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("Docker CLI is not installed")
	}
	if err := exec.Command(docker, "compose", "version").Run(); err != nil {
		t.Skip("Compose plugin is not installed")
	}
	path := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(path, assets.MustRead("compose.yaml"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(docker, "compose", "-p", "ipalpha-compose-regression", "--profile", "init", "-f", path, "config", "--format", "json")
	cmd.Env = append(os.Environ(), "bucket=must-not-be-expanded", "MONGO_USERNAME=fixture", "MONGO_PASSWORD=fixture", "RABBITMQ_USERNAME=fixture", "RABBITMQ_PASSWORD=fixture", "RUSTFS_ACCESS_KEY=fixture", "RUSTFS_SECRET_KEY=fixture")
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Compose config failed: %v\n%s", err, raw)
	}
	var model struct {
		Services map[string]struct {
			Command []string `json:"command"`
		} `json:"services"`
	}
	if err := json.Unmarshal(raw, &model); err != nil {
		t.Fatalf("invalid Compose JSON: %v", err)
	}
	command := strings.Join(model.Services["rustfs-init"].Command, "\n")
	if strings.Contains(command, "must-not-be-expanded") || strings.Count(command, `--bucket "$$bucket"`) != 2 {
		t.Fatal("Compose consumed a shell variable before the container could expand it")
	}
}

// Every published host port must stay on loopback (Docker bypasses host firewalls).
func TestComposePublishesOnlyOnLoopback(t *testing.T) {
	for _, line := range strings.Split(string(assets.MustRead("compose.yaml")), "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, `- "`) && strings.Contains(l, "_HOST_PORT") && !strings.HasPrefix(l, `- "127.0.0.1:`) {
			t.Fatalf("host port not bound to loopback: %s", l)
		}
	}
}

func TestAppleBindsInfraToLoopback(t *testing.T) {
	src, err := os.ReadFile("apple.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(src), "\n") {
		if strings.Contains(l, `"--publish", fmt.Sprintf("%d:`) {
			t.Fatalf("apple publish not bound to loopback: %s", strings.TrimSpace(l))
		}
	}
}
