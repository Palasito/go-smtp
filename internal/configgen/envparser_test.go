package configgen

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestEnv(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseEnvFile(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantLen   int
		check     func(t *testing.T, entries []EnvEntry)
		wantErr   bool
		usePath   string // if set, use this path instead of writing content
	}{
		{
			name:    "basic parsing",
			content: "FOO=bar\nBAZ=qux\n",
			wantLen: 2,
			check: func(t *testing.T, entries []EnvEntry) {
				if entries[0].Key != "FOO" || entries[0].Value != "bar" {
					t.Errorf("entry 0: got %q=%q, want FOO=bar", entries[0].Key, entries[0].Value)
				}
				if entries[1].Key != "BAZ" || entries[1].Value != "qux" {
					t.Errorf("entry 1: got %q=%q, want BAZ=qux", entries[1].Key, entries[1].Value)
				}
			},
		},
		{
			name:    "comments and blanks",
			content: "# This is a comment\n\nKEY=val\n\n# Another comment\n",
			wantLen: 1,
			check: func(t *testing.T, entries []EnvEntry) {
				if entries[0].Key != "KEY" || entries[0].Value != "val" {
					t.Errorf("got %q=%q, want KEY=val", entries[0].Key, entries[0].Value)
				}
			},
		},
		{
			name:    "quoted values",
			content: "A=\"hello world\"\nB='single quoted'\n",
			wantLen: 2,
			check: func(t *testing.T, entries []EnvEntry) {
				if entries[0].Value != "hello world" {
					t.Errorf("double-quoted: got %q, want %q", entries[0].Value, "hello world")
				}
				if entries[1].Value != "single quoted" {
					t.Errorf("single-quoted: got %q, want %q", entries[1].Value, "single quoted")
				}
			},
		},
		{
			name:    "equals in value",
			content: "KEY=a=b=c\n",
			wantLen: 1,
			check: func(t *testing.T, entries []EnvEntry) {
				if entries[0].Key != "KEY" || entries[0].Value != "a=b=c" {
					t.Errorf("got %q=%q, want KEY=a=b=c", entries[0].Key, entries[0].Value)
				}
			},
		},
		{
			name:    "whitespace trimming",
			content: "  KEY = value  \n",
			wantLen: 1,
			check: func(t *testing.T, entries []EnvEntry) {
				if entries[0].Key != "KEY" || entries[0].Value != "value" {
					t.Errorf("got %q=%q, want KEY=value", entries[0].Key, entries[0].Value)
				}
			},
		},
		{
			name:    "line numbers",
			content: "# comment\n\nFIRST=1\n\nSECOND=2\n",
			wantLen: 2,
			check: func(t *testing.T, entries []EnvEntry) {
				if entries[0].Line != 3 {
					t.Errorf("first entry line: got %d, want 3", entries[0].Line)
				}
				if entries[1].Line != 5 {
					t.Errorf("second entry line: got %d, want 5", entries[1].Line)
				}
			},
		},
		{
			name:    "file not found",
			usePath: "/nonexistent/path/.env",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.usePath
			if path == "" {
				path = writeTestEnv(t, tt.content)
			}

			entries, err := ParseEnvFile(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entries) != tt.wantLen {
				t.Fatalf("got %d entries, want %d", len(entries), tt.wantLen)
			}
			if tt.check != nil {
				tt.check(t, entries)
			}
		})
	}
}
