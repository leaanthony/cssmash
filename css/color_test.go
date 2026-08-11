package css

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"cssmash"
)

// resolveColor parses any color spelling the minifier is capable of emitting
// back into a resolved 8-bit RGB triple plus a real-valued alpha, so that a
// fold can be checked against its input by value rather than by bytes.
func resolveColor(s string) (r, g, b uint8, a float64, ok bool) {
	s = strings.TrimSpace(s)
	if s == "transparent" {
		return 0, 0, 0, 0, true
	}
	if hexValue, found := ShortenColorName[ToHash([]byte(strings.ToLower(s)))]; found {
		s = string(hexValue)
	} else {
		// ShortenColorHex maps the other way, for names shorter than their
		// hex spelling (teal, gold, ...).
		for hexValue, name := range ShortenColorHex {
			if strings.EqualFold(string(name), s) {
				s = hexValue
				break
			}
		}
	}
	if strings.HasPrefix(s, "#") {
		d := s[1:]
		switch len(d) {
		case 3, 4:
			expanded := ""
			for i := 0; i < len(d); i++ {
				expanded += string(d[i]) + string(d[i])
			}
			d = expanded
		case 6, 8:
		default:
			return 0, 0, 0, 0, false
		}
		v, err := strconv.ParseUint(d, 16, 64)
		if err != nil {
			return 0, 0, 0, 0, false
		}
		a = 1.0
		if len(d) == 8 {
			a = float64(v&0xff) / 255.0
			v >>= 8
		}
		return uint8(v >> 16), uint8(v >> 8), uint8(v), a, true
	}
	// rgb()/rgba() with comma or space separators; enough for the round-trip
	// check, since anything else means the value was left unfolded.
	if i := strings.IndexByte(s, '('); i >= 0 && strings.HasPrefix(s, "rgb") {
		fields := strings.FieldsFunc(s[i+1:strings.LastIndexByte(s, ')')], func(r rune) bool {
			return r == ',' || r == ' ' || r == '/'
		})
		if len(fields) < 3 {
			return 0, 0, 0, 0, false
		}
		var v [4]float64
		v[3] = 1.0
		for j, f := range fields {
			if j > 3 {
				return 0, 0, 0, 0, false
			}
			scale := 1.0
			if strings.HasSuffix(f, "%") {
				f = f[:len(f)-1]
				if j == 3 {
					scale = 1.0 / 100.0
				} else {
					scale = 255.0 / 100.0
				}
			}
			n, err := strconv.ParseFloat(f, 64)
			if err != nil {
				return 0, 0, 0, 0, false
			}
			v[j] = n * scale
		}
		return uint8(v[0] + 0.5), uint8(v[1] + 0.5), uint8(v[2] + 0.5), v[3], true
	}
	return 0, 0, 0, 0, false
}

func minifyColorValue(t *testing.T, prec AlphaPrecision, decl string) string {
	t.Helper()
	m := cssmash.New()
	w := &bytes.Buffer{}
	o := &Minifier{AlphaPrecision: prec}
	if err := o.Minify(m, w, strings.NewReader("a{"+decl+"}"), nil); err != nil {
		t.Fatalf("minify %q: %v", decl, err)
	}
	out := w.String()
	out = strings.TrimSuffix(strings.TrimPrefix(out, "a{"), "}")
	if i := strings.IndexByte(out, ':'); i >= 0 {
		return out[i+1:]
	}
	return out
}

// TestColorFoldPrecision is the contract for translucent color folding: the
// rounded tier may move alpha by at most half an 8-bit step and must leave RGB
// untouched, and the exact tier must be bit-exact or not fold at all. A fold
// that silently shifts a color produces perfectly valid CSS, so byte-level
// golden tests cannot catch it -- this compares resolved values instead.
func TestColorFoldPrecision(t *testing.T) {
	// The bound is half of one 8-bit step. Add a small slack for the float32
	// precision at which components are parsed.
	const bound = 1.0/510.0 + 1e-6

	channels := []int{0, 1, 17, 51, 102, 128, 153, 204, 254, 255}
	alphas := []float64{
		0, 0.0001, 0.004, 0.025, 0.05, 0.075, 0.1, 0.125, 0.15, 0.2,
		0.25, 0.3, 0.333333, 0.4, 0.5, 0.6, 0.666667, 0.75, 0.8, 0.9,
		0.925, 0.99, 0.996, 0.999,
	}

	folded, unfolded := 0, 0
	for _, r := range channels {
		for _, g := range channels {
			for _, a := range alphas {
				b := (r*7 + g*13) % 256
				in := fmt.Sprintf("color:rgba(%d,%d,%d,%s)", r, g, b, strconv.FormatFloat(a, 'f', -1, 64))
				out := minifyColorValue(t, AlphaRounded, in)

				gotR, gotG, gotB, gotA, ok := resolveColor(out)
				if !ok {
					t.Fatalf("%s -> %s: cannot resolve output color", in, out)
				}
				if int(gotR) != r || int(gotG) != g || int(gotB) != b {
					// An alpha of zero legitimately collapses to the
					// `transparent` keyword, which discards RGB.
					if gotA == 0 && a == 0 {
						continue
					}
					t.Errorf("%s -> %s: RGB changed, want %d,%d,%d got %d,%d,%d", in, out, r, g, b, gotR, gotG, gotB)
					continue
				}
				if math.Abs(gotA-a) > bound {
					t.Errorf("%s -> %s: alpha moved %g, bound is %g", in, out, math.Abs(gotA-a), bound)
				}

				// Exact tier: fold only when it costs nothing.
				exactOut := minifyColorValue(t, AlphaExact, in)
				_, _, _, exactA, ok := resolveColor(exactOut)
				if !ok {
					t.Fatalf("%s -> %s: cannot resolve exact-tier output", in, exactOut)
				}
				if strings.HasPrefix(exactOut, "#") || !strings.Contains(exactOut, "(") {
					if math.Abs(exactA-a) > 1e-6 {
						t.Errorf("%s -> %s: exact tier folded with alpha error %g", in, exactOut, math.Abs(exactA-a))
					}
					folded++
				} else {
					unfolded++
				}
			}
		}
	}
	if folded == 0 || unfolded == 0 {
		t.Errorf("exact tier should both fold and decline: folded=%d unfolded=%d", folded, unfolded)
	}
}

// TestColorFoldLegacyGuard verifies that Color Level 4 hex is never introduced
// where a renderer predating it would have to parse the declaration: vendor
// prefixed properties and functions exist precisely to serve those renderers,
// and an unparsable value drops the whole declaration.
func TestColorFoldLegacyGuard(t *testing.T) {
	tests := []struct {
		decl string
		want string
	}{
		{"box-shadow:0 0 5px rgba(0,0,0,.5)", "0 0 5px #00000080"},
		{"-webkit-box-shadow:0 0 5px rgba(0,0,0,.5)", "0 0 5px rgba(0,0,0,.5)"},
		{"-moz-box-shadow:0 0 5px rgba(0,0,0,.5)", "0 0 5px rgba(0,0,0,.5)"},
		{"background:linear-gradient(rgba(0,0,0,.5),red)", "linear-gradient(#00000080,red)"},
		{"background:-webkit-linear-gradient(rgba(0,0,0,.5),red)", "-webkit-linear-gradient(rgba(0,0,0,.5),red)"},
		{"--shadow:rgba(0,0,0,.5)", "rgba(0,0,0,.5)"},
	}
	for _, tt := range tests {
		if got := minifyColorValue(t, AlphaRounded, tt.decl); got != tt.want {
			t.Errorf("%s\n got: %s\nwant: %s", tt.decl, got, tt.want)
		}
	}
}

// TestColorFoldDisabled checks that AlphaNone leaves translucent colors alone.
func TestColorFoldDisabled(t *testing.T) {
	for _, decl := range []string{"color:rgba(0,0,0,.5)", "color:rgba(0,0,0,.2)"} {
		got := minifyColorValue(t, AlphaNone, decl)
		if strings.HasPrefix(got, "#") {
			t.Errorf("%s folded to %s with AlphaNone", decl, got)
		}
	}
}
