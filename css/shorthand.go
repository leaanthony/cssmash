package css

// Longhand-to-shorthand collapsing.
//
// A block that sets all four sides of a box individually can nearly always say
// the same thing in one declaration:
//
//	margin-top:8px;margin-right:-15px;margin-bottom:8px;margin-left:-15px
//	margin:8px -15px
//
// The collapse is exact, but only under conditions that are easy to get wrong,
// so they are enforced explicitly rather than assumed:
//
//   - Every one of the four longhands appears exactly once in the block. A
//     repeated longhand means a later one overrides an earlier one, and the
//     merged declaration would sit in only one of those positions.
//   - No other declaration in the block can write any of the same properties.
//     `margin-block`, `inset-inline` and the shorthand itself all can, and
//     their position relative to the longhands decides the outcome. Vendor
//     prefixed properties are exempt: -webkit-border-radius is a different
//     property from border-radius and never aliases it.
//   - All four agree on !important, which the shorthand can only carry as a
//     single flag.
//   - No value is compound. A value containing whitespace, a comma, a slash or
//     a function is either multi-part (border-top-left-radius takes two radii)
//     or substitutable (var(), env()), and in both cases splicing it into a
//     positional shorthand can change what it means.
//   - CSS-wide keywords (inherit, initial, ...) only collapse when all four
//     agree, since a shorthand cannot mix a keyword with real values.
//
// Given those, the four values are position-independent with respect to
// everything else in the block, so the shorthand is written at the position of
// the last longhand and the other three are dropped.

import (
	"bytes"
)

// declaration is one minified declaration, held back so that a whole block can
// be considered at once.
type declaration struct {
	name      []byte // property name, as written
	value     []byte // minified value, without the !important flag
	important bool
	suffix    []byte // the !important bytes exactly as the writer emitted them
	dropped   bool
}

// boxShorthand describes a shorthand that takes one to four values in
// top/right/bottom/left order (or the border-radius corner order).
type boxShorthand struct {
	name  string
	parts [4]string
	// conflicts reports whether some other unprefixed property in the same
	// block might also write one of the parts.
	conflicts func(name []byte) bool
}

func prefixFold(name []byte, prefix string) bool {
	return len(prefix) <= len(name) && bytes.EqualFold(name[:len(prefix)], []byte(prefix))
}

var boxShorthands = []boxShorthand{
	{
		name:  "margin",
		parts: [4]string{"margin-top", "margin-right", "margin-bottom", "margin-left"},
		// margin, margin-block, margin-inline and their -start/-end forms
		conflicts: func(name []byte) bool { return prefixFold(name, "margin") },
	},
	{
		name:      "padding",
		parts:     [4]string{"padding-top", "padding-right", "padding-bottom", "padding-left"},
		conflicts: func(name []byte) bool { return prefixFold(name, "padding") },
	},
	{
		name:  "inset",
		parts: [4]string{"top", "right", "bottom", "left"},
		conflicts: func(name []byte) bool {
			// inset, inset-block, inset-inline, and the physical properties
			// themselves appearing a second time.
			return prefixFold(name, "inset") ||
				bytes.EqualFold(name, []byte("top")) || bytes.EqualFold(name, []byte("right")) ||
				bytes.EqualFold(name, []byte("bottom")) || bytes.EqualFold(name, []byte("left"))
		},
	},
	{
		name: "border-radius",
		parts: [4]string{
			"border-top-left-radius", "border-top-right-radius",
			"border-bottom-right-radius", "border-bottom-left-radius",
		},
		// border-radius plus the logical corner properties
		// (border-start-start-radius and friends)
		conflicts: func(name []byte) bool {
			return prefixFold(name, "border") && bytes.Contains(bytes.ToLower(name), []byte("radius"))
		},
	},
}

var cssWideKeywords = [][]byte{
	[]byte("inherit"), []byte("initial"), []byte("unset"), []byte("revert"), []byte("revert-layer"),
}

func isCSSWideKeyword(value []byte) bool {
	for _, kw := range cssWideKeywords {
		if bytes.EqualFold(value, kw) {
			return true
		}
	}
	return false
}

// isSimpleBoxValue reports whether a value is a single indivisible token, and
// so can be placed positionally inside a shorthand.
func isSimpleBoxValue(value []byte) bool {
	if len(value) == 0 {
		return false
	}
	return bytes.IndexAny(value, " \t\r\n,/()") < 0
}

// collapseBoxValues renders four side values in the shortest equivalent form:
// one value when all sides agree, two when the axes agree, three when only the
// horizontal axis agrees, four otherwise.
func collapseBoxValues(v [4][]byte) []byte {
	top, right, bottom, left := v[0], v[1], v[2], v[3]
	n := 4
	if bytes.Equal(right, left) {
		n = 3
		if bytes.Equal(top, bottom) {
			n = 2
			if bytes.Equal(top, right) {
				n = 1
			}
		}
	}
	out := make([]byte, 0, len(top)+len(right)+len(bottom)+len(left)+3)
	for i := 0; i < n; i++ {
		if i != 0 {
			out = append(out, ' ')
		}
		out = append(out, v[i]...)
	}
	return out
}

// collapseShorthands rewrites complete sets of box longhands in a single
// declaration block into their shorthand, in place. Declarations that are
// merged away are marked dropped rather than removed, so that callers can keep
// their own indices stable.
func collapseShorthands(decls []declaration) {
	if len(decls) < 4 {
		return
	}
	for _, sh := range boxShorthands {
		idx, ok := findBoxParts(decls, sh)
		if !ok {
			continue
		}

		var values [4][]byte
		important := decls[idx[0]].important
		wide := false
		usable := true
		for i, at := range idx {
			d := decls[at]
			if d.important != important || !isSimpleBoxValue(d.value) {
				usable = false
				break
			}
			if isCSSWideKeyword(d.value) {
				wide = true
			}
			values[i] = d.value
		}
		if !usable {
			continue
		}
		if wide {
			// A shorthand cannot mix a CSS-wide keyword with real values.
			for i := 1; i < 4; i++ {
				if !bytes.EqualFold(values[i], values[0]) {
					usable = false
					break
				}
			}
			if !usable {
				continue
			}
		}

		merged := collapseBoxValues(values)
		// len(name)+1 for "prop:", plus a separating ';' for each declaration
		// after the first.
		was := 0
		for i, at := range idx {
			was += len(decls[at].name) + 1 + len(values[i])
			if i != 0 {
				was++
			}
		}
		now := len(sh.name) + 1 + len(merged)
		if now >= was {
			continue
		}

		last := idx[0]
		for _, at := range idx {
			if last < at {
				last = at
			}
		}
		for _, at := range idx {
			if at != last {
				decls[at].dropped = true
			}
		}
		decls[last].name = []byte(sh.name)
		decls[last].value = merged
	}
}

// findBoxParts locates exactly one declaration for each of the shorthand's
// four longhands and verifies that nothing else in the block writes them.
func findBoxParts(decls []declaration, sh boxShorthand) ([4]int, bool) {
	idx := [4]int{-1, -1, -1, -1}
	for i := range decls {
		if decls[i].dropped {
			continue
		}
		matched := false
		for p, part := range sh.parts {
			if bytes.EqualFold(decls[i].name, []byte(part)) {
				if idx[p] != -1 {
					return idx, false // set twice; position matters
				}
				idx[p] = i
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		// Some other property. Vendor-prefixed spellings are separate
		// properties and never alias the unprefixed longhands.
		if 0 < len(decls[i].name) && decls[i].name[0] == '-' {
			continue
		}
		if bytes.EqualFold(decls[i].name, []byte("all")) {
			// `all` resets every property, so it conflicts with every group
			// no matter which one is being collapsed.
			return idx, false
		}
		if sh.conflicts(decls[i].name) {
			return idx, false
		}
	}
	for _, at := range idx {
		if at == -1 {
			return idx, false
		}
	}
	return idx, true
}
