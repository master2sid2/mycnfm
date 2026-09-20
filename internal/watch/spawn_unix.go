//go:build unix

package watch

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// spawnDetached starts exe with args in a new session (setsid) with no
// stdio, so it survives the terminal, the shell and the parent exiting.
func spawnDetached(exe string, args []string) (int, error) {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}

func terminate(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGTERM)
}

// isWatcher reports whether pid is one of our watcher processes, by its
// command line — /proc where available, otherwise ps (macOS).
func isWatcher(pid int) bool {
	if data, err := os.ReadFile("/proc/" + itoa(pid) + "/cmdline"); err == nil {
		return strings.Contains(string(data), Command)
	}
	out, err := exec.Command("ps", "-p", itoa(pid), "-o", "args=").Output()
	return err == nil && strings.Contains(string(out), Command)
}
