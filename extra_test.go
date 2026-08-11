package cssmash

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"cssmash/parse"
	"github.com/tdewolff/test"
)

type passthroughMinifier struct{}

func (passthroughMinifier) Minify(_ *M, w io.Writer, r io.Reader, _ map[string]string) error {
	_, err := io.Copy(w, r)
	return err
}

type failingMinifier struct{}

func (failingMinifier) Minify(_ *M, _ io.Writer, _ io.Reader, _ map[string]string) error {
	return errors.New("minifier failed")
}

func TestAddRegexpReplace(t *testing.T) {
	m := New()
	pattern := regexp.MustCompile(`^text/`)
	var called int
	m.AddRegexp(pattern, MinifierFunc(func(_ *M, w io.Writer, r io.Reader, _ map[string]string) error {
		called = 1
		return nil
	}))
	m.AddRegexp(pattern, MinifierFunc(func(_ *M, w io.Writer, r io.Reader, _ map[string]string) error {
		called = 2
		return nil
	}))
	if len(m.pattern) != 1 {
		t.Errorf("expected pattern replacement, got %d patterns", len(m.pattern))
	}
	if err := m.Minify("text/plain", io.Discard, bytes.NewBufferString("x")); err != nil {
		t.Fatal(err)
	}
	test.T(t, called, 2)
}

func TestAddFuncRegexpReplace(t *testing.T) {
	m := New()
	pattern := regexp.MustCompile(`^text/`)
	var called int
	m.AddFuncRegexp(pattern, func(_ *M, w io.Writer, r io.Reader, _ map[string]string) error {
		called = 1
		return nil
	})
	m.AddFuncRegexp(pattern, func(_ *M, w io.Writer, r io.Reader, _ map[string]string) error {
		called = 2
		return nil
	})
	if len(m.pattern) != 1 {
		t.Errorf("expected pattern replacement, got %d patterns", len(m.pattern))
	}
	if err := m.Minify("text/plain", io.Discard, bytes.NewBufferString("x")); err != nil {
		t.Fatal(err)
	}
	test.T(t, called, 2)
}

func TestMatchPattern(t *testing.T) {
	m := New()
	pattern := regexp.MustCompile(`^text/`)
	m.AddRegexp(pattern, passthroughMinifier{})

	mimetype, params, minifier := m.Match("text/plain; charset=utf-8")
	test.T(t, mimetype, pattern.String())
	test.T(t, params["charset"], "utf-8")
	if minifier == nil {
		t.Error("expected minifier match")
	}

	_, _, minifier = m.Match("application/json")
	if minifier != nil {
		t.Error("expected no match")
	}
}

func TestBytesStringNotExist(t *testing.T) {
	m := New()
	b, err := m.Bytes("application/none", []byte("input"))
	test.T(t, err, ErrNotExist)
	test.T(t, string(b), "input")

	s, err := m.String("application/none", "input")
	test.T(t, err, ErrNotExist)
	test.T(t, s, "input")
}

func TestWriterDoubleClose(t *testing.T) {
	m := New()
	m.Add("text/plain", passthroughMinifier{})

	w := &bytes.Buffer{}
	z := m.Writer("text/plain", w)
	if _, err := z.Write([]byte("test")); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	test.T(t, w.String(), "test")
}

func TestWriterMinifyError(t *testing.T) {
	m := New()
	m.Add("text/plain", failingMinifier{})

	w := &bytes.Buffer{}
	z := m.Writer("text/plain", w)
	_, _ = z.Write([]byte("test")) // may fail when the minifier closes the pipe early
	err := z.Close()
	if err == nil || err.Error() != "minifier failed" {
		t.Errorf("expected minifier error, got %v", err)
	}
}

func TestMiddlewareWithError(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("test")) // runs in the minifier goroutine; ignore pipe errors
	})

	t.Run("success", func(t *testing.T) {
		m := New()
		m.Add("text/plain", passthroughMinifier{})
		var gotErr error
		h := m.MiddlewareWithError(next, func(w http.ResponseWriter, r *http.Request, err error) {
			gotErr = err
		})
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/test.txt", nil))
		if gotErr != nil {
			t.Errorf("unexpected error: %v", gotErr)
		}
		test.T(t, rr.Body.String(), "test")
	})

	t.Run("error", func(t *testing.T) {
		m := New()
		m.Add("text/plain", failingMinifier{})
		var gotErr error
		h := m.MiddlewareWithError(next, func(w http.ResponseWriter, r *http.Request, err error) {
			gotErr = err
		})
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/test.txt", nil))
		if gotErr == nil || gotErr.Error() != "minifier failed" {
			t.Errorf("expected minifier error, got %v", gotErr)
		}
	})
}

func TestUpdateErrorPosition(t *testing.T) {
	input := parse.NewInputString("line1\nline2\nerror")
	defer input.Restore()

	perr := &parse.Error{Message: "fail", Line: 1, Column: 1}
	err := UpdateErrorPosition(perr, input, 12) // offset of "error"
	perr2, ok := err.(*parse.Error)
	if !ok {
		t.Fatal("expected *parse.Error")
	}
	test.T(t, perr2.Line, 3)
	test.T(t, perr2.Column, 1)

	plain := errors.New("plain")
	if err := UpdateErrorPosition(plain, input, 0); err != plain {
		t.Error("expected plain error to pass through")
	}
}
