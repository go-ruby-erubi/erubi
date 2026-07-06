package erubi

import "regexp"

// Options configures NewEngine and NewCaptureEndEngine, mirroring the keyword
// arguments accepted by Erubi::Engine.new and Erubi::CaptureEndEngine.new.
//
// The tri-state boolean options are pointers so that "unset" is distinguishable
// from an explicit false, exactly as a missing Ruby hash key differs from a
// present false value. Use Bool to set them, e.g. Options{Trim: erubi.Bool(false)}.
// String options treat the zero value "" as "unset" (falling back to the
// erubi default), matching erubi's `properties[:x] || default` idiom.
type Options struct {
	// Escape makes <%= %> auto-HTML-escape (via EscapeFunc) and inverts the
	// <%= %> / <%== %> semantics. nil falls back to EscapeHTML, then false.
	Escape *bool
	// EscapeHTML is the lower-priority alias for Escape (erubi's :escape_html).
	EscapeHTML *bool
	// Trim enables erubi's whitespace-trim rules for <% %> / <%- -%> and the
	// comment tag. nil means the erubi default, true.
	Trim *bool
	// FreezeTemplateLiterals suffixes each literal text chunk with .freeze.
	// nil means the erubi default, true.
	FreezeTemplateLiterals *bool

	// Freeze prepends a "# frozen_string_literal: true" magic comment.
	Freeze bool
	// ChainAppends chains buffer appends (_buf << a << b) for speed.
	ChainAppends bool
	// Ensure wraps the template in begin/ensure restoring the previous BufVar.
	Ensure bool

	// EscapeFunc overrides the escape function name (erubi default resolves to
	// "::Erubi.h", or "__erubi.h" with a hoisted "__erubi = ::Erubi;" when
	// Escape is set). Empty means use that default.
	EscapeFunc string
	// BufVar names the buffer variable. Empty defaults to OutVar, then "_buf".
	BufVar string
	// OutVar is the lower-priority alias for BufVar (erubi's :outvar).
	OutVar string
	// BufVal is the buffer's initial value expression. Empty defaults to
	// "::String.new".
	BufVal string
	// Filename is the template filename, reported by Engine.Filename.
	Filename string
	// Preamble overrides the emitted preamble. Empty defaults to
	// "<BufVar> = <BufVal>;".
	Preamble string
	// Postamble overrides the emitted postamble. Empty defaults to
	// "<BufVar>.to_s\n".
	Postamble string
	// LiteralPrefix is emitted for an escaped opening delimiter (the <%% tag).
	// Empty defaults to "<%".
	LiteralPrefix string
	// LiteralPostfix is emitted for an escaped closing delimiter. Empty defaults
	// to "%>".
	LiteralPostfix string
	// Src is the initial source string the compiled output is appended to.
	Src string
	// Regexp overrides the scanning regexp. nil uses DefaultRegexp (or
	// CaptureEndRegexp for NewCaptureEndEngine).
	Regexp *regexp.Regexp

	// EscapeCapture (CaptureEndEngine only) makes <%|= %> escape by default and
	// <%|== %> not. nil defaults to the resolved Escape value.
	EscapeCapture *bool
	// YieldReturnsBuffer (CaptureEndEngine only) makes <%| %> insert the buffer
	// as an expression so the block yields the buffer.
	YieldReturnsBuffer bool
}

// Bool returns a pointer to b, for setting the tri-state Options fields.
func Bool(b bool) *bool { return &b }

// resolved is the fully-defaulted configuration the builder consumes.
type resolved struct {
	escape             bool
	trim               bool
	filename           string
	bufvar             string
	bufval             string
	re                 *regexp.Regexp
	literalPrefix      string
	literalPostfix     string
	preamble           string
	postamble          string
	chainAppends       bool
	textEnd            string
	escapefuncOpt      string
	freeze             bool
	ensure             bool
	src                string
	captureEnd         bool
	escapeCapture      bool
	yieldReturnsBuffer bool
	bufstack           string
}

// resolve applies erubi's defaulting rules. captureEnd selects the
// CaptureEndEngine defaults (regexp, bufstack, escape_capture).
func (o Options) resolve(captureEnd bool) resolved {
	escape := false
	if o.Escape != nil {
		escape = *o.Escape
	} else if o.EscapeHTML != nil {
		escape = *o.EscapeHTML
	}

	r := resolved{
		escape:         escape,
		trim:           o.Trim == nil || *o.Trim,
		filename:       o.Filename,
		bufvar:         firstNonEmpty(o.BufVar, o.OutVar, "_buf"),
		literalPrefix:  firstNonEmpty(o.LiteralPrefix, "<%"),
		literalPostfix: firstNonEmpty(o.LiteralPostfix, "%>"),
		chainAppends:   o.ChainAppends,
		escapefuncOpt:  o.EscapeFunc,
		freeze:         o.Freeze,
		ensure:         o.Ensure,
		src:            o.Src,
		captureEnd:     captureEnd,
	}
	r.bufval = firstNonEmpty(o.BufVal, "::String.new")
	if o.FreezeTemplateLiterals == nil || *o.FreezeTemplateLiterals {
		r.textEnd = "'.freeze"
	} else {
		r.textEnd = "'"
	}
	r.preamble = firstNonEmpty(o.Preamble, r.bufvar+" = "+r.bufval+";")
	r.postamble = firstNonEmpty(o.Postamble, r.bufvar+".to_s\n")
	switch {
	case o.Regexp != nil:
		r.re = o.Regexp
	case captureEnd:
		r.re = CaptureEndRegexp
	default:
		r.re = DefaultRegexp
	}
	if captureEnd {
		r.bufstack = "__erubi_stack"
		r.yieldReturnsBuffer = o.YieldReturnsBuffer
		if o.EscapeCapture != nil {
			r.escapeCapture = *o.EscapeCapture
		} else {
			r.escapeCapture = escape
		}
	}
	return r
}

// firstNonEmpty returns the first non-empty string, mirroring Ruby's `a || b`.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
