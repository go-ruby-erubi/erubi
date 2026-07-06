// Package erubi is a pure-Go (no cgo) reimplementation of the erubi gem —
// Rails' default ERB template engine and the modern, frozen-string successor to
// erubis. It is a template-to-Ruby-source COMPILER: given an ERB template it
// emits the exact Ruby source that erubi's Erubi::Engine#src would emit, matching
// the erubi gem (1.13.1, Ruby 4.0.5) byte-for-byte, so that a downstream Ruby
// evaluation (e.g. go-embedded-ruby/rbgo, feeding actionview's rendering seam)
// produces identical output.
//
// What it is NOT: the final eval of the compiled source against a binding needs
// a Ruby interpreter and is deliberately left to the consumer. This package
// compiles; the host evaluates. The escape helper ::Erubi.h that the emitted
// source calls is likewise a runtime helper the host supplies; HTMLEscape below
// documents and mirrors its semantics for parity and testing.
//
// The package faithfully ports erubi's lib/erubi.rb Engine#initialize: the
// <% %> (execute), <%= %> (output, auto-escaped when Escape), <%== %> (the
// inverted-escape output), <%# %> (comment) and <%% %%> literal tags, the trim
// rules for <% %>/<%- -%>, the frozen-string-literal handling, the chained-append
// optimization, the begin/ensure wrapping, and the preamble/postamble/regexp/
// literal-delimiter/escape-function overrides. CaptureEndEngine ports
// lib/erubi/capture_end.rb (the <%|= ... %> ... <%| end %> block-capture variant
// Rails uses for form_with-style helpers).
package erubi

import "regexp"

// DefaultRegexp is the scanner used by NewEngine, mirroring
// Erubi::Engine::DEFAULT_REGEXP. Ruby's /m flag (dot matches newline) maps to
// Go's (?s) flag.
var DefaultRegexp = regexp.MustCompile(`(?s)<%(={1,2}|-|#|%)?(.*?)([-=])?%>([ \t]*\r?\n)?`)

// CaptureEndRegexp is the scanner used by NewCaptureEndEngine, mirroring the
// regexp Erubi::CaptureEndEngine installs, adding the <%| , <%|= and <%|== tags.
var CaptureEndRegexp = regexp.MustCompile(`(?s)<%(\|?={1,2}|-|#|%|\|)?(.*?)([-=])?%>([ \t]*\r?\n)?`)

// InvalidIndicatorError is what NewEngine/NewCaptureEndEngine panics with when a
// custom Regexp yields a tag indicator the engine does not know how to handle,
// mirroring the ArgumentError raised by Erubi::Engine#handle. It is only
// reachable with a caller-supplied Regexp; the built-in regexps never produce an
// unknown indicator.
type InvalidIndicatorError struct{ Indicator string }

func (e *InvalidIndicatorError) Error() string {
	return "erubi: invalid indicator: " + e.Indicator
}

// HTMLEscape mirrors ::Erubi.h (ERB::Escape#html_escape): it replaces the five
// HTML-significant characters with their entity references (note "'" becomes
// "&#39;"). The compiled source emitted by this package calls ::Erubi.h at
// render time; this function documents and reproduces that helper's semantics so
// a host binding (or a test) can supply an identical escape.
func HTMLEscape(s string) string {
	// Fast path: nothing to escape.
	var n int
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&', '<', '>', '"', '\'':
			n++
		}
	}
	if n == 0 {
		return s
	}
	b := make([]byte, 0, len(s)+n*5)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			b = append(b, "&amp;"...)
		case '<':
			b = append(b, "&lt;"...)
		case '>':
			b = append(b, "&gt;"...)
		case '"':
			b = append(b, "&quot;"...)
		case '\'':
			b = append(b, "&#39;"...)
		default:
			b = append(b, s[i])
		}
	}
	return string(b)
}
