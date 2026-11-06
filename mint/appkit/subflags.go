package appkit

// Each command's own options.
//
// A command in Spec.Subcommands may have options of its own (SubcommandSpec
// Flags), as well as the program's options (Spec.Flags), which every
// command shows. The options in use are the selected command's own, then
// the program's (flags). Only the selected command's own options are in
// Values: selecting another command puts them aside and brings that
// command's back (useSubcommand), so two commands that use one option
// letter for different things (docker build -f is a file, docker logs -f
// follows) never share a value. A secret typed for one command is
// forgotten when another is selected, so it is never sent to a command it
// was not typed for.

// ownFlags is a command's own options, without any the program itself has.
func (a *ToolApp) ownFlags(sub *SubcommandSpec) []FlagSpec {
	if sub == nil || len(sub.Flags) == 0 {
		return nil
	}
	global := make(map[string]bool, len(a.Spec.Flags))
	for _, f := range a.Spec.Flags {
		global[f.Flag] = true
	}
	var out []FlagSpec
	for _, f := range sub.Flags {
		if f.Flag == "" || global[f.Flag] {
			continue
		}
		out = append(out, f)
	}
	return out
}

// flags is the options in use: the selected command's own, then the
// program's.
func (a *ToolApp) flags() []FlagSpec {
	own := a.ownFlags(a.activeSubcommandSpec())
	if len(own) == 0 {
		return a.Spec.Flags
	}
	return append(own, a.Spec.Flags...)
}

// allFlags is every option of the program and of all its commands, for
// what must cover any of them (masking secrets in the output).
func (a *ToolApp) allFlags() []FlagSpec {
	out := append([]FlagSpec(nil), a.Spec.Flags...)
	for i := range a.Spec.Subcommands {
		out = append(out, a.ownFlags(&a.Spec.Subcommands[i])...)
	}
	return out
}

// useSubcommand selects a command: the old one's own option values are put
// aside (and its secrets forgotten), and the new one's are brought back, or
// set to their defaults.
func (a *ToolApp) useSubcommand(name string) {
	if name != a.ActiveSubcommand {
		if old := a.activeSubcommandSpec(); old != nil {
			saved := map[string]string{}
			for _, f := range a.ownFlags(old) {
				if f.Kind == FlagSecret {
					a.forgetSecret(f.SecretKey())
					continue
				}
				if v, ok := a.Values[f.Flag]; ok {
					saved[f.Flag] = v
				}
				delete(a.Values, f.Flag)
			}
			if a.subValues == nil {
				a.subValues = map[string]map[string]string{}
			}
			a.subValues[old.Name] = saved
		}
		a.ActiveSubcommand = name
		for k, v := range a.subValues[name] {
			a.Values[k] = v
		}
	}
	a.fillSubDefaults()
}

// fillSubDefaults gives the selected command's own options their default
// values where they have none yet.
func (a *ToolApp) fillSubDefaults() {
	if a.Values == nil {
		a.Values = map[string]string{}
	}
	for _, f := range a.ownFlags(a.activeSubcommandSpec()) {
		if _, ok := a.Values[f.Flag]; ok {
			continue
		}
		switch {
		case f.Default != "":
			a.Values[f.Flag] = f.Default
		case f.Kind == FlagBool:
			a.Values[f.Flag] = "false"
		}
	}
}

// forgetSecret drops a stored secret.
func (a *ToolApp) forgetSecret(key string) {
	a.mu.Lock()
	st := a.store
	delete(a.secretSet, key)
	a.mu.Unlock()
	if st != nil {
		st.Delete(key)
	}
}
