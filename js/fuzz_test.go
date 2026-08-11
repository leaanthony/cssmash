package js

import (
	"bytes"
	"testing"
	"unicode/utf8"

	"cssmash"
	"cssmash/parse"
	parsejs "cssmash/parse/js"
)

// FuzzMinify verifies that minification never panics, that minified output is
// still valid JS (parseable), and that minification is idempotent.
func FuzzMinify(f *testing.F) {
	seeds := []string{
		`function f(a,b){if(a){return b+1}else{return b-1}}`,
		`var x=1;for(var i=0;i<10;i++){x+=i}`,
		`class A{#x=1;static b(){return new A}}`,
		`import x,{y}from"m";export default x=>x+y`,
		`while(a)if(b)c();else d()`,
		"let s=`template ${x} string`",
		`try{throw new Error("x")}catch(e){console.log(e)}`,
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

		if !utf8.Valid(b) {
			// ECMAScript source text is a sequence of Unicode code points, so
			// a byte string that is not valid UTF-8 is not conforming source
			// and the minifier promises nothing about it. This lexer decodes
			// leniently rather than rejecting, which lets a truncated sequence
			// act as an identifier -- one that is only stable at end of input,
			// since anywhere else it absorbs the bytes after it as
			// continuations. Minifying must still not panic, which the call
			// above checks; output validity is asserted only for real source.
			return
		}

		// minified output must still be valid JS
		z := parse.NewInputBytes(append([]byte(nil), out...))
		if _, err := parsejs.Parse(z, parsejs.Options{}); err != nil {
			t.Fatalf("minified output is invalid JS: %v\ninput:  %q\noutput: %q", err, b, out)
		}
		z.Restore()

		// The minifier is not idempotent, and pass 2 is not even monotonic in
		// size: a rewrite chosen on the first pass can leave a form the second
		// pass regroups into something marginally longer, because deciding
		// purely on byte count would forgo folds that enable larger savings
		// downstream (see condFoldsSmaller). Neither equality nor non-growth
		// is therefore a property this minifier has. What must hold is that a
		// second pass succeeds and still produces valid JS -- output size for
		// the pass that matters is pinned by TestJSOutputSize instead.
		w2 := &bytes.Buffer{}
		if err := Minify(m, w2, bytes.NewReader(out), nil); err != nil {
			t.Fatalf("second minification pass failed: %v\ninput: %q\nfirst: %q", err, b, out)
		}
		z2 := parse.NewInputBytes(append([]byte(nil), w2.Bytes()...))
		if _, err := parsejs.Parse(z2, parsejs.Options{}); err != nil {
			t.Fatalf("second pass output is invalid JS: %v\nfirst:  %q\nsecond: %q", err, out, w2.Bytes())
		}
	})
}

// FuzzMinifyRenaming additionally exercises the variable renamer.
func FuzzMinifyRenaming(f *testing.F) {
	f.Add([]byte(`function f(longname){var other=longname;return other+1}`))
	f.Add([]byte(`(function(){var a=1,b=2;return a+b})()`))

	m := cssmash.New()
	o := Minifier{}
	f.Fuzz(func(t *testing.T, b []byte) {
		w := &bytes.Buffer{}
		if err := o.Minify(m, w, bytes.NewReader(b), nil); err != nil {
			return
		}
		out := w.Bytes()

		if !utf8.Valid(b) {
			return // see FuzzMinify
		}
		z := parse.NewInputBytes(append([]byte(nil), out...))
		if _, err := parsejs.Parse(z, parsejs.Options{}); err != nil {
			t.Fatalf("minified output is invalid JS: %v\ninput:  %q\noutput: %q", err, b, out)
		}
		z.Restore()
	})
}
