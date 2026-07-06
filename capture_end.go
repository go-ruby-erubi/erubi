package erubi

// CaptureEndEngine is the compiled result of the capture-block ERB variant,
// mirroring Erubi::CaptureEndEngine. It supports capturing blocks via the
// <%|= %> and <%|== %> tags, explicitly ended with a <%| end %> block — the
// form Rails uses for form_with-style capture helpers. It embeds *Engine, so
// Src, Filename and BufVar are available directly.
type CaptureEndEngine struct {
	*Engine
}

// NewCaptureEndEngine compiles input into a CaptureEndEngine, mirroring
// Erubi::CaptureEndEngine.new. It installs CaptureEndRegexp (unless overridden by
// Options.Regexp) so the <%| , <%|= and <%|== tags are recognized, and honors
// the EscapeCapture and YieldReturnsBuffer options. It panics with
// *InvalidIndicatorError only if a caller-supplied Regexp yields an unknown
// indicator.
func NewCaptureEndEngine(input string, opts Options) *CaptureEndEngine {
	return &CaptureEndEngine{build(input, opts.resolve(true))}
}
