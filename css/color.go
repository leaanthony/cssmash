package css

// Translucent color folding.
//
// A translucent color written as rgba()/hsla() can almost always be written
// more compactly as a Color Level 4 hex literal: rgba(255,255,255,.15) is 21
// bytes, #ffffff1a is 9, and #fff3 is 4. The red, green and blue channels are
// already 8-bit, so they convert exactly. The alpha channel is not: CSS alpha
// is a real number in [0,1] while hex alpha is one of 256 steps, so folding it
// is only exact when alpha lands on a step.
//
// AlphaPrecision selects which folds are permitted. The distinction matters,
// so it is spelled out rather than left to a general "aggressive" switch.

import (
	"encoding/hex"
	"math"

	"cssmash/parse/css"
)

// AlphaPrecision controls how translucent colors are folded to hex notation.
type AlphaPrecision int

const (
	// AlphaRounded folds every translucent color, rounding alpha to the
	// nearest 1/255. The alpha error is at most 1/510 (~0.00196), which is
	// half of one 8-bit step: composited over any backdrop it moves a
	// channel by at most 0.5/255, below the quantization of an 8-bit
	// framebuffer. This is the default and matches what other minifiers do,
	// but it is not bit-exact -- getComputedStyle reports the folded value,
	// and the error can compound across many stacked translucent layers.
	AlphaRounded AlphaPrecision = iota

	// AlphaExact folds only colors whose alpha already lies on a 1/255 step
	// (0, 1, .2, .4, .6, .8, and any n/255). These folds are bit-exact: the
	// computed value is unchanged. Colors with other alphas are left as
	// rgba()/hsla().
	AlphaExact

	// AlphaNone disables translucent folding entirely.
	AlphaNone
)

// alphaStepEpsilon bounds how far alpha*255 may sit from an integer and still
// count as exact. Component values are parsed at float32 precision, so an
// alpha written as ".2" arrives as 0.20000000298; the tolerance has to absorb
// that without admitting a genuinely inexact alpha. The tightest inexact alpha
// reachable is n/255 +/- 1/510, which is 0.5 away from an integer after
// scaling -- four orders of magnitude above this bound.
const alphaStepEpsilon = 1e-4

// alpha8 quantizes an alpha in [0,1] to an 8-bit level, reporting whether the
// quantization was exact.
func alpha8(a float64) (byte, bool) {
	scaled := a * 255.0
	nearest := math.Round(scaled)
	if nearest < 0.0 {
		nearest = 0.0
	} else if 255.0 < nearest {
		nearest = 255.0
	}
	return byte(nearest), math.Abs(scaled-nearest) < alphaStepEpsilon
}

// channel8 quantizes a color channel in [0,1] to an 8-bit level. Unlike alpha,
// channels originate as 8-bit integers in the overwhelmingly common
// rgb()/rgba() spelling, so no exactness is reported: a channel written as a
// percentage is already being rounded by every renderer.
func channel8(v float64) byte {
	scaled := v*255.0 + 0.5
	if scaled < 0.0 {
		return 0
	} else if 255.0 < scaled {
		return 255
	}
	return byte(scaled)
}

// rgbaToToken renders a translucent color as the shortest equivalent hex
// token. r, g, b and a are in [0,1]. It reports false when the fold is not
// permitted at the given precision, in which case the caller keeps the
// original function notation.
//
// Fully opaque colors are not handled here -- they fold to #rgb/#rrggbb via
// rgbToToken, which finds shorter spellings still (color keywords).
func rgbaToToken(r, g, b, a float64, prec AlphaPrecision) (Token, bool) {
	if prec == AlphaNone {
		return Token{}, false
	}
	av, exact := alpha8(a)
	if !exact && prec != AlphaRounded {
		return Token{}, false
	}

	val := make([]byte, 9)
	val[0] = '#'
	hex.Encode(val[1:], []byte{channel8(r), channel8(g), channel8(b), av})

	// #rrggbbaa collapses to #rgba when every channel is a doubled digit.
	if val[1] == val[2] && val[3] == val[4] && val[5] == val[6] && val[7] == val[8] {
		val[2] = val[3]
		val[3] = val[5]
		val[4] = val[7]
		val = val[:5]
	}
	return Token{css.HashToken, val, nil, 0, 0}, true
}
