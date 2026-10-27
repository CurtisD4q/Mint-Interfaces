package appkit

import (
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func serviceApp(t *testing.T, name, bin string, adapter string) *ToolApp {
	t.Helper()
	v, ok := KnownServiceSpec(adapter)
	if !ok {
		v = ServiceSpec{Adapter: adapter}
	}
	return newTestApp(t, ToolSpec{Name: name, Bin: bin, Service: &v})
}

func TestServiceArgs(t *testing.T) {
	cases := []struct {
		adapter, bin string
		want         string
	}{
		{"python-http", "python3", "-m http.server --bind 127.0.0.1 --directory /srv/site -v 8000"},
		{"php", "php", "-S 127.0.0.1:8000 -t /srv/site -v"},
		{"httpd", "busybox", "httpd -f -p 127.0.0.1:8080 -h /srv/site -v"},
		{"nc", "nc", "-l -v 127.0.0.1 9000"},
	}
	for _, c := range cases {
		a := serviceApp(t, c.adapter, c.bin, c.adapter)
		a.Values[svcDirKey] = "/srv/site"
		a.ExtraArgs = "-v"
		if args, _ := a.BuildArgs(); strings.Join(args, " ") != c.want {
			t.Errorf("%s: %q, want %q", c.adapter, strings.Join(args, " "), c.want)
		}
	}
	// IPv6 addresses are bracketed where a host:port is needed.
	a := serviceApp(t, "php", "php", "php")
	a.Values[svcBindKey] = "::1"
	a.Values[svcPortKey] = "8443"
	a.Values[svcDirKey] = "/w"
	if args, _ := a.BuildArgs(); strings.Join(args, " ") != "-S [::1]:8443 -t /w" {
		t.Fatalf("php v6 %q", args)
	}
	g := newTestApp(t, ToolSpec{Name: "srv", Service: &ServiceSpec{Adapter: ServiceGeneric, PortFlag: "--port", BindFlag: "--host"}})
	if args, _ := g.BuildArgs(); strings.Join(args, " ") != "--host 127.0.0.1 --port 8000" {
		t.Fatalf("generic %q", args)
	}
	// A bad saved value falls back to the default.
	a.Values[svcBindKey] = "evil; rm -rf /"
	a.Values[svcPortKey] = "99999"
	if a.svcBind() != "127.0.0.1" || a.svcPort() != 8000 {
		t.Fatalf("bad values used: %s %d", a.svcBind(), a.svcPort())
	}
}

func TestServiceWarnings(t *testing.T) {
	dir := t.TempDir()
	a := serviceApp(t, "http.server", "python3", "python-http")
	a.Values[svcDirKey] = dir
	if w := a.ServiceWarnings(); len(w) != 0 {
		t.Fatalf("defaults warn: %+v", w)
	}
	a.Values[svcBindKey] = "0.0.0.0"
	w := a.ServiceWarnings()
	if len(w) != 1 || w[0].Blocking || !strings.Contains(w[0].Text, "anyone who can reach this computer can use "+dir) {
		t.Fatalf("wildcard: %+v", w)
	}
	home, _ := os.UserHomeDir()
	a.Values[svcBindKey] = "127.0.0.1"
	a.Values[svcDirKey] = home
	if w := a.ServiceWarnings(); len(w) == 0 || !w[0].Blocking || !strings.Contains(w[0].Text, "home folder") {
		t.Fatalf("home: %+v", w)
	}
	a.Values[svcDirKey] = "/"
	if w := a.ServiceWarnings(); len(w) == 0 || !w[0].Blocking {
		t.Fatalf("root: %+v", w)
	}
	a.Values[svcDirKey] = dir
	// A port in use.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	a.Values[svcPortKey] = strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	if w := a.ServiceWarnings(); len(w) != 1 || !strings.Contains(w[0].Text, "already in use") {
		t.Fatalf("in use: %+v", w)
	}
	if !runningAsRoot() {
		a.Values[svcPortKey] = "80"
		if w := a.ServiceWarnings(); len(w) == 0 || !strings.Contains(w[0].Text, "below 1024") {
			t.Fatalf("low port: %+v", w)
		}
	}
}

func TestValidateService(t *testing.T) {
	for _, v := range []ServiceSpec{{}, {Adapter: "apache"}, {Adapter: ServiceGeneric},
		{Adapter: ServiceGeneric, PortFlag: "port"}, {Adapter: ServicePHP, DefaultPort: 70000},
		{Adapter: ServicePHP, DefaultBind: "localhost; x"}} {
		v := v
		if err := (ToolSpec{Name: "x", Service: &v}).Validate(); err == nil {
			t.Errorf("accepted %+v", v)
		}
	}
	for _, n := range []string{"python-http", "php", "httpd", "nc"} {
		v, _ := KnownServiceSpec(n)
		if err := (ToolSpec{Name: n, Service: &v}).Validate(); err != nil {
			t.Errorf("%s: %v", n, err)
		}
	}
}

func waitStatus(t *testing.T, a *ToolApp, state string) ServiceStatus {
	t.Helper()
	var st ServiceStatus
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		a.svcProbe = serviceProbe{} // skip the cache
		if st = a.ServiceStatus(); st.State == state {
			return st
		}
	}
	t.Fatalf("status %+v, want %s; output %q", st, state, a.ActiveJob().Output())
	return st
}

// The real python3 -m http.server: starts, is seen listening at its real
// address, serves the chosen folder only, and stops cleanly.
func TestServicePythonHTTP(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	site := t.TempDir()
	os.WriteFile(filepath.Join(site, "index.html"), []byte("hello from mint"), 0o644)
	a := serviceApp(t, "http.server", py, "python-http")
	port := freePort(t)
	a.Values[svcPortKey], a.Values[svcDirKey] = strconv.Itoa(port), site
	if st := a.ServiceStatus(); st.State != "stopped" {
		t.Fatalf("before start: %+v", st)
	}
	job, err := a.StartJob()
	if err != nil {
		t.Fatal(err)
	}
	defer job.Stop()
	st := waitStatus(t, a, "listening")
	want := "127.0.0.1:" + strconv.Itoa(port)
	if st.Text != "Listening on "+want || st.Open || a.serviceURL(st) != "http://"+want+"/" {
		t.Fatalf("status %+v url %q", st, a.serviceURL(st))
	}
	resp, err := http.Get("http://" + want + "/index.html")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "hello from mint" {
		t.Fatalf("served %q", body)
	}
	// The request shows in the log.
	for deadline := time.Now().Add(3 * time.Second); !strings.Contains(job.Output(), "GET /index.html") && time.Now().Before(deadline); {
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(job.Output(), "GET /index.html") {
		t.Fatalf("request not logged: %q", job.Output())
	}
	if b := a.serviceBadges(); b[len(b)-1].Text != "listening" {
		t.Fatalf("badge %+v", b)
	}
	job.Stop()
	job.Wait()
	if _, err := net.DialTimeout("tcp", want, 300*time.Millisecond); err == nil {
		t.Fatal("still listening after Stop")
	}
	if st := a.ServiceStatus(); st.State != "exited" {
		t.Fatalf("after stop %+v", st)
	}
}

// Listening on every network is reported as open.
func TestServiceOpenToNetwork(t *testing.T) {
	nc, err := exec.LookPath("nc")
	if err != nil {
		t.Skip("nc not installed")
	}
	if out, _ := exec.Command(nc, "-h").CombinedOutput(); !strings.Contains(string(out), "OpenBSD") {
		t.Skip("not OpenBSD netcat")
	}
	a := serviceApp(t, "nc", nc, "nc")
	a.Values[svcPortKey] = strconv.Itoa(freePort(t))
	a.Values[svcBindKey] = "0.0.0.0"
	job, err := a.StartJob()
	if err != nil {
		t.Fatal(err)
	}
	defer job.Stop()
	st := waitStatus(t, a, "listening")
	if !st.Open || !strings.HasPrefix(st.Text, "Listening on 0.0.0.0:") {
		t.Fatalf("status %+v", st)
	}
	if b := a.serviceBadges(); !strings.Contains(b[len(b)-1].Text, "open to the network") {
		t.Fatalf("badge %+v", b)
	}
	// Received bytes are logged, binary summarised (runner, #37).
	c, err := net.Dial("tcp", "127.0.0.1:"+a.Values[svcPortKey])
	if err != nil {
		t.Fatal(err)
	}
	c.Write([]byte("hello\n\x00\x01\x02binary\n"))
	c.Close()
	for deadline := time.Now().Add(3 * time.Second); !strings.Contains(job.Output(), "[binary data") && time.Now().Before(deadline); {
		time.Sleep(50 * time.Millisecond)
	}
	if out := job.Output(); !strings.Contains(out, "hello") || !strings.Contains(out, "[binary data") {
		t.Fatalf("output %q", out)
	}
}

// A server that fails to start says why.
func TestServiceExitShown(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed")
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	a := serviceApp(t, "php", php, "php")
	a.Values[svcPortKey] = strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	a.Values[svcDirKey] = t.TempDir()
	job, err := a.StartJob()
	if err != nil {
		t.Fatal(err)
	}
	job.Wait()
	st := a.ServiceStatus()
	if st.State != "exited" || !strings.Contains(st.Text, "Exited (code") {
		t.Fatalf("status %+v; output %q", st, job.Output())
	}
}
