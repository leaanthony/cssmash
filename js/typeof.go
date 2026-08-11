package js

// Relational folding of typeof comparisons.
//
// `typeof x == "undefined"` is a nine-byte-longer way of writing
// `typeof x > "u"`. The typeof operator can only produce one of eight strings:
//
//	undefined boolean number bigint string symbol function object
//
// Exactly one of them, "undefined", sorts above "u": it begins with 'u' and is
// longer, so it compares greater. Every other result begins with a letter
// before 'u' ('b', 'f', 'n', 'o', 's'), so it compares less. None of them is
// equal to "u". The ordering is therefore total and the rewrite is exact in
// both directions:
//
//	typeof x == "undefined"   =>   typeof x > "u"
//	typeof x != "undefined"   =>   typeof x < "u"
//
// The one historical exception is Internet Explorer, whose ActiveX host
// objects could yield "unknown" -- which also sorts above "u" and would flip
// this comparison. No engine in service does that, and this fork already emits
// Color Level 4 CSS, so the same audience assumption applies.

import (
	"bytes"

	"cssmash/parse/js"
)

var uBytes = []byte(`"u"`)

// isUndefinedString reports whether i is a string literal spelling the eight
// characters of "undefined". Escaped spellings are not recognised; they are
// vanishingly rare and there is no need to decode a literal to save bytes.
func isUndefinedString(i js.IExpr) bool {
	lit, ok := i.(*js.LiteralExpr)
	if !ok || lit.TokenType != js.StringToken || len(lit.Data) != 11 {
		return false
	}
	quote := lit.Data[0]
	if quote != '"' && quote != '\'' || lit.Data[10] != quote {
		return false
	}
	return bytes.Equal(lit.Data[1:10], []byte("undefined"))
}

// isTypeof reports whether i is a `typeof ...` expression.
func isTypeof(i js.IExpr) bool {
	unary, ok := i.(*js.UnaryExpr)
	return ok && unary.Op == js.TypeofToken
}

// foldTypeofUndefined rewrites an equality test between `typeof x` and the
// string "undefined" as a relational test against "u". It reports false when
// the expression is not of that shape.
//
// The replacement operator binds more tightly than the one it replaces, so no
// grouping that was previously unnecessary becomes necessary.
func foldTypeofUndefined(expr *js.BinaryExpr) (*js.BinaryExpr, bool) {
	var op js.TokenType
	switch expr.Op {
	case js.EqEqToken, js.EqEqEqToken:
		op = js.GtToken
	case js.NotEqToken, js.NotEqEqToken:
		op = js.LtToken
	default:
		return nil, false
	}

	// typeof x == "undefined"  =>  typeof x > "u"
	if isTypeof(expr.X) && isUndefinedString(expr.Y) {
		return &js.BinaryExpr{op, expr.X, &js.LiteralExpr{js.StringToken, uBytes}}, true
	}
	// "undefined" == typeof x  =>  "u" < typeof x
	if isUndefinedString(expr.X) && isTypeof(expr.Y) {
		if op == js.GtToken {
			op = js.LtToken
		} else {
			op = js.GtToken
		}
		return &js.BinaryExpr{op, &js.LiteralExpr{js.StringToken, uBytes}, expr.Y}, true
	}
	return nil, false
}
