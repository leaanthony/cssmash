package js

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"cssmash"
	"cssmash/parse"
	"cssmash/parse/buffer"
)

func loadJSFixtures(tb testing.TB) map[string][]byte {
	tb.Helper()
	files, err := filepath.Glob("../testdata/js/*.js")
	if err != nil || len(files) == 0 {
		tb.Fatalf("no JS fixtures found: %v", err)
	}
	sort.Strings(files)
	fixtures := make(map[string][]byte, len(files))
	for _, f := range files {
		buf, err := os.ReadFile(f)
		if err != nil {
			tb.Fatal(err)
		}
		fixtures[filepath.Base(f)] = buf
	}
	return fixtures
}

// TestJSOutputSize pins the minified output size per fixture. An increase
// fails the test; an improvement fails with a message to update the golden
// values so gains are recorded deliberately.
func TestJSOutputSize(t *testing.T) {
	golden := map[string]int{
		"ace.js":               336991,
		"dot.js":               3303,
		"jquery-ui.js":         248663,
		"jquery.dataTables.js": 79208,
		"jquery.js":            84437,
		"moment.js":            34279,
	}

	m := cssmash.New()
	for name, buf := range loadJSFixtures(t) {
		w := buffer.NewWriter(make([]byte, 0, len(buf)))
		if err := Minify(m, w, buffer.NewReader(parse.Copy(buf)), nil); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		n := len(w.Bytes())
		want := golden[name]
		if n > want {
			t.Errorf("%s: output grew from %d to %d bytes", name, want, n)
		} else if n < want {
			t.Errorf("%s: output shrank from %d to %d bytes; update golden value in TestJSOutputSize", name, want, n)
		}
	}
}

func BenchmarkJS(b *testing.B) {
	m := cssmash.New()
	for name, buf := range loadJSFixtures(b) {
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(buf)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				w := buffer.NewWriter(make([]byte, 0, len(buf)))
				if err := Minify(m, w, buffer.NewReader(parse.Copy(buf)), nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkJSWriter(b *testing.B) {
	// measures the common io.Writer interface path with a bytes.Buffer
	buf := loadJSFixtures(b)["jquery.js"]
	m := cssmash.New()
	b.SetBytes(int64(len(buf)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := &bytes.Buffer{}
		if err := Minify(m, w, buffer.NewReader(parse.Copy(buf)), nil); err != nil {
			b.Fatal(err)
		}
	}
}
