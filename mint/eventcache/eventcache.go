// Package eventcache polls a background stream without blocking the UI.
//
// Derived from Terminal Settings Manager (Curtis, Apache-2.0):
// https://github.com/CurtisD4q/Terminal-Settings-Manager
//
// A caller supplies fetchers for named slices and, optionally, a shell
// command that writes one event line at a time. Poll checks for new event
// lines and marks slices dirty. Get re-fetches only dirty slices. The
// stream runs through mint/runner, so nothing is written to disk and Stop
// ends the command and its children.
package eventcache

import (
	"github.com/CurtisD4q/Mint-Interfaces/mint/runner"
)

// Options configure a Cache.
type Options struct {
	// Fetchers produce the value of each named slice.
	Fetchers map[string]func() any
	// Stream is an optional program, with its arguments, that prints one
	// event per line: {"nmcli", "monitor"}. It runs directly, never
	// through a shell.
	Stream []string
	// Classify maps an event line to the slice it affects. Returning "" (or
	// leaving Classify nil) marks every slice dirty.
	Classify func(line string) string
}

// Cache holds the last fetched value of each slice.
type Cache struct {
	fetchers  map[string]func() any
	values    map[string]any
	dirty     map[string]bool
	streamCmd []string
	classify  func(string) string
	started   bool

	job  *runner.Job
	seen int
}

// New creates a cache. Every slice starts dirty.
func New(o Options) *Cache {
	c := &Cache{
		fetchers:  o.Fetchers,
		values:    map[string]any{},
		dirty:     map[string]bool{},
		streamCmd: append([]string(nil), o.Stream...),
		classify:  o.Classify,
	}
	if c.fetchers == nil {
		c.fetchers = map[string]func() any{}
	}
	for name := range c.fetchers {
		c.dirty[name] = true
	}
	return c
}

// Start launches the stream command, if any. It is safe to call twice.
func (c *Cache) Start() bool {
	if c.started {
		return true
	}
	c.started = true
	if len(c.streamCmd) > 0 {
		c.job = runner.Start(runner.Cmd{Name: c.streamCmd[0], Args: c.streamCmd[1:], Label: "events"})
		c.seen = 0
	}
	return true
}

// Stop ends the stream command and everything it started.
func (c *Cache) Stop() {
	if !c.started {
		return
	}
	c.started = false
	if c.job != nil {
		c.job.Stop()
		c.job = nil
	}
}

// Poll reads any new event lines and reports whether there were some.
func (c *Cache) Poll() bool {
	if c.job == nil {
		return false
	}
	total := c.job.Total()
	n := total - c.seen
	if n <= 0 {
		return false
	}
	c.seen = total
	any := false
	for _, line := range c.job.Tail(n) {
		if line.Err || line.Text == "" {
			continue
		}
		any = true
		slice := ""
		if c.classify != nil {
			slice = c.classify(line.Text)
		}
		if slice != "" {
			c.dirty[slice] = true
		} else {
			c.MarkAllDirty()
		}
	}
	return any
}

// Get returns a slice's value, fetching it again only when dirty.
func (c *Cache) Get(slice string) any {
	if v, ok := c.values[slice]; c.dirty[slice] || !ok || v == nil {
		if fn := c.fetchers[slice]; fn != nil {
			c.values[slice] = safe(fn)
		}
		c.dirty[slice] = false
	}
	return c.values[slice]
}

func safe(fn func() any) (v any) {
	defer func() {
		if recover() != nil {
			v = nil
		}
	}()
	return fn()
}

// MarkDirty forces a slice to be fetched again. "" marks every slice.
func (c *Cache) MarkDirty(slice string) {
	if slice != "" {
		c.dirty[slice] = true
		return
	}
	c.MarkAllDirty()
}

// MarkAllDirty forces every slice to be fetched again.
func (c *Cache) MarkAllDirty() {
	for name := range c.fetchers {
		c.dirty[name] = true
	}
}
