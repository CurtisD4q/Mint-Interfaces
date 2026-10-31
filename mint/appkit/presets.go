package appkit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CurtisD4q/Mint-Interfaces/mint"
	"github.com/CurtisD4q/Mint-Interfaces/mint/util"
)

// savedPreset is one named set of option values. Secret options are never
// stored. The file lives next to the run history, with the same permissions.
type savedPreset struct {
	Name             string            `json:"name"`
	SavedAt          time.Time         `json:"saved_at"`
	Target           string            `json:"target,omitempty"`
	SecondTarget     string            `json:"second_target,omitempty"`
	ExtraArgs        string            `json:"extra_args,omitempty"`
	ActiveSubcommand string            `json:"active_subcommand,omitempty"`
	Values           map[string]string `json:"values,omitempty"`
	SaveToFile       bool              `json:"save_to_file,omitempty"`
	OutputFile       string            `json:"output_file,omitempty"`
	Profile          string            `json:"profile,omitempty"`
}

func (a *ToolApp) presetsPath() string {
	return filepath.Join(a.DataDir, "presets.json")
}

func (a *ToolApp) loadPresets() {
	b, err := os.ReadFile(a.presetsPath())
	if err != nil {
		return
	}
	var list []savedPreset
	if json.Unmarshal(b, &list) != nil {
		return
	}
	a.mu.Lock()
	a.presets = list
	a.mu.Unlock()
}

func (a *ToolApp) savePresets() {
	if a.DataDir == "" {
		return
	}
	_ = util.Mkdir(a.DataDir)
	a.mu.Lock()
	b, err := json.MarshalIndent(a.presets, "", "  ")
	a.mu.Unlock()
	if err != nil {
		return
	}
	if os.WriteFile(a.presetsPath(), b, 0o600) == nil {
		a.giveBack(a.presetsPath())
	}
}

// secretOption reports whether a flag's value must stay out of a preset.
func (a *ToolApp) secretOption(flag string) bool {
	for _, f := range a.Spec.Flags {
		if f.Flag != flag && f.SecretKey() != flag {
			continue
		}
		if f.Kind == FlagSecret || LooksSecret(f.Flag, f.Label, f.Hint) {
			return true
		}
	}
	return false
}

func (a *ToolApp) presetValues() map[string]string {
	out := map[string]string{}
	for k, v := range a.Values {
		if a.secretOption(k) {
			continue
		}
		out[k] = v
	}
	return out
}

func (a *ToolApp) capturePreset(name string) savedPreset {
	flags := a.secretFlagNames()
	return savedPreset{
		Name:             name,
		SavedAt:          time.Now(),
		Target:           maskFreeText(a.Target, flags),
		SecondTarget:     maskFreeText(a.SecondTarget, flags),
		ExtraArgs:        maskFreeText(a.ExtraArgs, flags),
		ActiveSubcommand: a.ActiveSubcommand,
		Values:           a.presetValues(),
		SaveToFile:       a.SaveToFile,
		OutputFile:       a.OutputFile,
		Profile:          a.Profile,
	}
}

// SavePreset stores the current options under name, replacing a preset
// with the same name. Secret and password options are left out.
func (a *ToolApp) SavePreset(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || a.DataDir == "" {
		return false
	}
	rec := a.capturePreset(name)
	a.mu.Lock()
	replaced := false
	for i := range a.presets {
		if a.presets[i].Name == name {
			a.presets[i] = rec
			replaced = true
			break
		}
	}
	if !replaced {
		a.presets = append(a.presets, rec)
	}
	a.mu.Unlock()
	a.savePresets()
	return true
}

// Presets returns the saved presets in the order they were saved.
func (a *ToolApp) Presets() []savedPreset {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]savedPreset, len(a.presets))
	copy(out, a.presets)
	return out
}

// DeletePreset removes the named preset. It reports whether one was removed.
func (a *ToolApp) DeletePreset(name string) bool {
	a.mu.Lock()
	kept := a.presets[:0]
	found := false
	for _, p := range a.presets {
		if p.Name == name {
			found = true
			continue
		}
		kept = append(kept, p)
	}
	if found {
		a.presets = kept
	}
	a.mu.Unlock()
	if found {
		a.savePresets()
	}
	return found
}

// ApplyPreset copies a saved preset onto the current options. Secrets were
// never stored, so they are left as they are.
func (a *ToolApp) ApplyPreset(name string) bool {
	a.mu.Lock()
	var rec savedPreset
	found := false
	for _, p := range a.presets {
		if p.Name == name {
			rec = p
			found = true
			break
		}
	}
	a.mu.Unlock()
	if !found {
		return false
	}
	a.Target = rec.Target
	a.SecondTarget = rec.SecondTarget
	a.ExtraArgs = rec.ExtraArgs
	if rec.ActiveSubcommand != "" {
		a.ActiveSubcommand = rec.ActiveSubcommand
	}
	a.SaveToFile = rec.SaveToFile
	if rec.OutputFile != "" {
		a.OutputFile = rec.OutputFile
	}
	if rec.Profile != "" {
		a.Profile = rec.Profile
	}
	for _, f := range a.Spec.Flags {
		if f.Kind == FlagSecret || LooksSecret(f.Flag, f.Label, f.Hint) {
			continue
		}
		key := f.Flag
		if v, ok := rec.Values[key]; ok {
			a.Values[key] = v
		}
	}
	return true
}

// PresetsScreen saves the current options, and lists, reruns and deletes
// what was saved.
func (a *ToolApp) PresetsScreen() *mint.Screen {
	s := &mint.Screen{Icon: "◇", Title: "Presets"}
	s.Refresh = func(scr *mint.Screen) {
		var rows []mint.Row
		rows = append(rows, mint.Action(mint.ActionOpts{
			Label: "Save current options…",
			Hint:  "save",
			OnActivate: func(c *mint.Ctx) {
				name, ok := c.Prompt("Save preset", "Name", "")
				if !ok || strings.TrimSpace(name) == "" {
					return
				}
				if a.SavePreset(name) {
					c.Toast("Saved " + strings.TrimSpace(name))
				}
			},
		}))
		list := a.Presets()
		if len(list) == 0 {
			rows = append(rows, mint.Note("  No presets saved yet."))
		}
		for _, p := range list {
			p := p
			rows = append(rows, mint.Action(mint.ActionOpts{
				Label: p.Name,
				Hint:  p.SavedAt.Format("15:04:05"),
				OnActivate: func(c *mint.Ctx) {
					c.Push(a.presetActions(p.Name))
				},
			}))
		}
		scr.Sections = []mint.Section{{Name: "Presets", Rows: rows}}
	}
	s.Refresh(s)
	return s
}

func (a *ToolApp) presetActions(name string) *mint.Screen {
	return &mint.Screen{
		Title: name,
		Sections: []mint.Section{{
			Name: "Preset",
			Rows: []mint.Row{
				mint.Action(mint.ActionOpts{
					Label: "Rerun " + name,
					Hint:  "run",
					OnActivate: func(c *mint.Ctx) {
						if !a.ApplyPreset(name) {
							c.ToastWarn("Preset was deleted")
							return
						}
						a.executeFromUI(c)
					},
				}),
				mint.Action(mint.ActionOpts{
					Label: "Delete " + name,
					Hint:  "delete",
					OnActivate: func(c *mint.Ctx) {
						if !c.Confirm("Delete preset?", "Delete "+name+"?") {
							return
						}
						if a.DeletePreset(name) {
							c.Toast("Deleted " + name)
							c.Pop()
						}
					},
				}),
			},
		}},
	}
}
