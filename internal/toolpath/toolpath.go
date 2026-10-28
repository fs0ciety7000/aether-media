// Package toolpath finds external tools (ffmpeg, ffprobe) that are installed
// but missing from PATH. Services started by launchd or cron get a minimal
// PATH (/usr/bin:/bin...) that leaves out Homebrew and ~/bin.
package toolpath

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Dirs are searched, in order, when a bare name is not found in PATH.
func Dirs() []string {
	var d []string
	switch runtime.GOOS {
	case "darwin":
		d = append(d, "/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin")
	case "windows":
		d = append(d, `C:\ffmpeg\bin`, filepath.Join(os.Getenv("ProgramFiles"), "ffmpeg", "bin"))
	default:
		d = append(d, "/usr/lib/jellyfin-ffmpeg", "/usr/local/bin", "/home/linuxbrew/.linuxbrew/bin", "/snap/bin")
	}
	if home, err := os.UserHomeDir(); err == nil {
		d = append(d, filepath.Join(home, "bin"), filepath.Join(home, ".local", "bin"))
	}
	return d
}

// Resolve returns name unchanged when it is a path or is found in PATH,
// otherwise the first executable with that name in Dirs. When nothing is
// found, name is returned so the caller reports the usual "not found" error.
func Resolve(name string) string {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return name
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	exe := name
	if runtime.GOOS == "windows" && filepath.Ext(exe) == "" {
		exe += ".exe"
	}
	for _, dir := range Dirs() {
		p := filepath.Join(dir, exe)
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() && (runtime.GOOS == "windows" || fi.Mode()&0o111 != 0) {
			return p
		}
	}
	return name
}
