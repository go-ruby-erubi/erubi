package erubi

import "strings"

// Engine is the compiled result for a template, mirroring Erubi::Engine. Its
// Src is the frozen Ruby source that, when eval'd against a binding, renders the
// template identically to the erubi gem.
type Engine struct {
	src      string
	filename string
	bufvar   string
}

// Src returns the compiled Ruby source (erubi's Engine#src).
func (e *Engine) Src() string { return e.src }

// Filename returns the template filename, if one was given (erubi's #filename).
func (e *Engine) Filename() string { return e.filename }

// BufVar returns the buffer variable name used by the compiled source (erubi's
// #bufvar).
func (e *Engine) BufVar() string { return e.bufvar }

// NewEngine compiles input into an Engine, mirroring Erubi::Engine.new. It
// panics with *InvalidIndicatorError only if a caller-supplied Options.Regexp
// yields an unknown tag indicator.
func NewEngine(input string, opts Options) *Engine {
	return build(input, opts.resolve(false))
}

// builder carries the mutable compile state, mirroring the instance variables of
// Erubi::Engine during #initialize.
type builder struct {
	src                strings.Builder
	escape             bool
	escapefunc         string
	bufvar             string
	bufval             string
	bufstack           string
	trim               bool
	chainAppends       bool
	bufferOnStack      bool
	textEnd            string
	captureEnd         bool
	escapeCapture      bool
	yieldReturnsBuffer bool
}

// build runs the full erubi compile pipeline for a resolved configuration.
func build(input string, r resolved) *Engine {
	b := &builder{
		escape:             r.escape,
		bufvar:             r.bufvar,
		bufval:             r.bufval,
		bufstack:           r.bufstack,
		trim:               r.trim,
		chainAppends:       r.chainAppends,
		textEnd:            r.textEnd,
		captureEnd:         r.captureEnd,
		escapeCapture:      r.escapeCapture,
		yieldReturnsBuffer: r.yieldReturnsBuffer,
	}

	// src begins with the initial value, then the optional magic comment, the
	// optional begin/ensure prologue, the escape-func hoist, then the preamble.
	b.src.WriteString(r.src)
	if r.freeze {
		b.src.WriteString("# frozen_string_literal: true\n")
	}
	if r.ensure {
		b.src.WriteString("begin; __original_outvar = " + r.bufvar)
		if isInstanceVar(r.bufvar) {
			b.src.WriteString("; ")
		} else {
			b.src.WriteString(" if defined?(" + r.bufvar + "); ")
		}
	}
	switch {
	case r.escapefuncOpt != "":
		b.escapefunc = r.escapefuncOpt
	case r.escape:
		b.escapefunc = "__erubi.h"
		b.src.WriteString("__erubi = ::Erubi; ")
	default:
		b.escapefunc = "::Erubi.h"
	}
	b.src.WriteString(r.preamble)

	b.scan(input, r)

	// Ensure the body ends with a newline before the postamble. The preamble is
	// always non-empty, so the source is never empty here.
	s := b.src.String()
	if s[len(s)-1] != '\n' {
		b.src.WriteByte('\n')
	}
	b.addPostamble(r.postamble)
	if r.ensure {
		b.src.WriteString("; ensure\n  " + r.bufvar + " = __original_outvar\nend\n")
	}

	return &Engine{src: b.src.String(), filename: r.filename, bufvar: r.bufvar}
}

// scan is the port of erubi's input.scan(regexp) loop.
func (b *builder) scan(input string, r resolved) {
	matches := r.re.FindAllStringSubmatchIndex(input, -1)
	pos := 0
	isBol := true

	for _, m := range matches {
		text := input[pos:m[0]]
		pos = m[1]

		hasInd := m[2] != -1
		indicator := ""
		if hasInd {
			indicator = input[m[2]:m[3]]
		}
		code := ""
		if m[4] != -1 {
			code = input[m[4]:m[5]]
		}
		tailch := ""
		tailchPresent := m[6] != -1
		if tailchPresent {
			tailch = input[m[6]:m[7]]
		}
		rspace := ""
		if m[8] != -1 {
			rspace = input[m[8]:m[9]]
		}

		ch := ""
		if hasInd {
			ch = indicator[:1]
		}

		// Compute the leading-whitespace capture (lspace); nil means "absent",
		// while a non-nil empty string is a meaningful present value.
		var lspace *string
		if ch != "=" {
			switch {
			case text == "":
				if isBol {
					lspace = strptr("")
				}
			case text[len(text)-1] == '\n':
				lspace = strptr("")
			default:
				if ri := strings.LastIndexByte(text, '\n'); ri >= 0 {
					if tail := text[ri+1:]; isSpaceTab(tail) {
						lspace = strptr(tail)
						text = text[:ri+1]
					}
				} else if isBol && isSpaceTab(text) {
					lspace = strptr(text)
					text = ""
				}
			}
		}

		isBol = rspace != ""

		b.addText(text)
		switch {
		case ch == "=":
			if tailchPresent {
				rspace = ""
			}
			b.addExpression(indicator, code)
			if rspace != "" {
				b.addText(rspace)
			}
		case ch == "" || ch == "-":
			if b.trim && lspace != nil && rspace != "" {
				b.addCode(*lspace + code + rspace)
			} else {
				if lspace != nil {
					b.addText(*lspace)
				}
				b.addCode(code)
				if rspace != "" {
					b.addText(rspace)
				}
			}
		case ch == "#":
			n := strings.Count(code, "\n")
			if rspace != "" {
				n++
			}
			if b.trim && lspace != nil && rspace != "" {
				b.addCode(strings.Repeat("\n", n))
			} else {
				if lspace != nil {
					b.addText(*lspace)
				}
				b.addCode(strings.Repeat("\n", n))
				if rspace != "" {
					b.addText(rspace)
				}
			}
		case ch == "%":
			ls := ""
			if lspace != nil {
				ls = *lspace
			}
			b.addText(ls + r.literalPrefix + code + tailch + r.literalPostfix + rspace)
		default:
			b.handle(indicator, code, tailchPresent, rspace, lspace)
		}
	}
	b.addText(input[pos:])
}

// addText appends a literal text chunk to the buffer (erubi's #add_text).
func (b *builder) addText(text string) {
	if text == "" {
		return
	}
	esc := escapeTextLiteral(text)
	b.withBuffer(func() {
		b.src.WriteString(" << '")
		b.src.WriteString(esc)
		b.src.WriteString(b.textEnd)
	})
}

// addCode appends Ruby code to the source (erubi's #add_code).
func (b *builder) addCode(code string) {
	b.terminateExpression()
	b.src.WriteString(code)
	if code == "" || code[len(code)-1] != '\n' {
		b.src.WriteByte(';')
	}
	b.bufferOnStack = false
}

// addExpression appends an expression tag's result, escaped or not per the
// indicator XOR the escape flag (erubi's #add_expression).
func (b *builder) addExpression(indicator, code string) {
	if (indicator == "=") != b.escape {
		b.addExpressionResult(code)
	} else {
		b.addExpressionResultEscaped(code)
	}
}

// addExpressionResult appends an unescaped expression (erubi's
// #add_expression_result).
func (b *builder) addExpressionResult(code string) {
	b.withBuffer(func() {
		b.src.WriteString(" << (")
		b.src.WriteString(code)
		b.src.WriteString(").to_s")
	})
}

// addExpressionResultEscaped appends an escaped expression (erubi's
// #add_expression_result_escaped).
func (b *builder) addExpressionResultEscaped(code string) {
	b.withBuffer(func() {
		b.src.WriteString(" << ")
		b.src.WriteString(b.escapefunc)
		b.src.WriteString("((")
		b.src.WriteString(code)
		b.src.WriteString("))")
	})
}

// addPostamble appends the postamble (erubi's #add_postamble).
func (b *builder) addPostamble(postamble string) {
	b.terminateExpression()
	b.src.WriteString(postamble)
}

// withBuffer wraps a buffer append, honoring chain_appends (erubi's #with_buffer).
func (b *builder) withBuffer(yield func()) {
	if b.chainAppends {
		if !b.bufferOnStack {
			b.src.WriteString("; ")
			b.src.WriteString(b.bufvar)
		}
		yield()
		b.bufferOnStack = true
	} else {
		b.src.WriteByte(' ')
		b.src.WriteString(b.bufvar)
		yield()
		b.src.WriteByte(';')
	}
}

// terminateExpression closes a pending chained expression (erubi's
// #terminate_expression).
func (b *builder) terminateExpression() {
	if b.chainAppends {
		b.src.WriteString("; ")
	}
}

// handle processes indicators the base engine does not know. The base engine
// raises; CaptureEndEngine handles the <%| , <%|= and <%|== capture tags
// (erubi's #handle).
func (b *builder) handle(indicator string, code string, tailchPresent bool, rspace string, lspace *string) {
	if b.captureEnd {
		switch indicator {
		case "|=", "|==":
			if tailchPresent {
				rspace = ""
			}
			if lspace != nil {
				b.addText(*lspace)
			}
			escapeCapture := (indicator == "|=") == b.escapeCapture
			b.terminateExpression()
			b.src.WriteString("begin; (" + b.bufstack + " ||= []) << " + b.bufvar +
				"; " + b.bufvar + " = " + b.bufval + "; " + b.bufstack + ".last << ")
			if escapeCapture {
				b.src.WriteString(b.escapefunc)
			}
			b.src.WriteString("((")
			b.src.WriteString(code)
			b.bufferOnStack = false
			if rspace != "" {
				b.addText(rspace)
			}
			return
		case "|":
			if tailchPresent {
				rspace = ""
			}
			if lspace != nil {
				b.addText(*lspace)
			}
			if b.yieldReturnsBuffer {
				b.terminateExpression()
				b.src.WriteString(" " + b.bufvar + "; ")
			}
			b.src.WriteString(code + ")).to_s; ensure; " + b.bufvar + " = " + b.bufstack + ".pop; end;")
			b.bufferOnStack = false
			if rspace != "" {
				b.addText(rspace)
			}
			return
		}
	}
	panic(&InvalidIndicatorError{Indicator: indicator})
}

// escapeTextLiteral backslash-escapes ' and \ for embedding text in a
// single-quoted Ruby literal (erubi's gsub(/['\\]/, '\\\\\&')).
func escapeTextLiteral(s string) string {
	if !strings.ContainsAny(s, "'\\") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		if c := s[i]; c == '\'' || c == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// isSpaceTab reports whether s is empty or all spaces/tabs (Ruby /\A[ \t]*\z/).
func isSpaceTab(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return false
		}
	}
	return true
}

// isInstanceVar reports whether bufvar is a single-@ instance variable name
// (Ruby /\A@[^@]/), used to skip the defined? guard in the ensure prologue.
func isInstanceVar(s string) bool {
	return len(s) >= 2 && s[0] == '@' && s[1] != '@'
}

// strptr returns a pointer to s.
func strptr(s string) *string { return &s }
