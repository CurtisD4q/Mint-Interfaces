package appkit

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestOriginalMixinsKeepTheirOrder(t *testing.T) {
	want := []MixinName{MixinFileOutput, MixinTableOutput, MixinCollapsibleOutput, MixinProfiles,
		MixinDynamicChoices, MixinRootAuth, MixinConfirm, MixinHistory}
	if got := AllMixins()[:8]; !reflect.DeepEqual(got, want) {
		t.Fatalf("first 8 mixins = %v, want %v", got, want)
	}
	for _, m := range MixinInfos() {
		if m.Summary == "" || m.Fields == "" {
			t.Errorf("%s has no summary or fields", m.Name)
		}
	}
}

// For the original 8, the registry agrees with the Mixins struct, for
// every archetype.
func TestEnabledMixinNamesMatchesMixinsStruct(t *testing.T) {
	original := map[MixinName]bool{}
	for _, n := range AllMixins()[:8] {
		original[n] = true
	}
	for _, def := range AllArchetypeDefs() {
		spec := ToolSpec{Name: "x"}
		def.ApplyTo(&spec)
		var names []MixinName
		for _, n := range spec.EnabledMixinNames() {
			if original[n] {
				names = append(names, n)
			}
		}
		if want := spec.EnabledMixins().Names(); !reflect.DeepEqual(names, want) {
			t.Errorf("%s: EnabledMixinNames %v, Mixins.Names %v", def.ID, names, want)
		}
	}
}

func TestRegisterMixinRejectsDuplicates(t *testing.T) {
	saved := mixinRegistry
	defer func() { mixinRegistry = saved }()
	mixinRegistry = append([]MixinInfo(nil), saved...)

	mustPanic := func(name string, m MixinInfo) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s: no panic", name)
			}
		}()
		registerMixin(m)
	}
	on := func(*ToolSpec) bool { return true }
	mustPanic("same name", MixinInfo{Name: MixinHistory, Order: 999, Enabled: on})
	mustPanic("same order", MixinInfo{Name: "WithNew", Order: 3, Enabled: on})
	mustPanic("no Enabled", MixinInfo{Name: "WithNew", Order: 998})
}

func TestValidateRunsEnabledMixinsOnly(t *testing.T) {
	saved := mixinRegistry
	defer func() { mixinRegistry = saved }()
	mixinRegistry = append([]MixinInfo(nil), saved...)

	registerMixin(MixinInfo{Name: "WithTest", Order: 997, Summary: "s", Fields: "Desc",
		Enabled:  func(s *ToolSpec) bool { return strings.HasPrefix(s.Desc, "test") },
		Validate: func(s *ToolSpec) error { return errors.New("bad " + s.Desc) }})

	if err := (ToolSpec{Name: "x", Desc: "other"}).Validate(); err != nil {
		t.Fatalf("disabled mixin validated: %v", err)
	}
	err := ToolSpec{Name: "x", Desc: "test one"}.Validate()
	if err == nil || err.Error() != "WithTest: bad test one" {
		t.Fatalf("Validate = %v", err)
	}
	if got := (ToolSpec{Name: "x", Desc: "test", KeepHistory: true}).EnabledMixinNames(); !reflect.DeepEqual(got, []MixinName{MixinHistory, "WithTest"}) {
		t.Fatalf("EnabledMixinNames = %v", got)
	}
}
