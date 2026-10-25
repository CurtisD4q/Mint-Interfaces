package mint

import (
	"errors"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
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
// hidden and SGR mouse reporting on. Equivalent to
// `stty raw -echo min 0 time 1`: reads return after 100 ms with no input,
// which is what drives the idle tick (poll, resize).
func ScreenEnter() {
	if screenActive {
		return
	}
	if t, err := tcget(0); err == nil {
		saved := *t
		savedTermios = &saved
		raw := *t
		makeRaw(&raw)
		raw.Cc[syscall.VMIN] = 0
		raw.Cc[syscall.VTIME] = 1
		_ = tcset(0, &raw)
	}
	out(esc + "?1049h")
	out(esc + "?25l")
	out(esc + "?1000h")
	out(esc + "?1006h")
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
	out(esc + "?1000l")
	out(esc + "?1006l")
	out(esc + "?25h")
	out(esc + "?1049l")
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
// character typed (one UTF-8 character) or a control byte such as "\r",
// "\t", "\x7f" or "\x03". Mouse is set for mouse events instead.
type Key struct {
	Name    string
	Special bool
	Mouse   *Mouse
}

// IsText reports whether the key is a printable character that belongs in
// a text field.
func (k *Key) IsText() bool {
	return k != nil && !k.Special && k.Mouse == nil && len(k.Name) > 0 && k.Name[0] >= 32 && k.Name != "\x7f"
}

var (
	inBuf        [512]byte
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
		// The terminal has gone (closed window, killed Emacs). Unwind to
		// App.Run, which returns so the app can clean up and exit.
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

func named(n string) *Key { return &Key{Name: n, Special: true} }
func char(s string) *Key  { return &Key{Name: s} }

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
			if b == '~' {
				num, _, _ := strings.Cut(string(params), ";")
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
