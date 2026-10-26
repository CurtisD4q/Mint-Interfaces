package appkit

import (
	"strings"
	"testing"
)

func BenchmarkRedact(b *testing.B) {
	m := newOutputMasker()
	m.add([]byte("hunter22pw"))
	m.add([]byte("an-api-token-value"))
	line := strings.Repeat("12:00:01.123 IP 10.0.0.1.443 > 10.0.0.2.51234: Flags [P.], seq 1:100 ", 3)
	b.SetBytes(int64(len(line)))
	for i := 0; i < b.N; i++ {
		m.Redact(line)
	}
}
