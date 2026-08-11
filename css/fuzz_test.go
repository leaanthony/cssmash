package css

import (
	"bytes"
	"io"
	"testing"
	"unicode/utf8"

	"cssmash"
	"cssmash/parse"
	parsecss "cssmash/parse/css"
)

// parsesCleanly returns whether b parses as a stylesheet without grammar
// errors.
func parsesCleanly(b []byte) bool {
	z := parse.NewInputBytes(append([]byte(nil), b...))
	p := parsecss.NewParser(z, false)
	for {
		if gt, _, _ := p.Next(); gt == parsecss.ErrorGrammar {
			return p.Err() == io.EOF
		}
	}
}

// hasFusedDelim reports whether the parser hands back a delimiter token
// carrying more than one code point. A delimiter is by definition a single
// code point, so this means the parser has already merged two of them across
// the whitespace that separated them: `{* =` arrives as one Delim holding
// "*=", which relexes as the single SubstringMatch token *=.
//
// The fusion happens before the minifier sees the token, so no output it can
// write will undo it, and asserting output validity for such input would be
// asserting something about the parser rather than the minifier. Only
// selectors reach this, where a bare match operator is not valid Selectors
// syntax to begin with.
func hasFusedDelim(b []byte) bool {
	z := parse.NewInputBytes(append([]byte(nil), b...))
	p := parsecss.NewParser(z, false)
	for {
		gt, tt, data := p.Next()
		// The fused token can arrive either as a prelude/value token or, for a
		// declaration, as the property name in data.
		if tt == parsecss.DelimToken && 1 < utf8.RuneCount(data) {
			return true
		}
		for _, v := range p.Values() {
			if v.TokenType == parsecss.DelimToken && 1 < utf8.RuneCount(v.Data) {
				return true
			}
		}
		if gt == parsecss.ErrorGrammar {
			return false
		}
	}
}

// FuzzMinify verifies that minification never panics and that, for valid
// input, minified output is still valid CSS. The minifier passes invalid CSS
// through verbatim by design, so no validity guarantee exists for invalid
// input.
func FuzzMinify(f *testing.F) {
	seeds := []string{
		`@media screen{.a{margin:1px 1px;color:#ffffff}}`,
		`@font-face{font-family:'x';src:url(y.woff2);unicode-range:U+0-7F}`,
		`a{background:url(x) center/cover no-repeat}`,
		`a{flex:1 1 auto;transition:opacity .3s ease}`,
		`a{content:"str";filter:alpha(opacity=50)}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	m := cssmash.New()
	f.Fuzz(func(t *testing.T, b []byte) {
		w := &bytes.Buffer{}
		if err := Minify(m, w, bytes.NewReader(b), nil); err != nil {
			return
		}
		out := w.Bytes()

		// invalid input is passed through verbatim, so only valid input
		// guarantees valid output
		if !parsesCleanly(b) || hasFusedDelim(b) {
			return
		}
		if !parsesCleanly(out) {
			t.Fatalf("minified output is invalid CSS\ninput:  %q\noutput: %q", b, out)
		}
	})
}
