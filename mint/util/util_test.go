package util

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunTimeoutFinishes(t *testing.T) {
	out, err := RunTimeout(5*time.Second, "sh", "-c", "echo hi; exit 3")
	if strings.TrimSpace(out) != "hi" || err == nil || err.Error() != "exit 3" {
		t.Fatalf("out %q err %v", out, err)
	}
}

func TestRunTimeoutKillsTheWholeGroup(t *testing.T) {
	start := time.Now()
	out, err := RunTimeout(300*time.Millisecond, "sh", "-c", "sleep 30 & echo $!; tail -f /dev/null")
	if !errors.Is(err, ErrTimeout) || time.Since(start) > 3*time.Second {
		t.Fatalf("err %v after %v", err, time.Since(start))
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(out))
	time.Sleep(100 * time.Millisecond)
	if pid > 0 && syscall.Kill(pid, 0) == nil {
		if b, _ := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); !strings.Contains(string(b), ") Z ") {
			t.Fatalf("background child %d survived the timeout", pid)
		}
	}
}

func TestRunTimeoutNeverReadsTheKeyboard(t *testing.T) {
	out, err := RunTimeout(2*time.Second, "sh", "-c", "read x; echo got:$x")
	if err != nil || strings.TrimSpace(out) != "got:" {
		t.Fatalf("out %q err %v", out, err)
	}
}

func TestStopAllEndsRunningCommands(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		_, err := RunTimeout(time.Minute, "sleep", "30")
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	StopAll()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("StopAll did not end the command")
	}
}
