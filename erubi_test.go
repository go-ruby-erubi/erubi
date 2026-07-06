package erubi

import (
	"regexp"
	"testing"
)

func TestEngineAccessors(t *testing.T) {
	e := NewEngine("hi <%= x %>", Options{Filename: "t.erb", BufVar: "buf"})
	if got := e.Filename(); got != "t.erb" {
		t.Errorf("Filename() = %q, want %q", got, "t.erb")
	}
	if got := e.BufVar(); got != "buf" {
		t.Errorf("BufVar() = %q, want %q", got, "buf")
	}
	if e.Src() == "" {
		t.Error("Src() is empty")
	}

	// Defaults: no filename, default buffer variable.
	d := NewEngine("x", Options{})
	if d.Filename() != "" {
		t.Errorf("default Filename() = %q, want empty", d.Filename())
	}
	if d.BufVar() != "_buf" {
		t.Errorf("default BufVar() = %q, want _buf", d.BufVar())
	}
}

func TestCaptureEndAccessors(t *testing.T) {
	e := NewCaptureEndEngine("<%|= f do %>x<%| end %>", Options{Filename: "c.erb"})
	if e.Filename() != "c.erb" {
		t.Errorf("Filename() = %q, want c.erb", e.Filename())
	}
	if e.BufVar() != "_buf" {
		t.Errorf("BufVar() = %q, want _buf", e.BufVar())
	}
}

func TestHTMLEscape(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"plain text", "plain text"},
		{"héllo 😀 unicode", "héllo 😀 unicode"},
		{"a & b", "a &amp; b"},
		{"<a>", "&lt;a&gt;"},
		{`"q"`, "&quot;q&quot;"},
		{"it's", "it&#39;s"},
		{`<a href="x">it's & more</a>`, `&lt;a href=&quot;x&quot;&gt;it&#39;s &amp; more&lt;/a&gt;`},
	}
	for _, c := range cases {
		if got := HTMLEscape(c.in); got != c.want {
			t.Errorf("HTMLEscape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInvalidIndicatorPanic(t *testing.T) {
	// A custom regexp (4 groups, like the built-ins) that produces the unknown
	// "@" indicator drives the base engine's handle into its panic path.
	re := regexp.MustCompile(`(?s)<%(@|=)?(.*?)([-=])?%>([ \t]*\r?\n)?`)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on invalid indicator")
		}
		err, ok := r.(*InvalidIndicatorError)
		if !ok {
			t.Fatalf("panic value = %T, want *InvalidIndicatorError", r)
		}
		if err.Indicator != "@" {
			t.Errorf("Indicator = %q, want @", err.Indicator)
		}
		if got := err.Error(); got != "erubi: invalid indicator: @" {
			t.Errorf("Error() = %q", got)
		}
	}()
	NewEngine("<%@ foo %>", Options{Regexp: re})
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", ""); got != "" {
		t.Errorf("firstNonEmpty(all empty) = %q, want empty", got)
	}
	if got := firstNonEmpty("", "b", "c"); got != "b" {
		t.Errorf("firstNonEmpty = %q, want b", got)
	}
}

// TestRegexpOverrideAccepted checks a caller-supplied regexp equal to the
// default is honored (exercises the Regexp!=nil resolve branch without a panic).
func TestRegexpOverrideAccepted(t *testing.T) {
	custom := regexp.MustCompile(`(?s)<%(={1,2}|-|#|%)?(.*?)([-=])?%>([ \t]*\r?\n)?`)
	a := NewEngine("hi <%= x %>", Options{Regexp: custom}).Src()
	b := NewEngine("hi <%= x %>", Options{}).Src()
	if a != b {
		t.Errorf("custom-regexp src differs:\n got=%q\nwant=%q", a, b)
	}
}
