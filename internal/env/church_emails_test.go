package env

import (
	"encoding/base64"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"os"
	"path/filepath"
	"testing"
)

func TestChurchEmailsEnvIsAddedAndPrivateValuesAreKept(t *testing.T) {
	w := ws(t)
	dir := w.Repo("notifications-api")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("MAIL_FROM=custom@example.test\nCHURCH_EMAIL_PUBLIC_URL=https://custom.example.test/api/notifications/church-emails\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.Dir(), ".env"), []byte("RUSTFS_ACCESS_KEY=custom-access\nRUSTFS_SECRET_KEY=custom-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(w); err != nil {
		t.Fatal(err)
	}
	first := envfile.Read(filepath.Join(dir, ".env"))
	key, err := base64.StdEncoding.DecodeString(first["CHURCH_EMAIL_UNSUBSCRIBE_KEY"])
	if err != nil || len(key) != 32 {
		t.Fatal("unsubscribe key must encode 32 random bytes")
	}
	if first["S3_ACCESS_KEY"] != "custom-access" || first["S3_SECRET_KEY"] != "custom-secret" || first["CHURCH_EMAIL_IMAGES_BUCKET"] != "church-email-images" || first["EXCHANGE_CHURCH_EMAIL_CHANGED"] != "notifications-api.churchEmail.changed" {
		t.Fatal("campaign config is incomplete")
	}
	if first["CHURCH_EMAIL_PUBLIC_URL"] != "https://custom.example.test/api/notifications/church-emails" || first["MAIL_FROM"] != "custom@example.test" {
		t.Fatal("local values were overwritten")
	}
	if _, err := Prepare(w); err != nil {
		t.Fatal(err)
	}
	second := envfile.Read(filepath.Join(dir, ".env"))
	if second["CHURCH_EMAIL_UNSUBSCRIBE_KEY"] != first["CHURCH_EMAIL_UNSUBSCRIBE_KEY"] || second["S3_SECRET_KEY"] != first["S3_SECRET_KEY"] {
		t.Fatal("re-run changed private local values")
	}
}
