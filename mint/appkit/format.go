package appkit

import "strings"

// Add-ons that parse a tool's output (WithDiff, WithFindings) need it in a
// format they can read, so they add options such as "-u" or "-f json1".
// The helpers here decide when, and where in the argument list.

// usesFlag reports whether args, before any "--", use one of flags: as
// itself, as --flag=value, or as a short option with its value attached
// (-U3, -fjson).
func usesFlag(args, flags []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		for _, f := range flags {
			switch {
			case arg == f, strings.HasPrefix(f, "--") && strings.HasPrefix(arg, f+"="):
				return true
			case len(f) == 2 && f[0] == '-' && f[1] != '-' && len(arg) > 2 && arg[1] != '-' && strings.HasPrefix(arg, f):
				return true
			}
		}
	}
	return false
}

// usesAny reports whether args, before any "--", use one of forms. A form
// is a flag as usesFlag reads it ("-u", "--format=json1", "-fjson1"), or a
// flag and its value as two words ("-f json1").
func usesAny(args, forms []string) bool {
	var single []string
	for _, form := range forms {
		flag, value, two := strings.Cut(form, " ")
		if !two {
			single = append(single, form)
			continue
		}
		for i := 0; i+1 < len(args) && args[i] != "--"; i++ {
			if args[i] == flag && args[i+1] == value {
				return true
			}
		}
	}
	return usesFlag(args, single)
}

// forceFormat inserts force into args at index at (just after any
// subcommand words, so "trivy image -f sarif …" and "diff -u -i -- a b"),
// unless the user already chose an output format with one of
// formatFlags. readable reports whether the output will be in a format
// the add-on reads: forced, or chosen by the user in one of the forms in
// readableFlags (see usesAny). The forced words, together, always count.
func forceFormat(args []string, at int, force, formatFlags, readableFlags []string) (out []string, readable bool) {
	readableFlags = append([]string{strings.Join(force, " ")}, readableFlags...)
	at = max(0, min(at, len(args)))
	if usesFlag(args[at:], formatFlags) || len(force) == 0 {
		return args, usesAny(args[at:], readableFlags)
	}
	out = make([]string, 0, len(args)+len(force))
	out = append(out, args[:at]...)
	out = append(out, force...)
	return append(out, args[at:]...), true
}
