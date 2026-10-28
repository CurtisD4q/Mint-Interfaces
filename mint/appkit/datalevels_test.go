package appkit

import "testing"

// formerTemplateLevels are the data levels the old templates gave their
// tools, taken just before the templates were removed. Moving to
// archetypes must never lower one.
var formerTemplateLevels = map[string]string{
	"age": DataSecret, "amass": DataSensitive, "ausearch": DataSensitive, "base64": DataSecret,
	"checksec": DataSensitive, "chkrootkit": DataSensitive, "clamscan": DataSensitive, "conntrack": DataSensitive,
	"dig": DataSensitive, "dmesg": DataSensitive, "dnsrecon": DataSensitive, "drill": DataSensitive,
	"dumpcap": DataSensitive, "eslint": DataSensitive, "faillock": DataSensitive, "ffuf": DataSensitive,
	"gobuster": DataSensitive, "gpg": DataSecret, "grype": DataSensitive, "hashcat": DataSecret,
	"host": DataSensitive, "john": DataSecret, "journalctl": DataSensitive, "last": DataSensitive,
	"lastlog": DataSensitive, "lynis": DataSensitive, "mtr": DataSensitive, "ngrep": DataSensitive,
	"nikto": DataSensitive, "nslookup": DataSensitive, "nuclei": DataSensitive, "openssl": DataSecret,
	"rkhunter": DataSensitive, "shellcheck": DataSensitive, "sqlite3": DataSensitive, "sqlmap": DataSensitive,
	"subfinder": DataSensitive, "tail": DataSensitive, "tcpdump": DataSensitive, "theHarvester": DataSensitive,
	"theharvester": DataSensitive, "tracepath": DataSensitive, "traceroute": DataSensitive, "trivy": DataSensitive,
	"tshark": DataSensitive, "whois": DataSensitive, "wpscan": DataSensitive,
}

func TestFormerTemplateLevelsKept(t *testing.T) {
	if len(formerTemplateLevels) < 40 {
		t.Fatalf("only %d tools", len(formerTemplateLevels))
	}
	for name, want := range formerTemplateLevels {
		spec := ToolSpec{Name: name}
		if a, ok := ArchetypeForTool(name); ok {
			spec.ArchetypeID = string(a.ID)
		}
		if got, _ := ClassifyData(spec); dataRank(got) < dataRank(want) {
			t.Errorf("%s: %q, was %q", name, got, want)
		}
		// Through New, as an app starts.
		app := New(ToolSpec{Name: name, Bin: name})
		if dataRank(app.Spec.Data) < dataRank(want) {
			t.Errorf("%s (app): %q, was %q", name, app.Spec.Data, want)
		}
	}
	// And every old template's own level, for apps that still name one.
	for _, lt := range legacyTemplates {
		if lt.data == "" {
			continue
		}
		for _, key := range []string{lt.id, lt.slug} {
			if got, _ := ClassifyData(ToolSpec{Name: "old9", Template: key}); dataRank(got) < dataRank(lt.data) {
				t.Errorf("%s: %q, was %q", key, got, lt.data)
			}
		}
	}
}
