package css

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"cssmash"
)

func minifyBlock(t *testing.T, block string) string {
	t.Helper()
	m := cssmash.New()
	w := &bytes.Buffer{}
	if err := Minify(m, w, strings.NewReader(block), nil); err != nil {
		t.Fatalf("minify %q: %v", block, err)
	}
	return w.String()
}

func TestCollapseShorthand(t *testing.T) {
	tests := []struct{ in, want string }{
		// the four collapse widths
		{"a{margin-top:1px;margin-right:1px;margin-bottom:1px;margin-left:1px}", "a{margin:1px}"},
		{"a{margin-top:1px;margin-right:2px;margin-bottom:1px;margin-left:2px}", "a{margin:1px 2px}"},
		{"a{margin-top:1px;margin-right:2px;margin-bottom:3px;margin-left:2px}", "a{margin:1px 2px 3px}"},
		{"a{margin-top:1px;margin-right:2px;margin-bottom:3px;margin-left:4px}", "a{margin:1px 2px 3px 4px}"},

		{"a{padding-top:0;padding-right:0;padding-bottom:0;padding-left:0}", "a{padding:0}"},
		{"a{top:0;right:0;bottom:0;left:0}", "a{inset:0}"},
		{"a{border-top-left-radius:4px;border-top-right-radius:4px;border-bottom-right-radius:0;border-bottom-left-radius:0}", "a{border-radius:4px 4px 0 0}"},

		// source order need not match shorthand order
		{"a{margin-left:4px;margin-bottom:3px;margin-right:2px;margin-top:1px}", "a{margin:1px 2px 3px 4px}"},
		// unrelated declarations are position-independent and survive
		{"a{color:red;margin-top:1px;margin-right:1px;margin-bottom:1px;margin-left:1px;display:none}", "a{color:red;margin:1px;display:none}"},
		// all four important
		{"a{margin-top:1px!important;margin-right:1px!important;margin-bottom:1px!important;margin-left:1px!important}", "a{margin:1px!important}"},
		// all four the same CSS-wide keyword
		{"a{margin-top:inherit;margin-right:inherit;margin-bottom:inherit;margin-left:inherit}", "a{margin:inherit}"},

		// --- refusals ---
		// a repeated longhand: the merged declaration could only sit in one place
		{"a{margin-top:1px;margin-right:2px;margin-bottom:3px;margin-left:4px;margin-top:9px}",
			"a{margin-top:1px;margin-right:2px;margin-bottom:3px;margin-left:4px;margin-top:9px}"},
		// the shorthand itself is present and its position decides the result
		{"a{margin-top:1px;margin:5px;margin-right:2px;margin-bottom:3px;margin-left:4px}",
			"a{margin-top:1px;margin:5px;margin-right:2px;margin-bottom:3px;margin-left:4px}"},
		// a logical property writes the same physical sides
		{"a{margin-top:1px;margin-right:2px;margin-bottom:3px;margin-left:4px;margin-block:9px}",
			"a{margin-top:1px;margin-right:2px;margin-bottom:3px;margin-left:4px;margin-block:9px}"},
		{"a{top:0;right:0;bottom:0;left:0;inset-inline:5px}", "a{top:0;right:0;bottom:0;left:0;inset-inline:5px}"},
		// `all` resets every property, so its position matters for every group
		{"a{top:0;all:unset;right:0;bottom:0;left:0}", "a{top:0;all:unset;right:0;bottom:0;left:0}"},
		{"a{margin-top:1px;all:unset;margin-right:1px;margin-bottom:1px;margin-left:1px}",
			"a{margin-top:1px;all:unset;margin-right:1px;margin-bottom:1px;margin-left:1px}"},
		// mixed importance cannot be carried by one flag
		{"a{margin-top:1px!important;margin-right:1px;margin-bottom:1px;margin-left:1px}",
			"a{margin-top:1px!important;margin-right:1px;margin-bottom:1px;margin-left:1px}"},
		// a CSS-wide keyword cannot be mixed with real values
		{"a{margin-top:inherit;margin-right:1px;margin-bottom:1px;margin-left:1px}",
			"a{margin-top:inherit;margin-right:1px;margin-bottom:1px;margin-left:1px}"},
		// compound values are not positionally splice-able
		{"a{border-top-left-radius:10px 20px;border-top-right-radius:4px;border-bottom-right-radius:4px;border-bottom-left-radius:4px}",
			"a{border-top-left-radius:10px 20px;border-top-right-radius:4px;border-bottom-right-radius:4px;border-bottom-left-radius:4px}"},
		// var() may substitute to more than one token
		{"a{margin-top:var(--x);margin-right:1px;margin-bottom:1px;margin-left:1px}",
			"a{margin-top:var(--x);margin-right:1px;margin-bottom:1px;margin-left:1px}"},
		// incomplete set
		{"a{margin-top:1px;margin-right:1px;margin-bottom:1px}", "a{margin-top:1px;margin-right:1px;margin-bottom:1px}"},
		// a vendor-prefixed property is a different property and does not block
		{"a{-webkit-margin-before:9px;margin-top:1px;margin-right:1px;margin-bottom:1px;margin-left:1px}",
			"a{-webkit-margin-before:9px;margin:1px}"},
		// collapsing must actually pay for itself
		{"a{top:auto;right:auto;bottom:auto;left:auto}", "a{inset:auto}"},
	}
	for _, tt := range tests {
		if got := minifyBlock(t, tt.in); got != tt.want {
			t.Errorf("%s\n got: %s\nwant: %s", tt.in, got, tt.want)
		}
	}
}

// resolveBox applies a declaration list in order, expanding the box shorthands
// the minifier knows how to produce, and returns the resulting longhand map.
// Two declaration lists that resolve identically are interchangeable.
func resolveBox(decls []string) map[string]string {
	out := map[string]string{}
	expand := map[string][4]string{
		"margin":        {"margin-top", "margin-right", "margin-bottom", "margin-left"},
		"padding":       {"padding-top", "padding-right", "padding-bottom", "padding-left"},
		"inset":         {"top", "right", "bottom", "left"},
		"border-radius": {"border-top-left-radius", "border-top-right-radius", "border-bottom-right-radius", "border-bottom-left-radius"},
	}
	for _, d := range decls {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		i := strings.IndexByte(d, ':')
		if i < 0 {
			continue
		}
		name, value := strings.TrimSpace(d[:i]), strings.TrimSpace(d[i+1:])
		if name == "all" {
			// `all` resets every property, including ones not named anywhere
			// in the block. Modelled by discarding everything accumulated so
			// far and recording the reset, so that a collapse which moves a
			// longhand across it resolves differently.
			for k := range out {
				delete(out, k)
			}
			out["\x00all"] = value
			continue
		}
		sides, ok := expand[name]
		if !ok {
			out[name] = value
			continue
		}
		f := strings.Fields(value)
		switch len(f) {
		case 1:
			f = []string{f[0], f[0], f[0], f[0]}
		case 2:
			f = []string{f[0], f[1], f[0], f[1]}
		case 3:
			f = []string{f[0], f[1], f[2], f[1]}
		}
		if len(f) != 4 {
			out[name] = value
			continue
		}
		for j, side := range sides {
			out[side] = f[j]
		}
	}
	return out
}

// TestCollapseShorthandEquivalence is the differential guard: a collapse that
// changes which value lands on which side still produces perfectly valid CSS
// that parses and fuzzes cleanly, so bytes cannot be compared. This resolves
// both sides to longhand maps and compares those instead.
func TestCollapseShorthandEquivalence(t *testing.T) {
	groups := [][4]string{
		{"margin-top", "margin-right", "margin-bottom", "margin-left"},
		{"padding-top", "padding-right", "padding-bottom", "padding-left"},
		{"top", "right", "bottom", "left"},
		{"border-top-left-radius", "border-top-right-radius", "border-bottom-right-radius", "border-bottom-left-radius"},
	}
	values := []string{"0", "1px", "2px", "-15px", "auto", "50%", "1em"}

	rng := rand.New(rand.NewSource(1))
	collapsed := 0
	for i := 0; i < 4000; i++ {
		g := groups[rng.Intn(len(groups))]

		// A random permutation of the four sides, sometimes incomplete, with
		// unrelated declarations mixed in.
		order := rng.Perm(4)
		n := 4
		if rng.Intn(6) == 0 {
			n = rng.Intn(4)
		}
		var decls []string
		for _, at := range order[:n] {
			decls = append(decls, fmt.Sprintf("%s:%s", g[at], values[rng.Intn(len(values))]))
		}
		if rng.Intn(3) == 0 {
			decls = append(decls, "color:red")
		}
		if rng.Intn(4) == 0 {
			decls = append(decls, "display:block")
		}
		if rng.Intn(5) == 0 {
			decls = append(decls, "all:unset")
		}
		rng.Shuffle(len(decls), func(a, b int) { decls[a], decls[b] = decls[b], decls[a] })

		in := "a{" + strings.Join(decls, ";") + "}"
		out := minifyBlock(t, in)
		body := strings.TrimSuffix(strings.TrimPrefix(out, "a{"), "}")
		if body == "" && n == 0 {
			continue
		}
		gotDecls := strings.Split(body, ";")
		if len(gotDecls) < len(decls) {
			collapsed++
		}

		want := resolveBox(decls)
		got := resolveBox(gotDecls)
		if len(want) != len(got) {
			t.Fatalf("%s -> %s\nresolved to %v, want %v", in, out, got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Fatalf("%s -> %s\n%s resolved to %q, want %q", in, out, k, got[k], v)
			}
		}
	}
	if collapsed == 0 {
		t.Error("no case collapsed; the test is not exercising the transform")
	}
	t.Logf("%d of 4000 blocks collapsed", collapsed)
}
