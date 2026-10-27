package appkit

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/procnet"
	"github.com/CurtisD4q/Mint-Interfaces/mint/runner"
)

// MixinService starts and stops a server and shows whether it is
// listening, where, and to whom.
const MixinService MixinName = "WithService"

// Service adapters (ServiceSpec.Adapter): how a server takes its port,
// address and folder.
const (
	ServicePythonHTTP    = "python-http"    // python3 -m http.server [-b ADDR] [-d DIR] PORT
	ServicePHP           = "php"            // php -S ADDR:PORT [-t DIR]
	ServiceBusyboxHTTPD  = "busybox-httpd"  // httpd -f -p ADDR:PORT [-h DIR]
	ServiceNetcatOpenBSD = "nc-openbsd"     // nc -l ADDR PORT (also ncat)
	ServiceNetcatClassic = "nc-traditional" // nc -l -p PORT -s ADDR
	ServiceGeneric       = "generic"        // PortFlag/BindFlag/DirFlag
)

// ServiceSpec configures WithService.
type ServiceSpec struct {
	Adapter     string `json:"adapter"`
	DefaultPort int    `json:"default_port,omitempty"`
	// DefaultBind is the address to listen on until the user changes it.
	// Empty means 127.0.0.1: only this computer. Mint never starts a
	// server open to the network unless the user chose that.
	DefaultBind string `json:"default_bind,omitempty"`
	ServesDir   bool   `json:"serves_dir,omitempty"` // the server shows a folder's files
	Scheme      string `json:"scheme,omitempty"`     // "http" to show a link to open
	// For ServiceGeneric: how the tool takes each setting.
	PortFlag string `json:"port_flag,omitempty"` // e.g. "--port"
	PortLast bool   `json:"port_last,omitempty"` // the port is the last argument instead
	BindFlag string `json:"bind_flag,omitempty"`
	DirFlag  string `json:"dir_flag,omitempty"`
}

var serviceDefaults = map[string]ServiceSpec{
	"python-http": {Adapter: ServicePythonHTTP, DefaultPort: 8000, ServesDir: true, Scheme: "http"},
	"php":         {Adapter: ServicePHP, DefaultPort: 8000, ServesDir: true, Scheme: "http"},
	"httpd":       {Adapter: ServiceBusyboxHTTPD, DefaultPort: 8080, ServesDir: true, Scheme: "http"},
	"nc":          {Adapter: ServiceNetcatOpenBSD, DefaultPort: 9000},
	"ncat":        {Adapter: ServiceNetcatOpenBSD, DefaultPort: 9000},
}

// KnownServiceSpec returns WithService's settings for a server Mint
// knows. For "python3 -m http.server", ask for "python-http".
func KnownServiceSpec(name string) (ServiceSpec, bool) {
	s, ok := serviceDefaults[toolBase(name)]
	return s, ok
}

func validateService(s *ToolSpec) error {
	v := s.Service
	switch v.Adapter {
	case ServicePythonHTTP, ServicePHP, ServiceBusyboxHTTPD, ServiceNetcatOpenBSD, ServiceNetcatClassic:
	case ServiceGeneric:
		if v.PortFlag == "" && !v.PortLast {
			return errors.New("the generic adapter needs port_flag or port_last")
		}
		for _, f := range []string{v.PortFlag, v.BindFlag, v.DirFlag} {
			if f != "" && !strings.HasPrefix(f, "-") {
				return fmt.Errorf("flags must start with -, got %q", f)
			}
		}
	case "":
		return errors.New("adapter is required")
	default:
		return fmt.Errorf("unknown adapter %q", v.Adapter)
	}
	if v.DefaultPort < 0 || v.DefaultPort > 65535 {
		return fmt.Errorf("default_port %d is not a port", v.DefaultPort)
	}
	if v.DefaultBind != "" && net.ParseIP(v.DefaultBind) == nil {
		return fmt.Errorf("default_bind %q is not an IP address", v.DefaultBind)
	}
	return nil
}

func init() {
	registerMixin(MixinInfo{
		Name: MixinService, Order: 105,
		Summary:  "start and stop a server, showing where it listens (Serve: http.server, php -S, nc -l)",
		Fields:   "Service *ServiceSpec",
		Enabled:  func(s *ToolSpec) bool { return s.Service != nil },
		Validate: validateService,
	})
}

// The service settings live in Values, so they are saved with the rest.
const (
	svcPortKey = "@service:port"
	svcBindKey = "@service:bind"
	svcDirKey  = "@service:dir"
)

func (a *ToolApp) svcPort() int {
	if p, err := strconv.Atoi(a.Values[svcPortKey]); err == nil && p >= 0 && p <= 65535 {
		return p
	}
	if a.Spec.Service.DefaultPort > 0 {
		return a.Spec.Service.DefaultPort
	}
	return 8000
}

func (a *ToolApp) svcBind() string {
	if b := a.Values[svcBindKey]; net.ParseIP(b) != nil {
		return b
	}
	if b := a.Spec.Service.DefaultBind; b != "" {
		return b
	}
	return "127.0.0.1"
}

func (a *ToolApp) svcDir() string {
	if d := a.Values[svcDirKey]; d != "" {
		return d
	}
	wd, _ := os.Getwd()
	return wd
}

// serviceArgs returns the arguments that go before the user's options
// (front) and after everything (back).
func (a *ToolApp) serviceArgs() (front, back []string) {
	v := a.Spec.Service
	port, bind := strconv.Itoa(a.svcPort()), a.svcBind()
	hostPort := net.JoinHostPort(bind, port)
	dir := a.svcDir()
	switch v.Adapter {
	case ServicePythonHTTP:
		front = []string{"-m", "http.server", "--bind", bind}
		if v.ServesDir {
			front = append(front, "--directory", dir)
		}
		back = []string{port}
	case ServicePHP:
		front = []string{"-S", hostPort}
		if v.ServesDir {
			front = append(front, "-t", dir)
		}
	case ServiceBusyboxHTTPD:
		if toolBase(a.Spec.BinaryName()) == "busybox" {
			front = []string{"httpd"}
		}
		front = append(front, "-f", "-p", hostPort)
		if v.ServesDir {
			front = append(front, "-h", dir)
		}
	case ServiceNetcatOpenBSD:
		front = []string{"-l"}
		back = []string{bind, port}
	case ServiceNetcatClassic:
		front = []string{"-l", "-p", port, "-s", bind}
	case ServiceGeneric:
		if v.BindFlag != "" {
			front = append(front, v.BindFlag, bind)
		}
		if v.ServesDir && v.DirFlag != "" {
			front = append(front, v.DirFlag, dir)
		}
		if v.PortLast {
			back = []string{port}
		} else {
			front = append(front, v.PortFlag, port)
		}
	}
	return front, back
}

// withServiceArgs places the service arguments around args.
func (a *ToolApp) withServiceArgs(args []string, at int) []string {
	if a.Spec.Service == nil {
		return args
	}
	front, back := a.serviceArgs()
	out := append([]string(nil), args[:at]...)
	out = append(out, front...)
	out = append(out, args[at:]...)
	return append(out, back...)
}

// ServiceWarning is something to tell the user before or while serving.
type ServiceWarning struct {
	Blocking bool // ask the user to type to confirm before starting
	Text     string
}

// isWildcard reports whether addr means every network.
func isWildcard(addr string) bool {
	ip := net.ParseIP(addr)
	return ip == nil || ip.IsUnspecified()
}

// ServiceWarnings checks the settings before starting.
func (a *ToolApp) ServiceWarnings() []ServiceWarning {
	v := a.Spec.Service
	var out []ServiceWarning
	bind, port := a.svcBind(), a.svcPort()
	if isWildcard(bind) {
		what := "the server"
		if v.ServesDir {
			what = a.svcDir()
		}
		out = append(out, ServiceWarning{Text: "Listening on every network (" + bind + "): anyone who can reach this computer can use " + what + "."})
	}
	if v.ServesDir {
		if why := sensitiveServeDir(a.svcDir()); why != "" {
			out = append(out, ServiceWarning{Blocking: true, Text: "The folder served is " + why + "."})
		}
		if fi, err := os.Stat(a.svcDir()); err != nil || !fi.IsDir() {
			out = append(out, ServiceWarning{Blocking: true, Text: a.svcDir() + " is not a folder."})
		}
	}
	if port > 0 && port < 1024 && !runningAsRoot() {
		out = append(out, ServiceWarning{Text: fmt.Sprintf("Ports below 1024 need root; %s will probably fail to start on %d.", a.Spec.Name, port)})
	}
	if job := a.ActiveJob(); job == nil || !job.Running() {
		if err := portFree(bind, port); err != nil {
			out = append(out, ServiceWarning{Text: err.Error()})
		}
	}
	return out
}

// sensitiveServeDir says why serving dir would expose too much, or "".
func sensitiveServeDir(dir string) string {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		real = filepath.Clean(dir)
	}
	home, _ := os.UserHomeDir()
	if inv := sudoInvoker(); inv != nil {
		home = inv.home
	}
	switch {
	case real == "/":
		return "the whole filesystem"
	case home != "" && real == filepath.Clean(home):
		return "your whole home folder, including keys and settings such as ~/.ssh"
	case filepath.Dir(real) == "/":
		return "a top-level system folder"
	}
	return ""
}

// portFree checks whether bind:port can be listened on now. It is only a
// hint: something else may take the port before the server does.
func portFree(bind string, port int) error {
	if port == 0 {
		return nil
	}
	l, err := net.Listen("tcp", net.JoinHostPort(bind, strconv.Itoa(port)))
	if err != nil {
		var se *os.SyscallError
		if errors.As(err, &se) && strings.Contains(se.Err.Error(), "in use") {
			return fmt.Errorf("Port %d is already in use on %s.", port, bind)
		}
		if strings.Contains(err.Error(), "address already in use") {
			return fmt.Errorf("Port %d is already in use on %s.", port, bind)
		}
		return nil // permission and the like: the server will say
	}
	l.Close()
	return nil
}

// ServiceStatus describes the server now.
type ServiceStatus struct {
	State     string // "stopped", "starting", "listening", "exited"
	Text      string // for people: "Listening on 127.0.0.1:8000"
	Listeners []procnet.Socket
	Open      bool // listening on every network
}

// serviceProbe caches the socket scan: it reads all of /proc, so not on
// every frame.
type serviceProbe struct {
	mu   sync.Mutex
	at   time.Time
	job  *runner.Job
	last []procnet.Socket
	err  error
}

func (p *serviceProbe) listeners(job *runner.Job) ([]procnet.Socket, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if job == p.job && time.Since(p.at) < 750*time.Millisecond {
		return p.last, p.err
	}
	p.job, p.at = job, time.Now()
	p.last, p.err = procnet.Listening(job.PID())
	return p.last, p.err
}

// Status reads the server's state: the job, and the sockets it holds (or,
// where /proc can't be read, whether the port answers).
func (a *ToolApp) ServiceStatus() ServiceStatus {
	job := a.ActiveJob()
	if job == nil {
		return ServiceStatus{State: "stopped", Text: "Stopped"}
	}
	if !job.Running() {
		st := ServiceStatus{State: "exited", Text: "Stopped"}
		switch job.State() {
		case runner.Failed:
			st.Text = fmt.Sprintf("Exited (code %d)", job.ExitCode())
			if e := job.LastError(); e != "" {
				st.Text += ": " + mint.CleanLineVisible(e)
			}
		case runner.Done:
			st.Text = "Exited"
		}
		return st
	}
	socks, err := a.svcProbe.listeners(job)
	if errors.Is(err, procnet.ErrUnsupported) {
		addr := net.JoinHostPort(a.svcBind(), strconv.Itoa(a.svcPort()))
		if isWildcard(a.svcBind()) {
			addr = net.JoinHostPort("127.0.0.1", strconv.Itoa(a.svcPort()))
		}
		if c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			c.Close()
			return ServiceStatus{State: "listening", Text: "Answering on " + addr, Open: isWildcard(a.svcBind())}
		}
		return ServiceStatus{State: "starting", Text: "Starting…"}
	}
	if len(socks) == 0 {
		text := "Starting…"
		if job.Elapsed() > 5*time.Second {
			text = "Running, but not listening on any port"
		}
		return ServiceStatus{State: "starting", Text: text}
	}
	st := ServiceStatus{State: "listening", Listeners: socks}
	var where []string
	for _, s := range socks {
		where = append(where, s.String())
		st.Open = st.Open || s.Wildcard()
	}
	st.Text = "Listening on " + strings.Join(where, ", ")
	return st
}

// serviceURL is a link to the server, if it speaks a known scheme.
func (a *ToolApp) serviceURL(st ServiceStatus) string {
	if a.Spec.Service.Scheme == "" || st.State != "listening" {
		return ""
	}
	host, port := a.svcBind(), strconv.Itoa(a.svcPort())
	if len(st.Listeners) > 0 {
		host, port = st.Listeners[0].Addr.String(), strconv.Itoa(st.Listeners[0].Port)
	}
	if isWildcard(host) {
		host = "127.0.0.1"
	}
	return a.Spec.Service.Scheme + "://" + net.JoinHostPort(host, port) + "/"
}

// ServiceScreen is the server's control panel.
func (a *ToolApp) ServiceScreen() *mint.Screen {
	v := a.Spec.Service
	bindChoices := []string{"127.0.0.1", "::1", "0.0.0.0", "::"}
	bindLabel := map[string]string{"127.0.0.1": "this computer", "::1": "this computer (IPv6)",
		"0.0.0.0": "every network", "::": "every network (IPv6)"}
	var labels []string
	for _, b := range bindChoices {
		labels = append(labels, bindLabel[b])
	}
	s := &mint.Screen{
		Icon:   "◉",
		Title:  a.Spec.Name + " — server",
		Status: a.serviceBadges,
		Poll: func(*mint.Screen) bool {
			job := a.ActiveJob()
			return job != nil && job.Changed()
		},
	}
	s.Refresh = func(scr *mint.Screen) {
		st := a.ServiceStatus()
		running := a.ActiveJob() != nil && a.ActiveJob().Running()
		status := []mint.Row{
			mint.Value(mint.ValueOpts{Label: "Status", Get: func() string { return a.ServiceStatus().Text }}),
		}
		if url := a.serviceURL(st); url != "" {
			status = append(status, mint.Value(mint.ValueOpts{Label: "Open", Value: url}))
		}
		if st.Open {
			status = append(status, mint.Label(mint.StyleCaption, "⚠ Open to every network this computer is on."))
		}
		settings := []mint.Row{
			mint.Text(mint.TextOpts{ID: "svc:port", Label: "Port", Get: func() string { return strconv.Itoa(a.svcPort()) },
				Set: func(val string) {
					p, err := strconv.Atoi(strings.TrimSpace(val))
					if err != nil || p < 0 || p > 65535 {
						mint.ToastError("A port is a number from 0 to 65535")
						return
					}
					a.Values[svcPortKey] = strconv.Itoa(p)
				}}),
			mint.Picker(mint.PickerOpts{ID: "svc:bind", Label: "Who can connect", Options: labels,
				Get: func() string {
					if l, ok := bindLabel[a.svcBind()]; ok {
						return l
					}
					return a.svcBind()
				},
				Set: func(l string) {
					for b, bl := range bindLabel {
						if bl == l {
							a.Values[svcBindKey] = b
						}
					}
				}}),
		}
		if v.ServesDir {
			settings = append(settings, mint.Text(mint.TextOpts{ID: "svc:dir", Label: "Folder served",
				Get: a.svcDir,
				Set: func(val string) {
					if val = strings.TrimSpace(val); val != "" {
						abs, _ := filepath.Abs(expandHome(val))
						a.Values[svcDirKey] = abs
					}
				}}))
		}
		if running {
			settings = append(settings, mint.Label(mint.StyleCaption, "Changes apply when the server restarts."))
		}
		var warn []mint.Row
		for _, w := range a.ServiceWarnings() {
			prefix := "Note: "
			if w.Blocking {
				prefix = "⚠ "
			}
			warn = append(warn, mint.Label(mint.StyleBody, prefix+mint.CleanLineVisible(w.Text)))
		}
		var acts []mint.Row
		if running {
			acts = append(acts,
				mint.Action(mint.ActionOpts{Label: "Stop", Hint: "stop", OnActivate: func(c *mint.Ctx) {
					if j := a.ActiveJob(); j != nil {
						j.Stop()
						c.Toast("Stopping " + a.Spec.Name)
					}
				}}),
				mint.Action(mint.ActionOpts{Label: "Restart with these settings", Hint: "restart", OnActivate: func(c *mint.Ctx) {
					if j := a.ActiveJob(); j != nil {
						j.Stop()
						j.Wait()
					}
					a.startService(c)
				}}))
		} else {
			acts = append(acts, mint.Action(mint.ActionOpts{Label: "Start", Hint: "start", OnActivate: a.startService}))
		}
		acts = append(acts, mint.Action(mint.ActionOpts{Label: "Open the request log…", Hint: "log", OnActivate: func(c *mint.Ctx) {
			c.Push(mint.ViewStyled(mint.ViewStyledOpts{Title: a.Spec.Name + " — log", Source: a.newFollowSource(), Follow: true, Status: a.serviceBadges}))
		}}))
		sections := []mint.Section{{Name: "Server", Rows: status}, {Name: "Settings", Rows: settings}}
		if len(warn) > 0 {
			sections = append(sections, mint.Section{Name: "Check before starting", Rows: warn})
		}
		sections = append(sections, mint.Section{Name: "Actions", Rows: acts},
			mint.Section{Name: "Recent requests", Rows: []mint.Row{mint.Log(mint.LogOpts{Label: "Log", Height: 8, Lines: func() []mint.LogLine {
				if j := a.ActiveJob(); j != nil {
					tail, _ := j.TailLines(200)
					return tail
				}
				return nil
			}})}})
		scr.Sections = sections
	}
	s.Refresh(s)
	return s
}

// startService checks the settings and starts the server.
func (a *ToolApp) startService(c *mint.Ctx) {
	for _, w := range a.ServiceWarnings() {
		if w.Blocking && !c.TypeToConfirm("Start anyway?", mint.CleanLineVisible(w.Text), a.Spec.Name) {
			c.ToastWarn("Not started")
			return
		}
	}
	if _, err := a.StartJob(); err != nil {
		c.ToastError(err.Error())
		return
	}
	c.Toast("Starting " + a.Spec.Name)
}

// serviceBadges adds the server's state to the usual badges.
func (a *ToolApp) serviceBadges() []mint.Badge {
	st := a.ServiceStatus()
	var b mint.Badge
	switch st.State {
	case "listening":
		b = mint.Badge{Text: "listening", Kind: mint.BadgeOK}
		if st.Open {
			b = mint.Badge{Text: "listening · open to the network", Kind: mint.BadgeWarn}
		}
	case "starting":
		b = mint.Badge{Text: "starting", Kind: mint.BadgeWarn}
	case "exited":
		b = mint.Badge{Text: strings.ToLower(st.Text), Kind: mint.BadgeError}
	default:
		b = mint.Badge{Text: "stopped", Kind: mint.BadgeIdle}
	}
	var data []mint.Badge
	if level, _ := a.DataLevel(); level != DataNormal {
		d, _ := dataBadge(level)
		data = append(data, d)
	}
	return append(data, b)
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
