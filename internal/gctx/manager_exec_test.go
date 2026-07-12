package gctx

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerExecutesNativeArgumentsWithoutShellComposition(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	directory := t.TempDir()
	binDirectory := t.TempDir()
	logPath := filepath.Join(directory, "calls.log")
	t.Setenv("FAKE_GCLOUD_DIR", directory)
	t.Setenv("FAKE_GCLOUD_LOG", logPath)
	t.Setenv("PATH", binDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))

	script := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$FAKE_GCLOUD_LOG"
case "$1 $2 ${3-}" in
  "config configurations list")
    printf '%s\n' '[{"name":"example-old","is_active":true,"properties":{"core":{"account":"old@example.com","project":"old-project"},"billing":{"quota_project":"old-quota"}}},{"name":"example-dev","is_active":false,"properties":{"core":{"account":"user@example.com","project":"example-project"},"billing":{"quota_project":"example-quota"}}}]'
    ;;
  "info --format=value(config.paths.global_config_dir) ")
    printf '%s\n' "$FAKE_GCLOUD_DIR"
    ;;
  "auth login user@example.com")
    printf '%s' 'target-adc' > "$FAKE_GCLOUD_DIR/application_default_credentials.json"
    chmod 600 "$FAKE_GCLOUD_DIR/application_default_credentials.json"
    ;;
esac
`
	path := filepath.Join(binDirectory, "gcloud")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(nil, io.Discard, io.Discard)

	result, err := manager.Switch(t.Context(), "example-dev")

	if err != nil {
		t.Fatalf("Switch() error = %v", err)
	}
	if result.Name != "example-dev" {
		t.Fatalf("result = %#v", result)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	wantLines := []string{
		"config configurations list --format=" + configurationFormat,
		"info --format=value(config.paths.global_config_dir)",
		"auth login user@example.com --brief --no-activate --update-adc --verbosity=error --configuration=example-dev",
		"auth application-default set-quota-project example-quota --configuration=example-dev",
		"config configurations activate example-dev --quiet",
	}
	if strings.Split(strings.TrimSpace(string(log)), "\n")[0] != wantLines[0] {
		t.Fatalf("first native call = %q", strings.Split(strings.TrimSpace(string(log)), "\n")[0])
	}
	for _, line := range wantLines {
		if !strings.Contains(string(log), line+"\n") {
			t.Fatalf("native log = %q, missing %q", log, line)
		}
	}
}
