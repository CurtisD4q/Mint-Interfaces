// Package procnet finds the network sockets a process holds, from Linux's
// /proc: which addresses and ports it is listening on. It reads only, and
// only the user's own processes (or any, as root).
package procnet

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Socket is one bound socket.
type Socket struct {
	Proto string // "tcp", "tcp6", "udp", "udp6"
	Addr  net.IP
	Port  int
	State string // "listen", "established", … ; "bound" for UDP
	Inode uint64
}

// String is "127.0.0.1:8000" or "[::1]:8000".
func (s Socket) String() string {
	return net.JoinHostPort(s.Addr.String(), strconv.Itoa(s.Port))
}

// Wildcard reports whether the socket accepts connections on every
// address (0.0.0.0 or ::).
func (s Socket) Wildcard() bool { return s.Addr.IsUnspecified() }

// ErrUnsupported is returned where there is no /proc to read.
var ErrUnsupported = errors.New("socket inspection needs Linux /proc")

var tcpStates = map[string]string{
	"01": "established", "02": "syn-sent", "03": "syn-recv", "04": "fin-wait1", "05": "fin-wait2",
	"06": "time-wait", "07": "close", "08": "close-wait", "09": "last-ack", "0A": "listen", "0B": "closing",
}

// ParseNetFile reads a /proc/net/{tcp,tcp6,udp,udp6} table.
func ParseNetFile(data []byte, proto string) ([]Socket, error) {
	var out []Socket
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 4096), 1<<20)
	first := true
	for sc.Scan() {
		if first { // header
			first = false
			continue
		}
		f := strings.Fields(sc.Text())
		if len(f) < 10 {
			continue
		}
		ip, port, err := parseAddr(f[1])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", proto, err)
		}
		inode, err := strconv.ParseUint(f[9], 10, 64)
		if err != nil {
			continue
		}
		state := tcpStates[strings.ToUpper(f[3])]
		if strings.HasPrefix(proto, "udp") {
			state = "bound"
			if strings.ToUpper(f[3]) == "01" {
				state = "connected"
			}
		}
		out = append(out, Socket{Proto: proto, Addr: ip, Port: port, State: state, Inode: inode})
	}
	return out, sc.Err()
}

// parseAddr reads "0100007F:1F40": the address as 32-bit words in host
// (little-endian) order, then the port in hex.
func parseAddr(s string) (net.IP, int, error) {
	host, portHex, ok := strings.Cut(s, ":")
	if !ok {
		return nil, 0, fmt.Errorf("bad address %q", s)
	}
	port, err := strconv.ParseUint(portHex, 16, 16)
	if err != nil {
		return nil, 0, fmt.Errorf("bad port in %q", s)
	}
	raw, err := hex.DecodeString(host)
	if err != nil || (len(raw) != 4 && len(raw) != 16) {
		return nil, 0, fmt.Errorf("bad address %q", s)
	}
	ip := make(net.IP, len(raw))
	for w := 0; w < len(raw); w += 4 {
		ip[w], ip[w+1], ip[w+2], ip[w+3] = raw[w+3], raw[w+2], raw[w+1], raw[w]
	}
	if len(ip) == 16 {
		if v4 := ip.To4(); v4 != nil && !ip.Equal(net.IPv6zero) && !ip.Equal(net.IPv6unspecified) {
			ip = v4 // ::ffff:a.b.c.d
		}
	}
	return ip, int(port), nil
}

// Root is the /proc to read; tests point it elsewhere.
var Root = "/proc"

// GroupPIDs returns the processes in process group pgid (a job the runner
// started leads its own group, so this is the job and its children).
func GroupPIDs(pgid int) ([]int, error) {
	if runtime.GOOS != "linux" {
		return nil, ErrUnsupported
	}
	entries, err := os.ReadDir(Root)
	if err != nil {
		return nil, err
	}
	var out []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(Root, e.Name(), "stat"))
		if err != nil {
			continue
		}
		// "pid (comm) state ppid pgrp …": comm may hold spaces and ")".
		rest := b[bytes.LastIndexByte(b, ')')+1:]
		f := strings.Fields(string(rest))
		if len(f) >= 3 {
			if g, err := strconv.Atoi(f[2]); err == nil && g == pgid {
				out = append(out, pid)
			}
		}
	}
	return out, nil
}

// SocketInodes returns the inodes of the sockets pid has open.
func SocketInodes(pid int) (map[uint64]bool, error) {
	dir := filepath.Join(Root, strconv.Itoa(pid), "fd")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[uint64]bool{}
	for _, e := range entries {
		link, err := os.Readlink(filepath.Join(dir, e.Name()))
		if err != nil || !strings.HasPrefix(link, "socket:[") {
			continue
		}
		if n, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]"), 10, 64); err == nil {
			out[n] = true
		}
	}
	return out, nil
}

// Listening returns the sockets the processes of group pgid are listening
// on (TCP) or bound to (UDP), with no duplicates.
func Listening(pgid int) ([]Socket, error) {
	pids, err := GroupPIDs(pgid)
	if err != nil {
		return nil, err
	}
	inodes := map[uint64]bool{}
	for _, pid := range pids {
		in, err := SocketInodes(pid)
		if err != nil {
			continue // exited, or not ours to read
		}
		for k := range in {
			inodes[k] = true
		}
	}
	if len(inodes) == 0 {
		return nil, nil
	}
	var out []Socket
	for _, proto := range []string{"tcp", "tcp6", "udp", "udp6"} {
		data, err := os.ReadFile(filepath.Join(Root, "net", proto))
		if err != nil {
			continue // no IPv6, say
		}
		socks, err := ParseNetFile(data, proto)
		if err != nil {
			continue
		}
		for _, s := range socks {
			if inodes[s.Inode] && s.Inode != 0 && (s.State == "listen" || s.State == "bound") {
				out = append(out, s)
			}
		}
	}
	return out, nil
}
