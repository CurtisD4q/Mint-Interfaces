package mint

import "testing"

// Ctrl+[ is Esc, in the CSI-u form too: vterm sends ESC [ 91 ; 7 u
// (Ctrl+Alt+[) when Esc is pressed twice. Plain [ and Alt+[ are not Esc.
func TestCSIuCtrlBracketIsEsc(t *testing.T) {
	isEsc := func(k *Key) bool { return k != nil && k.Name == "esc" }
	for _, p := range []string{"91;5", "91;7", "27", "27;5", "27;7"} {
		if k := decodeKittyCSIu(p); !isEsc(k) {
			t.Errorf("ESC [ %s u = %+v, want esc", p, k)
		}
	}
	for _, p := range []string{"91", "91;1", "91;3"} {
		if k := decodeKittyCSIu(p); isEsc(k) {
			t.Errorf("ESC [ %s u became esc", p)
		}
	}
}
