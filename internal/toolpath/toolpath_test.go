package toolpath

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolve(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exec bits")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir()) // empty PATH: only Dirs() can find it
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(bin, "aether-fake-ffprobe")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Resolve("aether-fake-ffprobe"); got != tool {
		t.Fatalf("Resolve = %q, want %q", got, tool)
	}
	if got := Resolve("aether-missing-tool"); got != "aether-missing-tool" {
		t.Fatalf("missing tool: got %q", got)
	}
	if got := Resolve("/opt/x/ffmpeg"); got != "/opt/x/ffmpeg" {
		t.Fatalf("explicit path changed: %q", got)
	}
}
