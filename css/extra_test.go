package css

// Additional tests targeting branches not covered by the upstream test tables,
// added to protect the optimization work on this codebase.

import (
	"bytes"
	"strings"
	"testing"

	"cssmash"
	"github.com/tdewolff/test"
)

func TestCSSExtra(t *testing.T) {
	cssTests := []struct {
		css      string
		expected string
	}{
		// a function left unclosed at EOF: the parser treats the } as closing
		// the function, so the minifier must too, or its synthesized ) makes
		// the output unparseable (regression from fuzzing)
		{"A{flex:0 0 a(000000000000000000}", "a{flex:0 0 a(0)}"},
		// an unterminated important comment must not gain a terminator made
		// out of its own last two bytes
		{"/*!* /0", "/*!* /0"},
		{"/*!ab*/", "/*!ab*/"},
		// a trailing backslash must not escape the punctuation written after
		// it: the declaration is malformed and dropped, while a selector or
		// at-rule prelude keeps a separating space
		{"{\\\r:", "{}"},
		{"\\\r{A0:", "\\ {a0:}"},
		{"@A\\\n{0", "@a\\ {0}"},
		// a truncated UTF-8 sequence would otherwise absorb the brace as a
		// continuation byte
		{"\\\xd0\\{\\0{", "\\\xd0\\ {}"},
		// the IE font-name quoting must not wrap a function token: its
		// arguments and closing paren would land after the closing quote
		{"{font:0 -A(", "{font:0 -A()}"},
		{"a{font:12px -apple-system}", "a{font:12px '-apple-system'}"},

		// empty rules and at-rules
		{"a{}", ""},
		{"a{}b{c:d}", "b{c:d}"},
		{"@media screen{}", ""},
		{"@media screen{a{}}", "@media screen{}"}, // TODO(size): drop emptied at-rules
		{"@media screen{a{b:c}}", "@media screen{a{b:c}}"},
		{"@supports (display:flex){a{b:c}}", "@supports(display:flex){a{b:c}}"},

		// custom properties
		{"a{--custom:value}", "a{--custom:value}"},
		{"a{--custom:  spaced  value }", "a{--custom:spaced  value}"},
		{"a{--custom: }", "a{--custom: }"},
		{"a{--custom:var(--other)}", "a{--custom:var(--other)}"},

		// !important handling
		{"a{color:red!important}", "a{color:red!important}"},
		{"a{color:red ! important}", "a{color:red!important}"},

		// data URIs
		{"a{background:url(data:text/plain;base64,aGk=)}", "a{background:url(data:,hi)}"},
		{"a{background:url(data:image/png;base64,iVBORw0KGgo=)}", "a{background:url(data:image/png,%89PNG%0D%0A%1A%0A)}"},

		// unicode-range normalization and merging
		{"@font-face{unicode-range:U+00-FF}", "@font-face{unicode-range:U+??}"},
		{"@font-face{unicode-range:U+0000-00FF}", "@font-face{unicode-range:U+??}"},
		{"@font-face{unicode-range:U+0-7F,U+80-FF}", "@font-face{unicode-range:U+??}"},
		{"@font-face{unicode-range:U+1,U+2,U+3}", "@font-face{unicode-range:U+1-3}"},
		{"@font-face{unicode-range:U+1,U+2,U+4}", "@font-face{unicode-range:U+1-2,U+4}"},
		{"@font-face{unicode-range:U+5}", "@font-face{unicode-range:U+5}"},
		{"@font-face{unicode-range:U+0-10FFFF}", "@font-face{unicode-range:initial}"},
		{"@font-face{unicode-range:U+100-200,U+150}", "@font-face{unicode-range:U+100-200}"},
		{"@font-face{unicode-range:U+??}", "@font-face{unicode-range:U+??}"},
		{"@font-face{unicode-range:u+1a2}", "@font-face{unicode-range:U+1A2}"},

		// colors
		{"a{color:#0000ffff}", "a{color:#00f}"},
		{"a{color:#ff000000}", "a{color:#f000}"},
		{"a{color:#11223344}", "a{color:#1234}"},
		{"a{color:#1234}", "a{color:#1234}"},
		{"a{color:RGB(255,0,0)}", "a{color:red}"},
		{"a{color:rgba(0,0,0,0)}", "a{color:transparent}"},
		{"a{color:hsla(0,0%,0%,.5)}", "a{color:#00000080}"},
		{"a{color:hsl(120 50% 50%)}", "a{color:#40bf40}"},
		{"a{color:rgb(255 0 0 / 50%)}", "a{color:#ff000080}"},
		{"a{color:rgb(51 102 153)}", "a{color:#369}"},
		{"a{color:rgb(20% 40% 60%)}", "a{color:#369}"},

		// numbers and dimensions
		{"a{margin:0.5em}", "a{margin:.5em}"},
		{"a{margin:+0.5em}", "a{margin:.5em}"},
		{"a{margin:0px}", "a{margin:0}"},
		{"a{margin:0em}", "a{margin:0}"},
		{"a{flex:0 0px}", "a{flex:0}"},
		{"a{z-index:05}", "a{z-index:05}"},   // TODO(size): integer properties skip Number() entirely
		{"a{z-index:1.0}", "a{z-index:1.0}"}, // TODO(size): same
		{"a{width:00010px}", "a{width:10px}"},
		{"a{margin:1e2px}", "a{margin:100px}"},
		{"a{transition:all 500ms}", "a{transition:all 500ms}"}, // TODO(size): unit conversion (500ms=>.5s) disabled in baseline

		// strings in content
		{"a{content:\"str\"}", "a{content:\"str\"}"},
		{"a{content:'str'}", "a{content:'str'}"},

		// selectors
		{"A:hover{b:c}", "a:hover{b:c}"},
		{"[ID=x]{b:c}", "[ID=x]{b:c}"},
		{".c1.c2{b:c}", ".c1.c2{b:c}"},
		{"a > b{c:d}", "a>b{c:d}"},
		{"a ~ b{c:d}", "a~b{c:d}"},
		{"a + b{c:d}", "a+b{c:d}"},
		{"a  b{c:d}", "a b{c:d}"},

		// @import variations
		{`@import url( "file" );`, `@import "file";`},
		{`@import "file" screen;`, `@import "file" screen;`},

		// font
		{"a{font:normal normal normal 16px/normal Arial}", "a{font:16px Arial}"},
		{"a{font:bold 16px Arial}", "a{font:700 16px Arial}"},
		{"a{font:16px/1.5 Arial}", "a{font:16px/1.5 Arial}"},
		{"a{font:italic small-caps 700 16px/2 \"Times New Roman\",serif}", "a{font:italic small-caps 700 16px/2 times new roman,serif}"},

		// background
		{"a{background:url(x) center center}", "a{background:url(x)50%}"},
		{"a{background:scroll none transparent}", "a{background:0 0}"},
		{"a{background:repeat repeat}", "a{background:0 0}"},
		{"a{background:10px 20px/30px 40px}", "a{background:10px 20px/30px 40px}"},
		{"a{background-position:right 20% bottom 10%}", "a{background-position:80% 90%}"},
		{"a{background-position:left 20% top 10%}", "a{background-position:20% 10%}"},
		{"a{background-position:right 20px bottom 10px}", "a{background-position:right 20px bottom 10px}"},

		// border and outline
		{"a{border:none}", "a{border:none}"},
		{"a{border:1px none red}", "a{border:1px red}"},
		{"a{outline:invert}", "a{outline:none}"},
		{"a{border-color:red red red red}", "a{border-color:red}"},
		{"a{border-color:red blue red blue}", "a{border-color:red blue red blue}"}, // TODO(size): fold 4 values to 2 like margin
		{"a{border-color:currentcolor}", "a{border-color:initial}"},

		// box-shadow and text-shadow
		{"a{box-shadow:1px 2px 3px 0}", "a{box-shadow:1px 2px 3px}"},
		{"a{box-shadow:1px 2px 0 0}", "a{box-shadow:1px 2px}"},
		{"a{box-shadow:initial}", "a{box-shadow:none}"},
		{"a{text-shadow:red 1px 2px}", "a{text-shadow:red 1px 2px}"},

		// flex
		{"a{flex:1 1 auto}", "a{flex:auto}"},
		{"a{flex:0 1 auto}", "a{flex:initial}"},
		{"a{flex:1 1 0}", "a{flex:1}"},
		{"a{flex:0 0 auto}", "a{flex:none}"},
		{"a{flex:2 1 0%}", "a{flex:2}"},
		{"a{flex:2 3 5px}", "a{flex:2 3 5px}"},
		{"a{flex-grow:initial}", "a{flex-grow:0}"},
		{"a{flex-shrink:initial}", "a{flex-shrink:1}"},
		{"a{flex-basis:initial}", "a{flex-basis:auto}"},
		{"a{order:initial}", "a{order:0}"},

		// text-decoration / emphasis / column-rule
		{"a{text-decoration:none solid currentcolor}", "a{text-decoration:none}"},
		{"a{text-emphasis:none}", "a{text-emphasis:none}"},
		{"a{column-rule:medium none currentcolor}", "a{column-rule:none}"},

		// ms-filter
		{`a{-ms-filter:"progid:DXImageTransform.Microsoft.Alpha(Opacity=50)"}`, `a{-ms-filter:"alpha(opacity=50)"}`},
		{`a{filter:progid:DXImageTransform.Microsoft.Alpha(Opacity=50)}`, `a{filter:alpha(opacity=50)}`},

		// errors pass through
		{"a{b:}", "a{b:}"},
		{"a{", ""},
	}

	m := cssmash.New()
	for _, tt := range cssTests {
		t.Run(tt.css, func(t *testing.T) {
			r := bytes.NewBufferString(tt.css)
			w := &bytes.Buffer{}
			err := Minify(m, w, r, nil)
			test.Minify(t, tt.css, err, w.String(), tt.expected)
		})
	}
}

func TestCSSExtraVersion2(t *testing.T) {
	cssTests := []struct {
		css      string
		expected string
	}{
		// CSS2: never produces exponents, space before !important
		{"a{margin:100.0px}", "a{margin:100px}"},
		{"a{margin:1e2px}", "a{margin:1e2px}"}, // input exponents are left as-is
		{"a{color:red!important}", "a{color:red !important}"},
		{"a{margin:.50em}", "a{margin:.5em}"},
	}

	m := cssmash.New()
	o := Minifier{Version: 2}
	for _, tt := range cssTests {
		t.Run(tt.css, func(t *testing.T) {
			r := bytes.NewBufferString(tt.css)
			w := &bytes.Buffer{}
			err := o.Minify(m, w, r, nil)
			test.Minify(t, tt.css, err, w.String(), tt.expected)
		})
	}
}

func TestCSSDeepNesting(t *testing.T) {
	// exceeds the tokensLevel recursion limit of 100, which skips deep processing
	depth := 105
	var sb strings.Builder
	sb.WriteString("a{margin:")
	for i := 0; i < depth; i++ {
		sb.WriteString("calc(")
	}
	sb.WriteString("1px")
	for i := 0; i < depth; i++ {
		sb.WriteString(")")
	}
	sb.WriteString("}")
	in := sb.String()

	m := cssmash.New()
	w := &bytes.Buffer{}
	if err := Minify(m, w, strings.NewReader(in), nil); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(w.String(), "a{margin:calc(") {
		t.Errorf("unexpected output: %s", w.String()[:60])
	}
}

func TestHashStringBytes(t *testing.T) {
	h := ToHash([]byte("margin"))
	if h == 0 {
		t.Fatal("expected nonzero hash for margin")
	}
	if s := h.String(); s != "margin" {
		t.Errorf("String() = %q, want margin", s)
	}
	if b := h.Bytes(); !bytes.Equal(b, []byte("margin")) {
		t.Errorf("Bytes() = %q, want margin", b)
	}
	if ToHash([]byte("unknownpropertyname")) != 0 {
		t.Error("expected zero hash for unknown property")
	}
}

func TestCSSIdempotent(t *testing.T) {
	inputs := []string{
		`@media screen{.a{margin:1px 1px;color:#ffffff;background:url(x) center}}`,
		`@font-face{font-family:'x';src:url(y.woff2);unicode-range:U+0-7F}`,
		`a{flex:1 1 auto;transition:opacity .3s ease}`,
	}
	m := cssmash.New()
	for _, cssInput := range inputs {
		w := &bytes.Buffer{}
		if err := Minify(m, w, strings.NewReader(cssInput), nil); err != nil {
			t.Fatal(err)
		}
		first := w.String()
		w2 := &bytes.Buffer{}
		if err := Minify(m, w2, strings.NewReader(first), nil); err != nil {
			t.Fatal(err)
		}
		if w2.String() != first {
			t.Errorf("not idempotent for %q:\nfirst:  %q\nsecond: %q", cssInput, first, w2.String())
		}
	}
}
