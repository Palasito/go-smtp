package configgen

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Palasito/go-smtp/internal/config"
)

func TestGenerateEnvTemplate(t *testing.T) {
	t.Run("contains all groups", func(t *testing.T) {
		var buf bytes.Buffer
		if err := GenerateEnvTemplate(&buf); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output := buf.String()
		for _, group := range config.Groups() {
			if !strings.Contains(output, group) {
				t.Errorf("output missing group %q", group)
			}
		}
	})

	t.Run("contains all env vars", func(t *testing.T) {
		var buf bytes.Buffer
		if err := GenerateEnvTemplate(&buf); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output := buf.String()
		for _, f := range config.Schema {
			if !strings.Contains(output, f.EnvVar) {
				t.Errorf("output missing env var %q", f.EnvVar)
			}
		}
	})

	t.Run("sensitive fields show CHANGE_ME", func(t *testing.T) {
		var buf bytes.Buffer
		if err := GenerateEnvTemplate(&buf); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output := buf.String()
		// WHITELIST_CLIENT_SECRET is sensitive
		if !strings.Contains(output, "WHITELIST_CLIENT_SECRET=CHANGE_ME") {
			t.Error("expected WHITELIST_CLIENT_SECRET=CHANGE_ME in output")
		}
	})
}

func TestGenerateComposeTemplate(t *testing.T) {
	t.Run("valid YAML structure", func(t *testing.T) {
		var buf bytes.Buffer
		if err := GenerateComposeTemplate(&buf); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output := buf.String()
		for _, want := range []string{"services:", "smtp-relay:", "image:", "environment:"} {
			if !strings.Contains(output, want) {
				t.Errorf("output missing %q", want)
			}
		}
	})

	t.Run("contains all groups", func(t *testing.T) {
		var buf bytes.Buffer
		if err := GenerateComposeTemplate(&buf); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output := buf.String()
		for _, group := range config.Groups() {
			if !strings.Contains(output, group) {
				t.Errorf("output missing group %q", group)
			}
		}
	})
}
