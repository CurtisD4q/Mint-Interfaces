package appkit

import "testing"

// A full-screen program is never polled, whatever its archetype says.
func TestFullScreenIsNeverPolled(t *testing.T) {
	app := &ToolApp{Spec: ToolSpec{Name: "x", ArchetypeID: "watch", ReadOnly: true, FullScreen: true}}
	if app.executionMode() != execOnce {
		t.Errorf("mode %v, want run once", app.executionMode())
	}
	app.Spec.FullScreen = false
	if app.executionMode() != execPoll {
		t.Errorf("without full-screen, watch polls: mode %v", app.executionMode())
	}
}
