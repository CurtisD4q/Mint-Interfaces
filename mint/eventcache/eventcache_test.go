package eventcache

import (
	"strings"
	"testing"
	"time"
)

func TestStreamMarksSlicesDirty(t *testing.T) {
	fetches := map[string]int{}
	c := New(Options{
		Fetchers: map[string]func() any{
			"wifi":  func() any { fetches["wifi"]++; return fetches["wifi"] },
			"audio": func() any { fetches["audio"]++; return fetches["audio"] },
		},
		Stream: `sleep 0.2; echo wifi changed; sleep 0.2; echo something else`,
		Classify: func(line string) string {
			if strings.HasPrefix(line, "wifi") {
				return "wifi"
			}
			return ""
		},
	})
	c.Get("wifi")
	c.Get("audio")
	c.Get("wifi") // cached
	if fetches["wifi"] != 1 || fetches["audio"] != 1 {
		t.Fatalf("initial fetches %v", fetches)
	}
	c.Start()
	defer c.Stop()
	deadline := time.Now().Add(3 * time.Second)
	for !c.Poll() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	c.Get("wifi")
	c.Get("audio")
	if fetches["wifi"] != 2 || fetches["audio"] != 1 {
		t.Fatalf("after wifi event %v", fetches)
	}
	for !c.Poll() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	c.Get("wifi")
	c.Get("audio")
	if fetches["wifi"] != 3 || fetches["audio"] != 2 {
		t.Fatalf("after unclassified event %v", fetches)
	}
}
