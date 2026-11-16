package watch

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/master2sid2/mycnfm/internal/mycnf"
)

func TestParseTimeout(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"", DefaultTimeout, false},
		{"30m", 30 * time.Minute, false},
		{"1h30m", 90 * time.Minute, false},
		{"0", 0, false},
		{"off", 0, false},
		{"OFF", 0, false},
		{"-5m", 0, true},
		{"soon", 0, true},
	}
	for _, c := range cases {
		got, err := ParseTimeout(c.in)
		if (err != nil) != c.wantErr || (!c.wantErr && got != c.want) {
			t.Errorf("ParseTimeout(%q) = %v, %v; want %v, err=%v", c.in, got, err, c.want, c.wantErr)
		}
	}
}

func withFastPoll(t *testing.T) {
	t.Helper()
	old := pollEvery
	pollEvery = 10 * time.Millisecond
	t.Cleanup(func() { pollEvery = old })
}

func writeFile(t *testing.T, content string) (path, hash string) {
	t.Helper()
	path = filepath.Join(t.TempDir(), ".my.cnf")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, mycnf.Hash(content)
}

func TestRunWipesAfterDeadlineWhenContentUnchanged(t *testing.T) {
	withFastPoll(t)
	path, hash := writeFile(t, "[client]\npassword = \"s3cret\"\n")

	if err := Run(context.Background(), path, hash, time.Now().Add(60*time.Millisecond)); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "s3cret") {
		t.Fatalf("password should be gone after the deadline, got:\n%s", data)
	}
}

func TestRunLeavesAReplacedFileAlone(t *testing.T) {
	withFastPoll(t)
	path, hash := writeFile(t, "original\n")
	// The user switched profiles before the deadline: different content now.
	if err := os.WriteFile(path, []byte("a different profile\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Run(context.Background(), path, hash, time.Now().Add(40*time.Millisecond)); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "a different profile\n" {
		t.Fatalf("a replaced file must never be wiped, got %q", data)
	}
}

func TestRunExitsWhenFileIsRemoved(t *testing.T) {
	withFastPoll(t)
	path, hash := writeFile(t, "x\n")
	_ = os.Remove(path)
	if err := Run(context.Background(), path, hash, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("run must return promptly (not wait an hour) and without error: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("Run must not recreate a removed file")
	}
}

func TestDisarmNeverSignalsAnUnrelatedProcess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pidFile, err := pidFilePath()
	if err != nil {
		t.Fatal(err)
	}
	// A recycled PID: this very test process, which is certainly not a watcher.
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Disarm(); err != nil {
		t.Fatalf("disarm: %v", err)
	}
	if _, err := os.Stat(pidFile); err == nil {
		t.Fatal("Disarm should remove the pid file")
	}
	// Still alive to assert this — the point of the test.
}

func TestDisarmWithNoPidFileIsFine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Disarm(); err != nil {
		t.Fatalf("disarm with nothing armed: %v", err)
	}
}

func TestArmWithZeroDurationOnlyDisarms(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dl, err := Arm("/nonexistent", "h", 0)
	if err != nil || !dl.IsZero() {
		t.Fatalf("Arm(0) should be a no-op returning a zero deadline, got %v, %v", dl, err)
	}
}

func TestRunStopsPromptlyWhenCancelled(t *testing.T) {
	path, hash := writeFile(t, "x\n")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, path, hash, time.Now().Add(time.Hour)) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run ignored cancellation (this is how SIGTERM stops a watcher)")
	}
	if data, _ := os.ReadFile(path); string(data) != "x\n" {
		t.Fatalf("a cancelled watcher must not wipe, got %q", data)
	}
}
