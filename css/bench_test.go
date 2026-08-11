package css

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

func loadCSSFixtures(tb testing.TB) map[string][]byte {
	tb.Helper()
	files, err := filepath.Glob("../testdata/css/*.css")
	if err != nil || len(files) == 0 {
		tb.Fatalf("no CSS fixtures found: %v", err)
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

// TestCSSOutputSize pins the minified output size per fixture. An increase
// fails the test; an improvement fails with a message to update the golden
// values so gains are recorded deliberately.
func TestCSSOutputSize(t *testing.T) {
	golden := map[string]int{
		"1.css": 112753,
		"2.css": 371,
		"3.css": 350,
		"4.css": 1657,
		"5.css": 23668,
		"6.css": 773,
		"7.css": 2568,
	}

	m := cssmash.New()
	for name, buf := range loadCSSFixtures(t) {
		w := buffer.NewWriter(make([]byte, 0, len(buf)))
		if err := Minify(m, w, buffer.NewReader(parse.Copy(buf)), nil); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		n := len(w.Bytes())
		want := golden[name]
		if n > want {
			t.Errorf("%s: output grew from %d to %d bytes", name, want, n)
		} else if n < want {
			t.Errorf("%s: output shrank from %d to %d bytes; update golden value in TestCSSOutputSize", name, want, n)
		}
	}
}

func BenchmarkCSS(b *testing.B) {
	m := cssmash.New()
	for name, buf := range loadCSSFixtures(b) {
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

func BenchmarkCSSWriter(b *testing.B) {
	// measures the common io.Writer interface path with a bytes.Buffer
	buf := loadCSSFixtures(b)["1.css"]
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
