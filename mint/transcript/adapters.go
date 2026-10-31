package transcript

import (
	"regexp"
	"strings"
)

// Adapter knows one interactive program: the options that make it behave
// with piped input, how to tell when a command's output has ended, when
// typed text is a whole command, and which commands reach outside it.
type Adapter struct {
	Name string
	// BaseArgs go before the user's options.
	BaseArgs []string
	// SafeArgs are added when the program accepts them (sqlite3 -safe
	// turns off .shell and friends); SafeProbe is how Mint checks.
	SafeArgs  []string
	SafeProbe []string
	// Markers are commands that print token on stdout and, if the
	// program can, on stderr. Without them Mint waits for the output to
	// go quiet (IdleEnd).
	Markers func(token string) (stdout, stderr string)
	// Complete reports whether text is a whole command to send now.
	// Partial SQL must never be sent: psql runs a following \echo before
	// an unfinished statement, so the end marker would come first.
	Complete func(text string) bool
	// Dollar enables PostgreSQL $tag$...$tag$ quoting for the scanner that
	// Complete and EndsInLineComment share.
	Dollar bool
	// Escape returns why line reaches outside the program (runs a shell,
	// pipes to one, loads code), or "".
	Escape func(line string) string
	// Comment reports whether a script line is only a comment or blank.
	Comment func(line string) bool
	// Secrets find passwords typed in a command: they are masked when the
	// command is shown and keep it out of saved history.
	Secrets []SecretRule
	// Refuse returns why line must not be sent at all, or "". These are
	// commands that would read their answer (a password, usually) from
	// standard input, where the next command and Mint's markers come from.
	Refuse func(line string) string
	// Cancel is how a running command is cancelled without ending the
	// program. With piped input most programs quit on Ctrl+C (psql,
	// sqlite3, redis-cli), so only those checked to carry on use CancelSignal.
	Cancel CancelMode
}

// CancelMode says how Session.Interrupt cancels a running command.
type CancelMode int

const (
	// CancelNone: the program has no way to cancel a command and carry
	// on; ending the session is the only way to stop one.
	CancelNone CancelMode = iota
	// CancelSignal: SIGINT to the program's process group, as Ctrl+C in a
	// terminal (gdb stops the program it is debugging and carries on).
	CancelSignal
	// CancelBackend: PostgreSQL. A second short psql connection asks the
	// server to cancel this session's query (pg_cancel_backend), as psql's
	// own Ctrl+C does; SIGINT would make psql quit, since its input is a pipe.
	CancelBackend
)

func everyLine(string) bool { return true }

// Adapters are the programs Mint knows. "lines" is for any other: one
// command per line, the end of output found by quiet.
var Adapters = map[string]Adapter{
	"psql": {
		Name:     "psql",
		BaseArgs: []string{"-X", "-q"}, // no ~/.psqlrc, no chatter
		Markers: func(t string) (string, string) {
			return `\echo ` + t, `\warn ` + t // \warn: psql 13 and later
		},
		Complete: func(s string) bool { return sqlComplete(s, true) },
		Dollar:   true,
		Escape: matchFirst(
			rule(`^\s*\\!`, "runs a shell command"),
			rule(`^\s*\\(o|out|g|gx|w|write)\s*\|`, "sends output to a shell command"),
			rule(`(?i)^\s*\\copy\b.*\bprogram\b`, "runs a program on this computer"),
			rule(`(?i)^\s*copy\b.*\bprogram\b`, "runs a program on the database server"),
			rule(`^\s*\\(i|include|ir|include_relative)\b`, "runs the commands in another file"),
			rule(`^\s*\\setenv\b`, "changes the environment of commands psql runs"),
		),
		Secrets: sqlSecrets,
		Refuse: matchFirst(
			rule(`^\s*\\password\b`, "asks for the new password on psql's input, which here is the command box: use ALTER ROLE … PASSWORD, or psql in a terminal"),
			rule(`^\s*\\prompt\b`, "reads its answer from psql's input, which here is the command box"),
		),
		Cancel:  CancelBackend,
		Comment: func(l string) bool { t := strings.TrimSpace(l); return t == "" || strings.HasPrefix(t, "--") },
	},
	"sqlite3": {
		Name:      "sqlite3",
		SafeArgs:  []string{"-safe"},
		SafeProbe: []string{"-safe", "-version"},
		Markers:   func(t string) (string, string) { return ".print " + t, "" },
		Secrets:   sqlSecrets,
		Complete:  func(s string) bool { return sqlComplete(s, false) },
		Escape: matchFirst(
			rule(`^\s*\.(shell|system)\b`, "runs a shell command"),
			rule(`^\s*\.(output|once)\s+['"]?\|`, "sends output to a shell command"),
			rule(`^\s*\.(import|read)\s+['"]?\|`, "reads from a shell command"),
			rule(`^\s*\.load\b`, "loads compiled code into sqlite3"),
			rule(`^\s*\.read\b`, "runs the commands in another file"),
			rule(`(?i)\bload_extension\s*\(`, "loads compiled code into sqlite3"),
		),
		Comment: func(l string) bool { t := strings.TrimSpace(l); return t == "" || strings.HasPrefix(t, "--") },
	},
	"gdb": {
		Name: "gdb",
		// No init files, no prompt (it would prefix every line), no
		// paging or confirmation questions, which would wait for input.
		BaseArgs: []string{"-q", "-nx", "-iex", "set prompt", "-iex", "set pagination off",
			"-iex", "set confirm off", "-iex", "set width 0", "-iex", "set height 0"},
		Markers:  func(t string) (string, string) { return `echo ` + t + `\n`, "" },
		Complete: everyLine,
		Escape: matchFirst(
			rule(`^\s*(shell|!|pipe|\|)`, "runs a shell command"),
			rule(`^\s*(python|py|python-interactive|pi|guile|gu)\b`, "runs Python or Guile code"),
			rule(`^\s*make\b`, "runs make"),
			rule(`^\s*source\b`, "runs the commands in another file"),
		),
		Cancel:  CancelSignal,
		Comment: func(l string) bool { t := strings.TrimSpace(l); return t == "" || strings.HasPrefix(t, "#") },
	},
	"redis-cli": {
		Name:     "redis-cli",
		Markers:  func(t string) (string, string) { return "ECHO " + t, "" },
		Complete: everyLine,
		Escape: matchFirst(
			rule(`(?i)^\s*module\s+load\b`, "loads code into the server"),
			rule(`(?i)^\s*config\s+set\b`, "changes the server's settings"),
			rule(`(?i)^\s*(eval|evalsha|fcall|function\s+load)\b`, "runs a script on the server"),
			rule(`(?i)^\s*(flushall|flushdb|shutdown|debug)\b`, "deletes data or stops the server"),
		),
		Secrets: []SecretRule{
			secret(`(?i)^(\s*auth\s+(?:\S+\s+)?)(\S+)`),
			secret(`(?i)^(\s*config\s+set\s+(?:requirepass|masterauth)\s+)(\S+)`),
			secret(`(?i)(\bacl\s+setuser\b.*?\s>)(\S+)`),
			secret(`(?i)^(\s*hello\s+\S+\s+auth\s+\S+\s+)(\S+)`),
		},
		Comment: func(l string) bool { return strings.TrimSpace(l) == "" },
	},
	"ftp": {
		Name:     "ftp",
		BaseArgs: []string{"-n"}, // no auto-login prompts on a pipe
		Complete: everyLine,
		Escape:   matchFirst(rule(`^\s*!`, "runs a shell command on this computer")),
		Secrets: []SecretRule{
			secret(`(?i)^(\s*user\s+\S+\s+)(\S+)`),
			secret(`(?i)^(\s*account\s+)(\S+)`),
		},
		Refuse:  matchFirst(rule(`(?i)^\s*user\s+\S+\s*$`, "would ask for the password on ftp's input, which here is the command box: type user NAME PASSWORD")),
		Comment: func(l string) bool { return strings.TrimSpace(l) == "" },
	},
	"lines": {
		Name:     "lines",
		Complete: everyLine,
		Escape:   func(string) string { return "" },
		Comment:  func(l string) bool { return strings.TrimSpace(l) == "" },
	},
}

// SecretRule finds a password in a command: Re's second group is the
// secret, its first what comes before it.
type SecretRule struct{ Re *regexp.Regexp }

func secret(re string) SecretRule { return SecretRule{regexp.MustCompile(re)} }

// sqlSecrets: CREATE/ALTER ROLE … PASSWORD '…', MySQL-style
// IDENTIFIED BY '…', and PostgreSQL dollar quotes.
var sqlSecrets = []SecretRule{
	secret(`(?is)(\bpassword\s+)('(?:[^']|'')*'|(\$\w*\$).*?\$\w*\$)`),
	secret(`(?is)(\bidentified\s+(?:with\s+\S+\s+)?by\s+)('(?:[^']|'')*')`),
}

// MaskSecrets returns cmd with any password the adapter finds replaced
// by •••, and whether it found one.
func MaskSecrets(a Adapter, cmd string) (string, bool) {
	found := false
	for _, r := range a.Secrets {
		cmd = r.Re.ReplaceAllStringFunc(cmd, func(m string) string {
			found = true
			sub := r.Re.FindStringSubmatch(m)
			return sub[1] + "•••"
		})
	}
	return cmd, found
}

type escapeRule struct {
	re  *regexp.Regexp
	why string
}

func rule(re, why string) escapeRule { return escapeRule{regexp.MustCompile(re), why} }

func matchFirst(rules ...escapeRule) func(string) string {
	return func(line string) string {
		for _, r := range rules {
			if r.re.MatchString(line) {
				return r.why
			}
		}
		return ""
	}
}

// IsClientCommand reports whether text is a psql backslash command or a
// sqlite3 dot command: a single line the client runs at once, even in the
// middle of an unfinished statement.
func IsClientCommand(text string) bool {
	t := strings.TrimSpace(text)
	return strings.HasPrefix(t, `\`) || strings.HasPrefix(t, ".")
}

// sqlComplete reports whether text ends a statement: a ";" outside
// quotes and comments is the last thing in it, or (dot or backslash
// commands) the line is a client command. dollar enables PostgreSQL
// $tag$…$tag$ quoting.
func sqlComplete(text string, dollar bool) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	if IsClientCommand(t) {
		return true // \d, .tables: one line, run at once
	}
	state, _, lastSemi := scanSQL(t, dollar)
	return (state == sqlNormal || state == sqlLineComment) && lastSemi
}

// sqlScanState is where a scan of SQL text ended.
const (
	sqlNormal = iota
	sqlSingle
	sqlDouble
	sqlLineComment
	sqlBlockComment
	sqlDollarQuote
)

// scanSQL walks text the way a SQL client does and reports the state it
// finished in, the open comment depth, and whether the last character that
// was not whitespace is a semicolon.
func scanSQL(t string, dollar bool) (state, depth int, lastSemi bool) {
	const (
		normal       = sqlNormal
		single       = sqlSingle
		double       = sqlDouble
		lineComment  = sqlLineComment
		blockComment = sqlBlockComment
		dollarQuote  = sqlDollarQuote
	)
	state, depth = normal, 0
	tag := ""
	for i := 0; i < len(t); i++ {
		c := t[i]
		switch state {
		case normal:
			switch {
			case c == '\'':
				state = single
			case c == '"':
				state = double
			case c == '-' && i+1 < len(t) && t[i+1] == '-':
				state = lineComment
				i++
				continue
			case c == '/' && i+1 < len(t) && t[i+1] == '*':
				state, depth = blockComment, 1
				i++
				continue
			case c == '$' && dollar:
				if j := strings.IndexByte(t[i+1:], '$'); j >= 0 && validTag(t[i+1:i+1+j]) {
					tag = t[i : i+j+2]
					state = dollarQuote
					i += j + 1
				}
			}
			if c == ';' {
				lastSemi = true
			} else if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
				lastSemi = false
			}
		case single:
			if c == '\'' {
				if i+1 < len(t) && t[i+1] == '\'' {
					i++ // '' inside a string
				} else {
					state = normal
				}
			}
		case double:
			if c == '"' {
				state = normal
			}
		case lineComment:
			if c == '\n' {
				state = normal
			}
		case blockComment:
			if c == '*' && i+1 < len(t) && t[i+1] == '/' {
				if depth--; depth == 0 {
					state = normal
				}
				i++
			} else if c == '/' && i+1 < len(t) && t[i+1] == '*' {
				depth++ // PostgreSQL nests them
				i++
			}
		case dollarQuote:
			if strings.HasPrefix(t[i:], tag) {
				state = normal
				i += len(tag) - 1
			}
		}
	}
	return state, depth, lastSemi
}

// EndsInLineComment reports whether text ends inside a "--" comment: the
// comment runs to the end of the text, so anything appended after it on the
// same line becomes part of the comment. A program that is sent a closing
// marker on such a line swallows it.
func EndsInLineComment(text string, dollar bool) bool {
	state, _, _ := scanSQL(text, dollar)
	return state == sqlLineComment
}

func validTag(s string) bool {
	for i, r := range s {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
