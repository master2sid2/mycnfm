// Package watch implements the auto-wipe timer: after a profile is applied, a
// detached copy of mycnfm sleeps until a deadline and then erases ~/.my.cnf —
// but only if the file still holds exactly what was written, so a profile the
// user has since switched to (or cleaned) is never touched.
//
// The watcher needs no vault access and no password: its argv is just the
// file path, the content hash to guard and the deadline. Only one watcher
// exists at a time; arming a new one (or cleaning) stops the previous one.
package watch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/master2sid2/mycnfm/internal/config"
	"github.com/master2sid2/mycnfm/internal/mycnf"
)

const (
	// DefaultTimeout is how long an applied profile stays in ~/.my.cnf when
	// no -t is given.
	DefaultTimeout = time.Hour

	// Command is the hidden subcommand a watcher process is started with.
	Command = "__wipe-after"
)

// pollEvery bounds how long the watcher sleeps between checks. It re-reads
// the wall clock on every wake, so a machine that slept past the deadline
// wipes within one interval of resuming (a plain Sleep would not count the
// suspended time).
var pollEvery = 30 * time.Second

// ParseTimeout parses a -t value. Empty means DefaultTimeout; "0" or "off"
// disables the timer; anything else is a Go duration such as "15m" or "1h30m".
func ParseTimeout(s string) (time.Duration, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return DefaultTimeout, nil
	case "off":
		return 0, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("invalid timeout %q (use e.g. 30m, 1h30m, 0 or off): %w", s, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("timeout must not be negative: %q", s)
	}
	return d, nil
}

// pidFilePath is where the running watcher's PID is recorded: next to the
// vault, since there is at most one watcher per user (one ~/.my.cnf).
func pidFilePath() (string, error) {
	vault, err := config.DefaultPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(vault), "watcher.pid"), nil
}

// Arm stops any previous watcher and, if d > 0, starts a new detached one
// guarding path/hash. It returns the deadline (zero when d <= 0). If the
// watcher can't be started the deadline is still returned alongside the
// error, so the caller can record it as the launch-time backstop.
func Arm(path, hash string, d time.Duration) (time.Time, error) {
	if err := Disarm(); err != nil {
		return time.Time{}, err
	}
	if d <= 0 {
		return time.Time{}, nil
	}
	deadline := time.Now().Add(d).Truncate(time.Second)

	exe, err := os.Executable()
	if err != nil {
		return deadline, fmt.Errorf("locate executable: %w", err)
	}
	pid, err := spawnDetached(exe, []string{Command, path, hash, strconv.FormatInt(deadline.Unix(), 10)})
	if err != nil {
		return deadline, fmt.Errorf("start wipe timer: %w", err)
	}
	pidFile, err := pidFilePath()
	if err != nil {
		return deadline, err
	}
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(pid)+"\n"), 0o600); err != nil {
		return deadline, fmt.Errorf("record watcher pid: %w", err)
	}
	return deadline, nil
}

// Disarm stops the running watcher, if any. It only signals a process that
// really is one of ours (a recycled PID belonging to something else is left
// alone).
func Disarm() error {
	pidFile, err := pidFilePath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(pidFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if pid, convErr := strconv.Atoi(strings.TrimSpace(string(data))); convErr == nil && pid > 0 && isWatcher(pid) {
		_ = terminate(pid)
	}
	if err := os.Remove(pidFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Run is the watcher's body: wait for the deadline, then wipe path — but only
// while it still contains the content with the given hash. If the file was
// replaced, cleaned or removed in the meantime, it exits without touching it.
//
// ctx is cancelled on SIGTERM/SIGINT (see main), which is how Disarm stops a
// watcher; without honouring it the signal would be swallowed and the old
// watcher would linger until its deadline.
func Run(ctx context.Context, path, hash string, deadline time.Time) error {
	for {
		if !stillGuarded(path, hash) {
			return nil
		}
		wait := time.Until(deadline)
		if wait <= 0 {
			break
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(min(wait, pollEvery)):
		}
	}
	if !stillGuarded(path, hash) { // re-check right before acting
		return nil
	}
	return mycnf.Clean(path)
}

// stillGuarded reports whether path still holds the content we're watching.
// A transient read error counts as "yes" so a hiccup never cancels the wipe.
func stillGuarded(path, hash string) bool {
	cur, ok, err := mycnf.CurrentHash(path)
	if err != nil {
		return true
	}
	return ok && cur == hash
}

func itoa(n int) string { return strconv.Itoa(n) }
