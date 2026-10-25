package mint

import (
	"errors"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

const esc = "\x1b["

// Reset clears all colours and attributes.
const Reset = esc + "0m"

func move(r, c int) string {
	return esc + strconv.Itoa(r) + ";" + strconv.Itoa(c) + "H"
}

func out(s string) { _, _ = os.Stdout.WriteString(s) }

// ── raw mode ─────────────────────────────────────────────────────────────────

var (
	savedTermios *syscall.Termios
	screenActive bool
	sigCh        chan os.Signal
)

func tcget(fd int) (*syscall.Termios, error) {
	var t syscall.Termios
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), ioctlGetTermios, uintptr(unsafe.Pointer(&t))); e != 0 {
		return nil, e
	}
	return &t, nil
}

func tcset(fd int, t *syscall.Termios) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), ioctlSetTermios, uintptr(unsafe.Pointer(t))); e != 0 {
		return e
	}
	return nil
}

// ScreenEnter switches to the alternate screen in raw mode with the cursor
// hidden, SGR mouse reporting, and bracketed paste enabled according to the
// probed terminal capability runtime.
func ScreenEnter() {
	if screenActive {
		return
	}
	rt.profile = loadOrProbeTermProfile(false)
	rt.initSignals()
	C = rt.quantizePalette(loadPalette())
	if t, err := tcget(0); err == nil {
		saved := *t
		savedTermios = &saved
		raw := *t
		makeRaw(&raw)
		raw.Cc[syscall.VMIN] = 0
		raw.Cc[syscall.VTIME] = 1
		_ = tcset(0, &raw)
	}
	out(rt.enterSeq())
	screenActive = true

	// If we are killed, put the terminal back first.
	sigCh = make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT)
	go func(ch chan os.Signal) {
		if s, ok := <-ch; ok {
			ScreenLeave()
			signal.Reset(s)
			_ = syscall.Kill(os.Getpid(), s.(syscall.Signal))
		}
	}(sigCh)
}

// ScreenLeave restores the terminal exactly as ScreenEnter found it.
func ScreenLeave() {
	if !screenActive {
		return
	}
	out(rt.leaveSeq())
	if savedTermios != nil {
		_ = tcset(0, savedTermios)
	}
	if sigCh != nil {
		signal.Stop(sigCh)
		close(sigCh)
		sigCh = nil
	}
	screenActive = false
}

// TermSize returns the terminal's rows and columns (24x80 if unknown).
func TermSize() (int, int) {
	var ws struct{ Row, Col, X, Y uint16 }
	for _, fd := range []int{0, 1} {
		if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws))); e == 0 && ws.Row > 0 && ws.Col > 0 {
			return int(ws.Row), int(ws.Col)
		}
	}
	return 24, 80
}

// ── input ────────────────────────────────────────────────────────────────────

// Mouse is a decoded SGR mouse press.
type Mouse struct {
	Btn, Col, Row      int
	WheelUp, WheelDown bool
	Click              bool
}

// Key is one input event.
//
// Special keys have a Name and Special set: "up", "down", "left", "right",
// "home", "end", "pgup", "pgdn", "insert", "delete", "f1"…"f12", "btab"
// (shift-Tab), "esc", or "M-x" for Alt+x. Everything else is a literal
// character typed (one UTF-8 character), a bracketed paste (Paste != ""),
// or a control byte such as "\r", "\t", "\x7f" or "\x03". Mouse is set for
// mouse events instead.
type Key struct {
	Name    string
	Special bool
	paste   string
	Mouse   *Mouse
}

// IsText reports whether the key is a printable character that belongs in
// a text field.
func (k *Key) IsText() bool {
	return k != nil && !k.Special && k.Mouse == nil && len(k.Name) > 0 && k.Name[0] >= 32 && k.Name != "\x7f"
}

var (
	inBuf        [4096]byte
	inPos, inLen int
	// readInput reads raw terminal input; tests replace it.
	readInput = func(b []byte) (int, error) { return syscall.Read(0, b) }
)

// errHangup is raised when the terminal disappears.
var errHangup = errors.New("mint: terminal closed")

// readByte returns the next input byte, or false when the 100 ms read
// timeout expires with nothing typed.
func readByte() (byte, bool) {
	if inPos < inLen {
		b := inBuf[inPos]
		inPos++
		return b, true
	}
	n, err := readInput(inBuf[:])
	if err != nil && !errors.Is(err, syscall.EINTR) && !errors.Is(err, syscall.EAGAIN) {
		panic(errHangup)
	}
	if n <= 0 {
		inPos, inLen = 0, 0
		return 0, false
	}
	inPos, inLen = 1, n
	return inBuf[0], true
}

func readUTF8(first byte) string {
	need := 0
	switch {
	case first >= 0xC2 && first <= 0xDF:
		need = 1
	case first >= 0xE0 && first <= 0xEF:
		need = 2
	case first >= 0xF0 && first <= 0xF4:
		need = 3
	default:
		return string([]byte{first})
	}
	s := []byte{first}
	for i := 0; i < need; i++ {
		b, ok := readByte()
		if !ok {
			break
		}
		s = append(s, b)
	}
	return string(s)
}

func named(n string) *Key  { return &Key{Name: n, Special: true} }
func char(s string) *Key   { return &Key{Name: s} }
func pasted(s string) *Key { return &Key{Name: s, paste: s} }

// tildeKeys maps the number in "ESC [ n ~" to a key.
var tildeKeys = map[string]string{
	"1": "home", "7": "home", "4": "end", "8": "end", "2": "insert", "3": "delete",
	"5": "pgup", "6": "pgdn",
	"11": "f1", "12": "f2", "13": "f3", "14": "f4", "15": "f5", "17": "f6",
	"18": "f7", "19": "f8", "20": "f9", "21": "f10", "23": "f11", "24": "f12",
}

// finalKeys maps the final byte of "ESC [ … X" or "ESC O X" to a key.
var finalKeys = map[byte]string{
	'A': "up", 'B': "down", 'C': "right", 'D': "left", 'H': "home", 'F': "end",
	'P': "f1", 'Q': "f2", 'R': "f3", 'S': "f4", 'Z': "btab",
}

func readBracketedPaste() *Key {
	var buf []byte
	const endMarker = "\x1b[201~"
	for len(buf) < 65536 {
		b, ok := readByte()
		if !ok {
			break
		}
		buf = append(buf, b)
		if len(buf) >= len(endMarker) && string(buf[len(buf)-len(endMarker):]) == endMarker {
			buf = buf[:len(buf)-len(endMarker)]
			break
		}
	}
	return pasted(string(buf))
}

func decodeKittyCSIu(params string) *Key {
	codeStr, modStr, _ := strings.Cut(params, ";")
	code, err := strconv.Atoi(codeStr)
	if err != nil || code <= 0 {
		return nil
	}
	mod := 1
	if modStr != "" {
		if m, err := strconv.Atoi(modStr); err == nil && m > 0 {
			mod = m
		}
	}
	switch code {
	case 27:
		return named("esc")
	case 13:
		return char("\r")
	case 9:
		if (mod-1)&1 != 0 {
			return named("btab")
		}
		return char("\t")
	case 127, 8:
		return char("\x7f")
	}
	if (mod-1)&4 != 0 && code >= 'a' && code <= 'z' {
		return char(string(rune(code - 'a' + 1)))
	}
	if (mod-1)&2 != 0 && code >= 32 && code < 127 {
		return named("M-" + string(rune(code)))
	}
	if code >= 32 && utf8.ValidRune(rune(code)) {
		return char(string(rune(code)))
	}
	return nil
}

// readCSI reads the rest of an "ESC [" sequence: parameter bytes, then one
// final byte. Unknown or cut-off sequences return nil, so they are ignored
// instead of being mistaken for Esc.
func readCSI() *Key {
	var params []byte
	for {
		b, ok := readByte()
		if !ok {
			if len(params) == 0 {
				return named("esc")
			}
			return nil
		}
		if b >= 0x40 && b <= 0x7e { // final byte
			if len(params) > 0 && params[0] == '<' && (b == 'M' || b == 'm') {
				return mouseKey(string(params[1:]), b == 'M')
			}
			if b == 'I' {
				rt.focused.Store(true)
				return nil
			}
			if b == 'O' {
				rt.focused.Store(false)
				return nil
			}
			if b == 'u' {
				return decodeKittyCSIu(string(params))
			}
			if b == '~' {
				num, _, _ := strings.Cut(string(params), ";")
				if num == "200" {
					return readBracketedPaste()
				}
				if n, ok := tildeKeys[num]; ok {
					return named(n)
				}
				return nil
			}
			if n, ok := finalKeys[b]; ok {
				return named(n)
			}
			return nil
		}
		params = append(params, b)
		if len(params) > 32 {
			return nil
		}
	}
}

func mouseKey(params string, press bool) *Key {
	parts := strings.Split(params, ";")
	if len(parts) != 3 || !press {
		return nil // releases and malformed reports are idle ticks
	}
	n, e1 := strconv.Atoi(parts[0])
	col, e2 := strconv.Atoi(parts[1])
	row, e3 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return nil
	}
	return &Key{Mouse: &Mouse{Btn: n, Col: col, Row: row,
		WheelUp: n == 64, WheelDown: n == 65, Click: n == 0}}
}

// ReadKey waits up to about 100 ms for one key or mouse event. It returns
// nil when nothing arrived, for mouse releases, and for keys Mint does not
// know; callers treat nil as an idle tick.
func ReadKey() *Key {
	c, ok := readByte()
	if !ok {
		return nil
	}
	if c == 0x1b {
		a, ok := readByte()
		if !ok {
			return named("esc")
		}
		switch {
		case a == '[':
			return readCSI()
		case a == 'O':
			b, ok := readByte()
			if !ok {
				return named("esc")
			}
			if n, ok := finalKeys[b]; ok {
				return named(n)
			}
			return nil
		case a == 0x1b:
			return named("esc")
		case a >= 0xC2:
			return named("M-" + readUTF8(a))
		case a >= 32 && a < 0x7f:
			return named("M-" + string([]byte{a}))
		}
		return named("esc")
	}
	if c >= 0xC2 {
		return char(readUTF8(c))
	}
	return char(string([]byte{c}))
}

// isTerminal reports whether standard input is a terminal.
func isTerminal() bool {
	_, err := tcget(0)
	return err == nil
}
