package js

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"cssmash"
	"cssmash/parse"
	"cssmash/parse/buffer"
)

func nodePath(tb testing.TB) string {
	tb.Helper()
	path, err := exec.LookPath("node")
	if err != nil {
		tb.Skip("node not found in PATH; skipping semantic execution tests")
	}
	return path
}

func runNode(tb testing.TB, node string, src []byte) string {
	tb.Helper()
	file := filepath.Join(tb.TempDir(), "prog.js")
	if err := os.WriteFile(file, src, 0o644); err != nil {
		tb.Fatal(err)
	}
	out, err := exec.Command(node, file).CombinedOutput()
	if err != nil {
		tb.Fatalf("node failed: %v\noutput: %s\nprogram: %s", err, out, src)
	}
	return string(out)
}

// TestJSSemanticsNode executes self-checking programs before and after
// minification and requires identical output. The size ratchet in
// TestJSOutputSize only guards byte counts; this guards behavior, in
// particular for scope-sensitive transforms where a wrong merge or rename
// changes which variable an assignment hits.
func TestJSSemanticsNode(t *testing.T) {
	node := nodePath(t)

	programs := []string{
		// assignment to a captured outer variable following a local var
		// declaration must not be merged into it
		`(function(){
			var outer;
			function setIt(x){var t=x+1;outer=x;return t}
			setIt(41);
			console.log("outer =", outer);
		})();`,

		// reading an outer variable into a local, then overwriting the outer:
		// merging would make the read see the hoisted local instead
		`(function(){
			var l="outerval";
			var g=function(){var s=l;l=false;console.log("s =",s,"l =",l)};
			g();
			console.log("after:", l);
		})();`,

		// a comparator mutating an outer flag during sort (the Sizzle
		// uniqueSort pattern)
		`(function(){
			var hasDuplicate;
			function sortOrder(a,b){if(a===b)hasDuplicate=true;return a-b}
			function uniq(list){
				var v,out=[],i=0;
				hasDuplicate=false;
				list.sort(sortOrder);
				if(hasDuplicate){for(;v=list[i++];)v===list[i]||out.push(v)}else{out=list}
				return out;
			}
			console.log(uniq([3,1,2,1,3]).join(","));
		})();`,

		// same-function assignments reached through nested blocks may merge;
		// behavior must be unchanged
		`(function(){
			var x;
			{x=0}
			for(var i=0;i<3;i++){x+=i}
			console.log("x =",x);
			function f(){var a;{a=1}return a}
			console.log("f =",f());
		})();`,

		// string falsiness: empty, line continuation only, real escapes
		"console.log(\"\"?1:2, \"\\\n\"?3:4, \"\\n\"?5:6, \"\\0\"?7:8, \"x\"?9:10);",

		// typeof folded to a relational comparison against "u": every one of
		// the eight results typeof can produce must land on the same side of
		// the comparison as it did of the equality it replaced
		`(function(){
			var vals=[undefined,null,true,1,1n,"s",Symbol(),{},[],function(){},new Date()];
			var out=[];
			for(var i=0;i<vals.length;i++){
				var v=vals[i];
				out.push(typeof v==="undefined");
				out.push(typeof v!=="undefined");
				out.push("undefined"===typeof v);
			}
			out.push(typeof notDeclaredAnywhere==="undefined");
			out.push(typeof notDeclaredAnywhere!=="undefined");
			console.log(out.join(","));
		})();`,

		// early exits inverted and made to absorb the rest of the block:
		// a bare return/continue is what falling off the end already does
		`(function(){
			var log=[];
			function f(a,b){if(a)return;log.push("f-body");if(b)return;log.push("f-tail")}
			function g(a){if(a)return;var v=1;log.push("g"+v)}
			function h(a){if(a)return;return "h-value"}
			function loop(n){var out=[];for(var i=0;i<n;i++){if(i%2)continue;out.push(i);if(i>3)continue;out.push("small"+i)}return out.join("|")}
			function nested(a,b,c){if(a)return;if(b)return;if(c)return;log.push("all-false")}
			[[0,0],[0,1],[1,0],[1,1]].forEach(function(p){f(p[0],p[1])});
			[0,1].forEach(g);
			log.push(String(h(0)),String(h(1)));
			log.push(loop(8));
			nested(0,0,0); nested(0,0,1); nested(1,0,0);
			// a var declaration absorbed into the inverted if is still hoisted
			function hoist(a){if(a)return typeof later;var later=5;return typeof later}
			log.push(hoist(1),hoist(0));
			console.log(log.join(","));
		})();`,

		// early-exit inversion must NOT reach statement lists where falling
		// off the end is not what the return/continue did: a switch clause
		// falls through to the next clause, and a nested block continues with
		// the statements after it
		`(function(){
			var log=[];
			function sw(x,a){
				switch(x){
				case 1:
					if(a)return "early";
					log.push("case1-tail");
				case 2:
					log.push("case2");
					break;
				default:
					log.push("default");
				}
				return "end";
			}
			log.push(sw(1,1),sw(1,0),sw(2,0),sw(9,0));
			function blk(a){ {if(a)return "ret"; log.push("blk-tail")} log.push("after-blk"); return "fell" }
			log.push(blk(1),blk(0));
			function loopSwitch(n){
				var out=[];
				for(var i=0;i<n;i++){
					switch(i%3){
					case 0:
						if(i>0)continue;
						out.push("zero"+i);
					case 1:
						out.push("one"+i);
						break;
					default:
						out.push("d"+i);
					}
					out.push("after"+i);
				}
				return out.join("|");
			}
			log.push(loopSwitch(7));
			function tryFin(a){
				var out=[];
				try{ if(a)return "try-early"; out.push("try-tail") }
				catch(e){ out.push("catch") }
				finally{ out.push("finally") }
				out.push("after");
				return out.join("|");
			}
			log.push(tryFin(1),tryFin(0));
			console.log(log.join(","));
		})();`,

		// closures over loop variables and shadowing
		`(function(){
			var fns=[];
			for(var i=0;i<3;i++){(function(i){fns.push(function(){return i})})(i)}
			console.log(fns.map(function(f){return f()}).join(","));
			var v=1;
			function outerRead(){return v}
			function shadow(){var v=2;return v+outerRead()}
			console.log(shadow());
		})();`,

		// var hoisting across statements that get merged into for-init
		`(function(){
			var acc="";
			var n=3;
			var out=[];
			var i=n;
			while(i--){out.push(i)}
			acc=out.join("");
			console.log(acc,i);
		})();`,
	}

	m := cssmash.New()
	for _, src := range programs {
		want := runNode(t, node, []byte(src))
		w := &bytes.Buffer{}
		if err := Minify(m, w, bytes.NewReader([]byte(src)), nil); err != nil {
			t.Fatalf("minify failed: %v\nprogram: %s", err, src)
		}
		if got := runNode(t, node, w.Bytes()); got != want {
			t.Errorf("minified program behaves differently\nprogram:  %s\nminified: %s\nwant output: %q\ngot output:  %q", src, w.Bytes(), want, got)
		}
	}
}

// TestJSFixturesSyntaxNode verifies that every minified fixture is still
// syntactically valid JavaScript according to node --check.
func TestJSFixturesSyntaxNode(t *testing.T) {
	node := nodePath(t)

	m := cssmash.New()
	for name, buf := range loadJSFixtures(t) {
		w := buffer.NewWriter(make([]byte, 0, len(buf)))
		if err := Minify(m, w, buffer.NewReader(parse.Copy(buf)), nil); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		file := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(file, w.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(node, "--check", file).CombinedOutput(); err != nil {
			t.Errorf("%s: minified output fails node --check: %v\n%s", name, err, out)
		}
	}
}
