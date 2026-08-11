package js

// Additional tests targeting branches not covered by the upstream test tables,
// added to protect the optimization work on this codebase.

import (
	"bytes"
	"strings"
	"testing"

	"cssmash"
	"github.com/tdewolff/test"
)

func TestJSExtra(t *testing.T) {
	jsTests := []struct {
		js       string
		expected string
	}{
		// hasSideEffects via void and boolean short-circuit folding
		{`void 0`, `0[0]`},
		{`void x`, `void x`},
		{`void f()`, `void f()`},
		{`void new A`, `void new A`},
		{`void a.b`, `void a.b`},
		{`void a[0]`, `void a[0]`},
		{`void(a?b:c)`, `void(a?b:c)`},
		{`void(a=b)`, `void(a=b)`},
		{`void(a,b)`, `void(a,b)`},
		{`void[]`, `0[0]`},
		{`void{}`, `0[0]`},
		{`void[a,b]`, `void[a,b]`},
		{`void{a:b}`, `void{a:b}`},
		{`void{[a]:b}`, `void{[a]:b}`},
		{`x&&!0`, `x`},
		{`!0&&x`, `x`},
		{`!1||x`, `x`},
		{`x||!1`, `x`},
		{`!1&&x`, `!1&&x`},
		{`x&&!1`, `x&&!1`},
		{`""&&x`, `""&&x`},
		{`x&&""`, `x&&""`},
		{`""||x`, `""||x`},
		{`x||""`, `x||""`},

		// if/else statement optimization
		{`if(a)b();else c()`, `a?b():c()`},
		{`if(!a)b();else c()`, `a?c():b()`},
		{`if(a)b()`, `a&&b()`},
		{`if(!a)b()`, `a||b()`},
		{`if(a);else b()`, `a||b()`},
		{`if(a);`, `a`},
		{`if(!a);`, `!a`},
		{`if(1)a`, `a`},
		{`if(0)a`, ``},
		{`if(0)a;else b`, `b`},
		{`if("")a;else b`, `b`},
		{`if("x")a;else b`, `a`},
		// a string of only line continuations is empty and falsy; strings with
		// real escapes are not
		{"if(\"\\\n\")a;else b", `b`},
		{"if(\"\\\r\n\")a;else b", `b`},
		{"if(\"\\\n\\\n\")a;else b", `b`},
		{"if(\"\\n\")a;else b", `a`},
		{"if(\"\\0\")a;else b", `a`},
		{"if(\"\\\nx\")a;else b", `a`},
		{`if(0)a()`, ``},
		{`function f(){if(a)return b;else return c}`, `function f(){return a?b:c}`},
		{`function f(){if(a)return;else return}`, `function f(){a}`},
		{`function f(){if(a)throw b;else throw c}`, `function f(){throw a?b:c}`},
		{`if(a){}else{}`, `a`},
		{`if(a)if(b)c`, `a&&b&&c`},
		{`x;if(a)break;else y`, `if(x,a)break;y`},

		// merge if/return sequences
		{`function f(){if(a)return b;return c}`, `function f(){return a?b:c}`},
		{`function f(){if(a)return;return b}`, `function f(){if(!a)return b}`},
		{`function f(){if(a)throw b;throw c}`, `function f(){throw a?b:c}`},
		{`function f(){if(a);return}`, `function f(){a}`},

		// minifyStmtOrBlock expansion (single stmt becoming two or zero)
		{`while(x)if(a)continue;else b()`, `for(;x;)a||b()`},
		{`while(x)if(1);`, `for(;x;);`},
		{`do if(1);while(x)`, `do;while(x)`},
		{`with(a)if(b)c`, `with(a)b&&c`},
		{`label:if(a)break label;else b()`, `label:{if(a)break label;b()}`},

		// empty and flow statements
		{`;`, ``},
		{`{}`, ``},
		{`{{}}`, ``},
		{`function f(){return}`, `function f(){}`},
		{`function f(){return undefined}`, `function f(){}`},
		{`function f(){return void 0}`, `function f(){}`},
		{`function f(){a;return void 0}`, `function f(){a}`},
		{`function f(){a,b;return void 0}`, `function f(){return a,b}`},
		{`while(a)continue`, `for(;a;);`},
		{`while(a)continue;`, `for(;a;);`},

		// var hoisting and merging
		{`var a;a=5`, `var a=5`},
		{`var a;var b;a,b`, `var a,b;a,b`},
		{`var a=1;a=2`, `var a=1,a=2`},
		{`var a,b;a=1,b=2`, `var a=1,b=2`},
		{`var a;for(a=0;;);`, `var a;for(a=0;;);`},
		{`var a;for(;;);`, `for(var a;;);`},
		{`a();while(b);`, `for(a();b;);`},
		{`var a;while(b);`, `for(var a;b;);`},
		{`a();if(b)c`, `a(),b&&c`},
		{`a();switch(b){}`, `switch(a(),b){}`},
		{`a();with(b);`, `with(a(),b);`},
		{`var a=1;var b=2,c=3;a,b,c`, `var a=1,b=2,c=3;a,b,c`},

		// assignments to enclosing-scope variables must never be merged into a
		// local var declaration: that would re-declare them locally and shadow
		// the outer binding (regression for unsafe Var.Link following)
		{`var a;function f(){var t=1;a=2;return t}`, `var a;function f(){var t=1;return a=2,t}`},
		{`var l=1,g=function(){var s=l;l=2;return s};g()`, `var l=1,g=function(){var s=l;return l=2,s};g()`},
		{`function o(){var a;function f(){var t=1;a=2;return t}return f(),a}`, `function o(){var a;function f(){var t=1;return a=2,t}return f(),a}`},

		// optimizeVarOrder for let/const (array/object first)
		{`let a=1,[b]=c;a,b`, `let[b]=c,a=1;a,b`},
		{`let a=1,[b]=a;a,b`, `let a=1,[b]=a;a,b`},
		{`let a=1,{b}=c;a,b`, `let{b}=c,a=1;a,b`},
		{`let a=1,[b]=f();a,b`, `let[b]=f(),a=1;a,b`},
		{`let a=1,[b]=x?y:z;a,b`, `let[b]=x?y:z,a=1;a,b`},
		{`let a=1,[b]=class extends a{};a,b`, `let a=1,[b]=class extends a{};a,b`},
		{`let a=1,[b]=[a];a,b`, `let a=1,[b]=[a];a,b`},
		{`var b,a,c;b,a,c`, `var a,b,c;b,a,c`},

		// unused lexical declarations in blocks (bindingUsed)
		{`{let a=1}`, ``},
		{`{let a=f()}`, `f()`},
		{`{let a=f(),b=g()}`, `f(),g()`},
		{`{let a=1;b=a}`, `{let a=1;b=a}`},
		{`{const a=1}`, ``},
		{`{let [a]=x}`, `x`},
		{`{let {a}=x}`, `x`},
		{`{let {a,...b}=x}`, `x`},
		{`{let [...a]=x}`, `x`},
		{`{let [a]=x;b=a}`, `{let[a]=x;b=a}`},
		{`{class a{}}`, ``},

		// strings, templates, regexps
		{`"a" + "b"`, `"ab"`},
		{`"a" + b + "c" + "d"`, `"a"+b+"cd"`},
		{`'a' + 'b' + c`, `"ab"+c`},
		{"`a\nb`", "`a\nb`"},
		{`tag` + "`a${b}c`", "tag`a${b}c`"},
		{`/[\-]/`, `/[-]/`},
		{`/[a\-z]/`, `/[a\-z]/`},
		{`/\./`, `/\./`},
		{`/a/;`, `/a/`},

		// numbers
		{`0x10`, `16`},
		{`0b1_0`, `2`},
		{`0o1_0`, `8`},
		{`0xFFFFFFFFFFFF`, `0xFFFFFFFFFFFF`},
		{`a["123"]`, `a[123]`},
		{`a["b"]`, `a.b`},
		{`a?.["b"]`, `a?.b`},
		{`(5).toString`, `5..toString`},
		// an empty export clause is what marks a file as a module, so it
		// cannot be dropped down to a bare (and invalid) `export`
		{`export{}`, `export{}`},
		// a parenthesized comma expression flattens as an operand, but not as
		// an assignment target: a,b=c regroups as a,(b=c)
		{`(a,b)&&c`, `a,b&&c`},
		{`x=(a,b)&&c`, `x=(a,b)&&c`},
		{`(a,b)=c`, `(a,b)=c`},
		// logical assignment binds looser than ||, so a?a:b??=c must not
		// become a||b??=c (these were missing from the precedence maps, which
		// made exprPrec report the loosest possible binding for them)
		{`a?a:b??=c`, `a?a:b??=c`},
		{`a?a:b||=c`, `a?a:b||=c`},
		{`a?a:b&&=c`, `a?a:b&&=c`},
		{`a?b??=c:a`, `a?b??=c:a`},
		{`a??=b`, `a??=b`},
		// a decimal literal that normalizes to a bare integer still needs the
		// second dot, and forms that cannot take a decimal point do not
		{`(0.).A`, `0..A`},
		{`(1.0).A`, `1..A`},
		{`(0x10).A`, `16..A`}, // folded to decimal first, so it needs the second dot
		{`(1e6).A`, `1e6.A`},
		// numeric separators survive normalization; .0_0 once became .0_
		{`(.0_0).A`, `0..A`},
		{`(1_0).A`, `1_0..A`},
		{`(5.5).toString`, `5.5.toString`},
		{`1..toString()`, `1..toString()`},

		// folds: Number, Math
		{`Number(!0)`, `Number(!0)`},
		{`Number(!1)`, `Number(!1)`},
		{`Number(null)`, `0`},
		{`Number(5.5)`, `5.5`},
		{`Number(0x10)`, `16`},
		{`Number(undefined)`, `NaN`},
		{`Math.pow(a,b)`, `a**b`},
		{`Math.pow(a,b)+c`, `a**b+c`},
		{`Math.trunc(a)`, `a|0`},
		{`Math.trunc(a)+b`, `(a|0)+b`},
		{`Math.sqrt(a)`, `a**.5`},
		{`Math.sqrt(a)+b`, `a**.5+b`},
		{`Math.abs(ab)`, `ab<0?-ab:ab`},
		{`Math.abs(abcdefgh)`, `Math.abs(abcdefgh)`},

		// folds: equality, increments
		{`a=a+1`, `++a`},
		{`a=a-1`, `--a`},
		{`a=1+a`, `++a`},
		{`a=b+1`, `a=b+1`},
		{`a===null||a===undefined`, `a==null`},
		{`a!==null&&a!==undefined`, `a!=null`},
		{`a==null||a==undefined`, `a==null`},
		{`typeof a==="string"`, `typeof a=="string"`},
		{`"string"===typeof a`, `"string"==typeof a`},
		{`typeof a!=="string"`, `typeof a!="string"`},
		{`!!a`, `!!a`},
		{`!(!a)`, `!!a`},
		{`!(a==b)`, `a!=b`},
		{`!(a===b)`, `a!==b`},
		{`!(a&&b)`, `!a||!b`},
		{`!(a==0||b==0)`, `a!=0&&b!=0`},
		{`a==0?b:c`, `a==0?b:c`},
		{`a?a:b`, `a||b`},
		{`a?b:a`, `a&&b`},
		{`a?b:b`, `a,b`},
		{`a==null?b:a`, `a??b`},
		{`a==null?undefined:a.b`, `a?.b`},
		{`a==null?undefined:a.b()`, `a?.b()`},
		{`f(a)?g(a):h(a)`, `f(a)?g(a):h(a)`},
		{`x?a(1):a(2)`, `a(x?1:2)`},
		{`x?f(a):f(b)`, `f(x?a:b)`},

		// arrow functions
		{`(a)=>a`, `a=>a`},
		{`async (a)=>a`, `async a=>a`},
		{`(a=1)=>a`, `(a=1)=>a`},
		{`([a])=>a`, `([a])=>a`},
		{`()=>{a();return b}`, `()=>(a(),b)`},
		{`()=>{a();b();return c}`, `()=>(a(),b(),c)`},
		{`()=>{if(a)b;return c}`, `()=>(a&&b,c)`},
		{`()=>{a();return}`, `()=>{a()}`},
		{`()=>{return a()}`, `()=>a()`},
		{`(a,b)=>{return a+b}`, `(a,b)=>a+b`},

		// functions and classes
		{`function f(){new.target}`, `function f(){new.target}`},
		{`import.meta.url`, `import.meta.url`},
		{`function*f(){yield}`, `function*f(){yield}`},
		{`function*f(){yield undefined}`, `function*f(){yield}`},
		{`function*f(){yield* a}`, `function*f(){yield*a}`},
		{`async function f(){for await(a of b);}`, `async function f(){for await(a of b);}`},
		{`class a{static{b}}`, `class a{static{b}}`},
		{`class a{[b]=1}`, `class a{[b]=1}`},
		{`class a{static[b]=1}`, `class a{static[b]=1}`},
		{`class a{static b=1}`, `class a{static b=1}`},
		{`class a{b}`, `class a{b}`},
		{`class a{get b(){}}`, `class a{get b(){}}`},
		{`class a{static async *b(){}}`, `class a{static async*b(){}}`},
		{`switch(a){}`, `switch(a){}`},
		{`try{}finally{a}`, `try{}finally{a}`},
		{`try{}catch(a){b}finally{c}`, `try{}catch{b}finally{c}`},
		{`a:for(;;)break a`, `a:for(;;)break a`},
		{`function f(a,{b}){return a+b}`, `function f(a,{b}){return a+b}`},

		// imports/exports
		{`export{a as "b"}from"m"`, `export{a as "b"}from"m"`},
		{`export*as"b"from"m"`, `export*as "b" from"m"`},
		{`export default class{}`, `export default class{}`},
		{`export default class a{}`, `export default class a{}`},
		{`export default function(){};c=d`, `export default function(){}c=d`},

		// optional chaining and templates
		{`a?.b`, `a?.b`},
		{`a?.(b)`, `a?.(b)`},
		{`a==null?undefined:a[b]`, `a?.[b]`},

		// misc expression contexts
		{`x=function(){return{a}}`, `x=function(){return{a}}`},
		{`if(x)({a:1})`, `x&&{a:1}`},
		{`a,b,c`, `a,b,c`},
		{`typeof a==="undefined"`, `typeof a>"u"`},
		{`typeof a=="undefined"`, `typeof a>"u"`},
		{`typeof a!=="undefined"`, `typeof a<"u"`},
		{`typeof a!="undefined"`, `typeof a<"u"`},
		{`"undefined"===typeof a`, `"u"<typeof a`},
		{`"undefined"!==typeof a`, `"u">typeof a`},
		{`typeof a.b.c==="undefined"`, `typeof a.b.c>"u"`},
		{`if(typeof a==="undefined")b()`, `typeof a>"u"&&b()`},
		// only the exact string "undefined" folds
		{`typeof a==="undefine"`, `typeof a=="undefine"`},
		{`typeof a==="undefinedd"`, `typeof a=="undefinedd"`},
		{`a==="undefined"`, `a==="undefined"`},
		{`a=void b()`, `a=void b()`},
		{`delete a.b`, `delete a.b`},
		{`x=let[0]`, `x=let[0]`},
		{`for(var a in b);`, `for(var a in b);`},
		{`for(a in b);`, `for(a in b);`},
		{`for(var a of b);`, `for(var a of b);`},
		{`for(const a of b);`, `for(const a of b);`},
		{`a in b`, `a in b`},
		{`for(;a in b;);`, `for(;a in b;);`},
		{`a instanceof b`, `a instanceof b`},

		// syntax errors pass through as error
	}

	m := cssmash.New()
	o := Minifier{KeepVarNames: true, useAlphabetVarNames: true}
	for _, tt := range jsTests {
		t.Run(tt.js, func(t *testing.T) {
			r := bytes.NewBufferString(tt.js)
			w := &bytes.Buffer{}
			err := o.Minify(m, w, r, nil)
			test.Minify(t, tt.js, err, w.String(), tt.expected)
		})
	}
}

func TestJSExtraVarRenaming(t *testing.T) {
	jsTests := []struct {
		js       string
		expected string
	}{
		{`function f(){var abcdefgh;abcdefgh}`, `function f(){var a;a}`},
		{`function f(){var a,b,c;a;b;c}`, `function f(){var a,b,c;a,b,c}`},
		{`function f(unused){var a;a}`, `function f(){var b;b}`},
		{`function f(used,unused){used}`, `function f(a){a}`},
		{`function f(unused,...rest){rest}`, `function f(b,...a){a}`},
		{`(a,unused)=>a`, `(a)=>a`}, // TODO(size): drop parens when params shrink to one
		{`class x{static #foo=1;static bar(){this.#foo}}`, `class x{static#a=1;static bar(){this.#a}}`},
	}

	m := cssmash.New()
	o := Minifier{useAlphabetVarNames: true}
	for _, tt := range jsTests {
		t.Run(tt.js, func(t *testing.T) {
			r := bytes.NewBufferString(tt.js)
			w := &bytes.Buffer{}
			err := o.Minify(m, w, r, nil)
			test.Minify(t, tt.js, err, w.String(), tt.expected)
		})
	}
}

func TestJSExtraOptions(t *testing.T) {
	m := cssmash.New()

	// Precision
	{
		w := &bytes.Buffer{}
		o := Minifier{KeepVarNames: true, useAlphabetVarNames: true, Precision: 3}
		err := o.Minify(m, w, bytes.NewBufferString(`a=1.23456;b=123456`), nil)
		test.Minify(t, `precision`, err, w.String(), `a=1.23,b=123e3`)
	}

	// KeepVarNames keeps names
	{
		w := &bytes.Buffer{}
		o := Minifier{KeepVarNames: true}
		err := o.Minify(m, w, bytes.NewBufferString(`function f(){var longname;longname}`), nil)
		test.Minify(t, `keepvarnames`, err, w.String(), `function f(){var longname;longname}`)
	}

	// Version gates
	for _, tt := range []struct {
		version  int
		js       string
		expected string
	}{
		{2015, `a==null?b:a`, `a==null?b:a`}, // ?? requires 2020
		{2020, `a==null?b:a`, `a??b`},        // ?? allowed
		{2018, `try{}catch(a){}`, `try{}catch(a){}`},
		{2019, `try{}catch(a){}`, `try{}catch{}`},
	} {
		w := &bytes.Buffer{}
		o := Minifier{KeepVarNames: true, useAlphabetVarNames: true, Version: tt.version}
		err := o.Minify(m, w, bytes.NewBufferString(tt.js), nil)
		test.Minify(t, tt.js, err, w.String(), tt.expected)
	}

	// Syntax error returns error
	{
		w := &bytes.Buffer{}
		err := Minify(m, w, bytes.NewBufferString(`function(`), nil)
		if err == nil {
			t.Error("expected error for invalid JS")
		}
	}
}

func TestJSIdempotent(t *testing.T) {
	// minifying minified output must not change it further
	inputs := []string{
		`function f(a,b){if(a){return b+1}else{return b-1}}`,
		`var x=1,y=2;for(var i=0;i<10;i++){x+=i}console.log(x,y)`,
		`class A{constructor(a){this.a=a}static b(){return new A(1)}}`,
		`import x,{y}from"m";export default x=>x+y`,
		`while(a)if(b)c();else d()`,
	}
	m := cssmash.New()
	o := Minifier{}
	for _, js := range inputs {
		w := &bytes.Buffer{}
		if err := o.Minify(m, w, bytes.NewBufferString(js), nil); err != nil {
			t.Fatal(err)
		}
		first := w.String()
		w2 := &bytes.Buffer{}
		if err := o.Minify(m, w2, strings.NewReader(first), nil); err != nil {
			t.Fatal(err)
		}
		if w2.String() != first {
			t.Errorf("not idempotent for %q:\nfirst:  %q\nsecond: %q", js, first, w2.String())
		}
	}
}
