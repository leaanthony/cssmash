# cssmash

JavaScript and CSS minification for Go, in-process and with no external
toolchain. On the benchmark corpus in `testdata/` it produces smaller output
than esbuild on every file.

| | cssmash | esbuild v0.28.1 | |
|---|---:|---:|---|
| JavaScript (6 files, 1.9 MB in) | **786,881** | 789,756 | 0.36% smaller |
| CSS (7 files, 178 KB in) | **142,140** | 142,494 | 0.25% smaller |

Per file, smallest wins: ace.js −1682, dataTables −571, jquery.js −317,
jquery-ui −215, moment −61, dot.js −29; 1.css −147, 7.css −145, 3.css −43,
5.css −15, 2.css −2, 4.css −1, 6.css −1.

Throughput is roughly 80 MB/s on JavaScript and 113–132 MB/s on CSS,
single-threaded, with no cgo and one test-only dependency. The CSS figure is
down about 20% from before shorthand collapsing, which has to hold a
declaration block in memory rather than stream it; that is the price of the
transform and it is paid on every stylesheet, not only ones that collapse.

## Usage

```go
import (
	"cssmash"
	"cssmash/css"
	"cssmash/js"
)

m := cssmash.New()
m.AddFunc("text/css", css.Minify)
m.AddFunc("application/javascript", js.Minify)

out, err := m.String("text/css", "a { color: #ff0000; }")
// "a{color:red}"
```

Or call `css.Minify` / `js.Minify` directly with an `io.Writer`/`io.Reader`.
Both packages expose a `Minifier` struct for options.

## Provenance

cssmash began as a trimmed fork of
[tdewolff/minify](https://github.com/tdewolff/minify) v2.24.16 and its parser
[tdewolff/parse](https://github.com/tdewolff/parse) v2.8.15 (vendored under
`parse/`), keeping only the JavaScript and CSS minifiers. That lineage is real
and is not something a refactor can undo, so it is stated plainly here and the
upstream MIT license is retained in [LICENSE](LICENSE). The parser and the bulk
of both minifiers are upstream's work.

What is original to this project is the optimization and correctness work
described below. Inherited code has deliberately *not* been renamed or
restructured to obscure its origin: keeping it recognisable is what makes it
possible to pull in upstream fixes later.

## Optimization passes original to cssmash

Each lives in its own file with its correctness argument written out, because
in a minifier the interesting part is not the transform but the conditions
under which it stays exact.

**`css/color.go` — translucent color folding.** `rgba(255,255,255,.15)` is 21
bytes; `#ffffff1a` is 9 and `#fff3` is 4. RGB converts exactly; alpha does not,
since CSS alpha is a real number in [0,1] and hex alpha is one of 256 steps.
Rather than bury that, `AlphaPrecision` names it:

- `AlphaRounded` (default) folds every translucent color, moving alpha by at
  most 1/510 — half of one 8-bit step, below the quantization of the
  framebuffer it composites into.
- `AlphaExact` folds only alphas already on a 1/255 step (`0`, `1`, `.2`, `.4`,
  `.6`, `.8`, any *n*/255). These folds are bit-exact.
- `AlphaNone` disables folding.

Folding is skipped inside vendor-prefixed properties and functions: a renderer
old enough to need `-webkit-box-shadow` would reject `#rrggbbaa` and drop the
declaration entirely.

The default is the aggressive tier because on this corpus nothing else wins:
CSS totals 142,140 rounded, 142,540 exact-only and 142,596 with folding off,
against esbuild's 142,494. Lossless transforms alone cannot close the gap. But
the exact tier costs only 0.03% over the default, so bit-exactness is cheap if
you need it — and it is a guarantee esbuild does not offer at all, since it
rounds unconditionally with no opt-out.

**`css/shorthand.go` — box longhand collapsing.** `margin`, `padding`, `inset`
and `border-radius` collapse from their four longhands, which required holding
declarations back and considering a block as a whole rather than streaming them
out. Refused when any longhand repeats, when another unprefixed property in the
block writes the same sides (`margin-block`, `inset-inline`, the shorthand
itself, `all`), on mixed `!important`, on compound or substitutable values, or
on CSS-wide keywords unless all four agree.

Note that `inset` is newer than `#rrggbbaa` in browser support (Chrome 87 /
Safari 14.1, against Chrome 62 / Safari 10), and unlike the color folding it
has no opt-out: on a renderer that does not know it, the declaration is dropped
and all four positioning values are lost. It is worth 44 bytes on 1.css, where
the margin is 147, so removing that entry from `boxShorthands` still beats
esbuild on every file if you need to support those renderers.

**`js/typeof.go` — relational typeof folding.** `typeof x == "undefined"`
becomes `typeof x > "u"`, nine bytes shorter per site. `typeof` yields one of
eight strings; exactly one sorts above `"u"`, the rest begin with a letter
before `u`, and none equals `"u"`, so the rewrite is exact in both directions.

**`js/stmtlist.go` — early-exit inversion.** `if(a)return; rest` becomes
`if(!a)rest`, which the writer then collapses to `a||rest`. A bare `return`
does exactly what falling off the end of a function body already does, and a
bare `continue` likewise for a loop body. Refused when the absorbed statement
is a lexical declaration.

Also: two conditional rewrites now check that they actually shrink before
firing, since coercing a condition with `!!` and parenthesising it can cost
more than the `?:` being replaced.

## Correctness fixes

One bug was introduced by the fork itself and never existed upstream:
assignments to a captured outer variable were merged into a local `var`,
re-declaring it and shadowing the outer binding. This broke jQuery's Sizzle
sort comparator, and was the entire source of the fork's original claimed size
advantage.

The remaining sixteen are inherited — upstream v2.24.16 still has them — and
each was verified to reproduce at the import commit before being fixed. The
ones found by fuzzing have checked-in corpus entries.

*Silently wrong output:*

- `""` and strings containing only line continuations were treated as truthy,
  so `""?1:2` folded to `1`.
- Any `#rrggbb00` collapsed to `#0000`, discarding RGB from a fully transparent
  color. That RGB is observable through interpolation — it is why fading to
  `transparent` washes through grey, and why stylesheets write
  `rgba(255,255,255,0)` instead. Upstream fixed this for the `rgba()` spelling
  (issue #327) but not the hex one.

*Output that did not re-parse:*

- `@import url( "x.css" )` emitted invalid doubled quotes.
- A declaration whose function was closed by a mismatched `}`.
- An unterminated `/*!` comment had a terminator forged from its own last two
  bytes, spilling the tail into the stylesheet.
- A property, selector or at-rule prelude ending in a dangling backslash, or
  mid-UTF-8-sequence, absorbed the punctuation written after it.
- `(0.).A` wrote `0.A`, which relexes as a number followed by an identifier,
  and `(.0_0).A` wrote `.0_.A` because that branch normalized through the CSS
  number formatter, which leaves JS numeric separators in place.
- `(a,b)=c` dropped the parentheses around an assignment target; `a,b=c`
  regroups as `a,(b=c)`.
- Two adjacent selector delimiters were written with their separator dropped,
  fusing `* =` into the single `*=` match operator.
- `&&=`, `||=` and `??=` were missing from all three operator precedence maps,
  so `exprPrec` reported the loosest possible binding for them and every
  precedence decision involving them was wrong: `a?a:b??=c` minified to the
  invalid `a||b??=c`.
- The IE9/10/11 workaround that quotes a font name beginning with `-` fired on
  function tokens too, so `font:0 -A(` became `font:0 '-A(')` — the arguments
  and closing parenthesis are written after the closing quote.
- `export{}` wrote a bare `export`; an empty clause cannot be dropped, since it
  is what marks a file as a module.
- A lone non-ASCII byte — the truncated head of a UTF-8 sequence — was sorted
  away from the end of input by the gzip-ordering heuristic, where it then ate
  the following bytes as continuations.

*Other:* the JS lexer classifies identifiers with the same lenient UTF-8
decoding it lexes with.

Two inherited behaviours are knowingly kept:

- `-ms-filter` rewrites `"progid:DXImageTransform.Microsoft.Alpha(Opacity=80)"`
  to `"alpha(opacity=80)"`, which IE accepts as a documented shorthand.
- The CSS parser can merge two delimiters separated by whitespace into one
  token: `{* =` arrives as a single `Delim` holding `"*="`, which relexes as
  the `*=` match operator. This happens before the minifier sees the tokens, so
  nothing it writes can undo it; the fuzz target detects and skips such input
  rather than pretend the output guarantee holds. A bare match operator is not
  valid Selectors syntax, so no real stylesheet reaches it. The writer
  separately refuses to fuse adjacent selector tokens of its own accord
  (`fusesWith`).

## Contributing

### Bug Reporting

Create a PR with test cases to reproduce the issue.

### Bug Fixing

Create a PR with a fix to an existing or new test.

### Performance

Create a PR with a benchmark test and a fix. Provide before/after numbers.

## Testing

```sh
go test ./...
```

The suite is built around the observation that a minifier bug usually produces
*valid* output — so parsing and fuzzing cannot see it, and byte-level goldens
flag it as an improvement.

- **Size goldens** (`js/bench_test.go`, `css/bench_test.go`) pin fixture output
  in *both* directions. A shrink fails until the diff has been inspected; this
  is how the scope miscompile above first presented itself, as a 3-byte win.
- **Node semantic tests** (`js/semantic_test.go`) run self-checking programs
  before and after minification and require identical output, plus `node
  --check` over every minified fixture. Skipped when `node` is absent. These
  cover the scope-sensitive transforms, all eight `typeof` results, and
  early-exit inversion.
- **Resolved-value differential tests** compare meaning rather than bytes:
  `css/shorthand_test.go` resolves 4000 randomized blocks back to longhand maps
  on both sides, and `css/color_test.go` checks every fold against its input as
  a resolved RGBA tuple, asserting bit-exactness for the exact tier and the
  stated bound for the rounded one.
- **Fuzz targets** for both minifiers, the variable renamer and the numeric
  parsing helpers, with invariants matched
  to what is actually promised. CSS output validity is asserted only for input
  that parses cleanly, since invalid CSS is passed through verbatim by design.
  JS output validity only for input that is valid UTF-8, since ECMAScript
  source is defined over Unicode code points. Neither minifier is idempotent,
  and a second JS pass is not even monotonic in size — a fold can be worth
  taking because of what it enables downstream even when it costs a byte where
  it sits — so what is asserted of a second pass is that it succeeds and still
  produces valid output. Single-pass size is pinned by the goldens instead.

Benchmarks: `go test -bench . -run xxx ./js/ ./css/`.

## License

MIT, per the upstream projects — see [LICENSE](LICENSE).
