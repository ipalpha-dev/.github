// Package gateway configures the application's cpamc connection, not the coding CLI.
package gateway

import (
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

// NormalizeURL accepts cpamc's origin or /v1 base. Plain HTTP is loopback-only.
// Management lives on that same origin, so arbitrary URL prefixes are not supported.
func NormalizeURL(value string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || (u.Path != "" && u.Path != "/" && u.Path != "/v1" && u.Path != "/v1/") {
		return "", errors.New("gateway_url_invalid")
	}
	if u.Scheme == "http" {
		ip := net.ParseIP(u.Hostname())
		if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return "", errors.New("gateway_url_https_required")
		}
	}
	u.Path = "/v1"
	u.RawPath = ""
	return u.String(), nil
}

// Configure changes only the three explicitly selected gateway env values.
// Other values, including obsolete vendor keys, remain untouched.
func Configure(w *workspace.Workspace, mode, base, clientKey, managementKey string) error {
	if mode != "simulator" && mode != "real" {
		return errors.New("gateway_mode_invalid")
	}
	if mode == "simulator" {
		base = "http://127.0.0.1:8317/v1"
		clientKey = catalog.CPAMCSimulatorClientKey
		managementKey = catalog.CPAMCSimulatorManagementKey
	}
	base, err := NormalizeURL(base)
	if err != nil {
		return err
	}
	if strings.TrimSpace(clientKey) == "" || strings.ContainsAny(clientKey+managementKey, "\r\n\x00") || (mode == "real" && (clientKey == catalog.CPAMCSimulatorClientKey || managementKey == catalog.CPAMCSimulatorManagementKey)) {
		return errors.New("gateway_key_invalid")
	}
	if managementKey != "" && managementKey == clientKey {
		return errors.New("gateway_keys_must_differ")
	}
	file, err := envfile.Load(filepath.Join(w.Repo("ai-api"), ".env"))
	if err != nil {
		return err
	}
	for key, value := range map[string]string{"AI_BASE_URL": base, "AI_API_KEY": clientKey, "CPAMC_MANAGEMENT_KEY": managementKey} {
		file.Set(key, envfile.Quote(value))
	}
	if err := file.Save(); err != nil {
		return err
	}
	w.Settings.GatewayMode = mode
	return w.Save()
}
