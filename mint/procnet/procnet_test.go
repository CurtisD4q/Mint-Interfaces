package procnet

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Captured from Linux /proc/net/tcp.
const tcpSample = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:494F 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 26029 1 00000000d8f42d70 100 0 0 10 0
   1: 00000000:07E8 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 744 1 0000000010ec82fa 100 0 0 10 0
   2: 020200C0:D4B2 0A684FA0:01BB 01 00000000:00000000 02:00000008 00000000     0        0 2553 2 00000000c7659279 20 4 32 549 -1
   3: 0100007F:EE0A 0100007F:92B7 06 00000000:00000000 03:000005CA 00000000     0        0 0 3 0000000009607ca5
`

// IPv6 table lines in the kernel's format.
const tcp6Sample = `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000001000000:1F90 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 41001 1 0000000000000000 100 0 0 10 0
   1: 00000000000000000000000000000000:0050 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 41002 1 0000000000000000 100 0 0 10 0
   2: 0000000000000000FFFF00000100007F:1F91 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 41003 1 0000000000000000 100 0 0 10 0
   3: B80D0120000000000000000001000000:0016 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 41004 1 0000000000000000 100 0 0 10 0
`

const udpSample = `   sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops
  100: 00000000:0044 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 12345 2 0000000000000000 0
`

func TestParseNetFile(t *testing.T) {
	socks, err := ParseNetFile([]byte(tcpSample), "tcp")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"127.0.0.1:18767 listen 26029", "0.0.0.0:2024 listen 744", "192.0.2.2:54450 established 2553", "127.0.0.1:60938 time-wait 0"}
	for i, w := range want {
		got := socks[i].String() + " " + socks[i].State + " " + itoa(socks[i].Inode)
		if got != w {
			t.Errorf("tcp %d = %q, want %q", i, got, w)
		}
	}
	if !socks[1].Wildcard() || socks[0].Wildcard() {
		t.Error("wildcard detection")
	}

	socks, err = ParseNetFile([]byte(tcp6Sample), "tcp6")
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"[::1]:8080", "[::]:80", "127.0.0.1:8081", "[2001:db8::1]:22"}
	for i, w := range want {
		if got := socks[i].String(); got != w {
			t.Errorf("tcp6 %d = %q, want %q", i, got, w)
		}
	}
	if !socks[1].Wildcard() {
		t.Error(":: is a wildcard")
	}

	socks, _ = ParseNetFile([]byte(udpSample), "udp")
	if len(socks) != 1 || socks[0].String() != "0.0.0.0:68" || socks[0].State != "bound" {
		t.Fatalf("udp %+v", socks)
	}

	for _, bad := range []string{"h\n 0: XYZ:0050 0 0A a b c d e 5", "h\n 0: 0100007F 0 0A a b c d e 5", "h\n 0: 0100007F:ZZZZ 0 0A a b c d e 5"} {
		if _, err := ParseNetFile([]byte(bad), "tcp"); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func itoa(n uint64) string { return strconv.FormatUint(n, 10) }

func TestGroupPIDsParsesCommWithSpaces(t *testing.T) {
	dir := t.TempDir()
	old := Root
	Root = dir
	defer func() { Root = old }()
	write := func(pid, stat string) {
		os.MkdirAll(dir+"/"+pid, 0o755)
		os.WriteFile(dir+"/"+pid+"/stat", []byte(stat), 0o644)
	}
	write("100", "100 (python3) S 1 100 100 0 -1")
	write("101", "101 (evil) 1 100 (x) S 100 100 100 0 -1") // comm with ") 1 100 ("
	write("102", "102 (other) S 1 102 102 0 -1")
	pids, err := GroupPIDs(100)
	if err != nil {
		t.Skip(err)
	}
	if len(pids) != 2 {
		t.Fatalf("pids %v", pids)
	}
}

// A real listener in a new process group is found, at its real address.
func TestListeningRealProcess(t *testing.T) {
	if _, err := os.Stat("/proc/self/fd"); err != nil {
		t.Skip("no /proc")
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	cmd := exec.Command(py, "-c", `
import socket, time
s = socket.socket(); s.bind(("127.0.0.1", 0)); s.listen()
print(s.getsockname()[1], flush=True)
time.sleep(30)`)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	buf := make([]byte, 16)
	n, _ := out.Read(buf)
	port := strings.TrimSpace(string(buf[:n]))

	var socks []Socket
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if socks, err = Listening(cmd.Process.Pid); len(socks) > 0 {
			break
		}
	}
	if err != nil || len(socks) != 1 || socks[0].String() != "127.0.0.1:"+port || socks[0].State != "listen" {
		t.Fatalf("Listening = %+v, %v (port %s)", socks, err, port)
	}
	// Our own test process isn't in that group.
	if s, _ := Listening(os.Getpid() + 1_000_000); len(s) != 0 {
		t.Fatalf("unrelated group has sockets: %+v", s)
	}
}
