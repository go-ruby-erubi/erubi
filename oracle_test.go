package erubi

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// The differential oracle installs the erubi gem on the ubuntu/macos CI lanes
// and compiles every (template, scenario) pair both here and with the gem,
// comparing Erubi::Engine#src / Erubi::CaptureEndEngine#src byte-for-byte. The
// emitted Ruby source is the whole contract: identical source means a downstream
// Ruby eval renders identically to Rails.

// baseTemplates exercise every non-capture tag kind, the <%% / %%> literals, all
// trim situations, the lspace sub-cases, multiline/quoted/unicode bodies, and
// boundary cases.
var baseTemplates = []string{
	// Plain text (no tags): the pos==0 rest path, quotes/backslash escaping.
	"",
	"plain text",
	"line1\nline2\nline3",
	"trailing newline\n",
	"quote ' and back\\slash mix",
	"unicode héllo wörld 日本語 😀 end",
	"a # b #{x} #@v literal hashes",
	// Execute tags.
	"<% x = 1 %>",
	"<%-%>",
	"<%  %>",
	"a<% x = 1 %>b",
	"<% if true %>yes<% end %>",
	"<% [1,2].each do |i| %>i=<%= i %>\n<% end %>",
	// Expression tags (output).
	"<%= 1 + 2 %>",
	"x = <%= a %>, y = <%= b %>",
	"<%= \"quoted\" %>",
	"<%= name %>\n",
	"<%= x -%>\n",
	"<%== raw %>",
	"<%== r %><%= e %>",
	// Comment tags.
	"<%# a comment %>after",
	"before<%# c %>after",
	"<%# multi\nline\ncomment %>x",
	"  <%# c %>\n",
	"x<%# c %>\n",
	// Literal escapes.
	"a<%%b",
	"a<%% literal open",
	"<%%= not an expr %%>",
	"a%%>b",
	"<%= 1 %><%%literal%%><% y=2 %>",
	// Multiline mixes and newline-adjacent tags.
	"top\n<% x=1 %>\nmid\n<%= x %>\nbot\n",
	"<%= 1 %>\n<%= 2 %>\n<%= 3 %>\n",
	"a\n<% c %>\nb",
	"<% c %>\n",
	"\n<% c %>",
	// lspace sub-cases: indent before tag on its own line (trim on/off differ),
	// indent+tag not at bol, non-space prefix.
	"  <% x %>\n",
	"a\n  <% x %>\n",
	"a\nb<% x %>\n",
	"ab<% x %>\n",
	"a<% x %>b<% y %>",
	"<ul>\n<% items.each do |i| -%>\n  <li><%= i %></li>\n<% end -%>\n</ul>\n",
	// Edge: unterminated-looking text.
	"just a < percent like <%notclosed",
	"%>orphan close",
}

// captureTemplates exercise the CaptureEndEngine tags.
var captureTemplates = []string{
	"<%|= form do %>x<%| end %>",
	"<%|== form do %>y<%| end %>",
	"before <%|= f do %><%= inner %><%| end %> after",
	"<%|= a do %>1<%| end %>\n<%|== b do %>2<%| end %>\n",
	"<% outer %><%|= f do %>body<%| end %>",
	"plain <%= x %> then <%|= cap do %>z<%| end %>",
	// lspace (indentation) + rspace (trailing newline) around capture tags.
	"  <%|= f do %>\nx\n  <%| end %>\n",
	// tailch (-%>) on both capture tags trims the following newline.
	"<%|= f do -%>\nbody<%| end -%>\nrest",
	"  <%|== g do -%>\ninner<%| end -%>\n",
}

// scenario pairs a Go Options value with the equivalent Ruby keyword hash body.
type scenario struct {
	name     string
	capture  bool
	opts     Options
	rubyOpts string
}

var scenarios = []scenario{
	{name: "default", opts: Options{}},
	{name: "escape", opts: Options{Escape: Bool(true)}, rubyOpts: "escape: true"},
	{name: "escape_html", opts: Options{EscapeHTML: Bool(true)}, rubyOpts: "escape_html: true"},
	{name: "no_escape", opts: Options{Escape: Bool(false)}, rubyOpts: "escape: false"},
	{name: "trim_off", opts: Options{Trim: Bool(false)}, rubyOpts: "trim: false"},
	{name: "no_freeze_literals", opts: Options{FreezeTemplateLiterals: Bool(false)}, rubyOpts: "freeze_template_literals: false"},
	{name: "freeze", opts: Options{Freeze: true}, rubyOpts: "freeze: true"},
	{name: "chain", opts: Options{ChainAppends: true}, rubyOpts: "chain_appends: true"},
	{name: "chain_escape", opts: Options{ChainAppends: true, Escape: Bool(true)}, rubyOpts: "chain_appends: true, escape: true"},
	{name: "ensure", opts: Options{Ensure: true}, rubyOpts: "ensure: true"},
	{name: "ensure_ivar", opts: Options{Ensure: true, BufVar: "@out"}, rubyOpts: `ensure: true, bufvar: "@out"`},
	{name: "ensure_cvar", opts: Options{Ensure: true, BufVar: "@@out"}, rubyOpts: `ensure: true, bufvar: "@@out"`},
	{name: "bufvar", opts: Options{BufVar: "buf"}, rubyOpts: `bufvar: "buf"`},
	{name: "outvar", opts: Options{OutVar: "out2"}, rubyOpts: `outvar: "out2"`},
	{name: "bufval", opts: Options{BufVal: "[]"}, rubyOpts: `bufval: "[]"`},
	{name: "escapefunc", opts: Options{Escape: Bool(true), EscapeFunc: "myesc"}, rubyOpts: `escape: true, escapefunc: "myesc"`},
	{name: "preamble", opts: Options{Preamble: "BUF=+'';"}, rubyOpts: `preamble: "BUF=+'';"`},
	{name: "postamble", opts: Options{Postamble: "BUF\n"}, rubyOpts: `postamble: "BUF\n"`},
	{name: "literal_pp", opts: Options{LiteralPrefix: "[[", LiteralPostfix: "]]"}, rubyOpts: `literal_prefix: "[[", literal_postfix: "]]"`},
	{name: "src_init", opts: Options{Src: "PRE;"}, rubyOpts: `src: "PRE;"`},

	// CaptureEndEngine scenarios.
	{name: "capture_default", capture: true, opts: Options{}},
	{name: "capture_escape", capture: true, opts: Options{Escape: Bool(true)}, rubyOpts: "escape: true"},
	{name: "capture_escape_capture", capture: true, opts: Options{EscapeCapture: Bool(true)}, rubyOpts: "escape_capture: true"},
	{name: "capture_no_escape_capture", capture: true, opts: Options{Escape: Bool(true), EscapeCapture: Bool(false)}, rubyOpts: "escape: true, escape_capture: false"},
	{name: "capture_yield_returns_buffer", capture: true, opts: Options{YieldReturnsBuffer: true}, rubyOpts: "yield_returns_buffer: true"},
	{name: "capture_chain", capture: true, opts: Options{ChainAppends: true}, rubyOpts: "chain_appends: true"},
}

// oracleCase is one differential comparison request sent to Ruby.
type oracleCase struct {
	Tpl   string `json:"tpl"`
	Klass string `json:"klass"`
	Opts  string `json:"opts"`
}

func TestDifferentialSrcAgainstMRI(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not on PATH; skipping differential oracle")
	}

	var cases []oracleCase
	var got []string
	var meta []string

	for _, sc := range scenarios {
		templates := baseTemplates
		klass := "Engine"
		if sc.capture {
			templates = captureTemplates
			klass = "CaptureEndEngine"
		}
		for _, tpl := range templates {
			var src string
			if sc.capture {
				src = NewCaptureEndEngine(tpl, sc.opts).Src()
			} else {
				src = NewEngine(tpl, sc.opts).Src()
			}
			got = append(got, src)
			meta = append(meta, sc.name+" tpl="+tpl)
			cases = append(cases, oracleCase{Tpl: tpl, Klass: klass, Opts: sc.rubyOpts})
		}
	}

	want := mriSrc(t, cases)
	if len(want) != len(got) {
		t.Fatalf("oracle returned %d results, expected %d", len(want), len(got))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("SRC mismatch [%s]\n got=%q\nwant=%q", meta[i], got[i], want[i])
		}
	}
}

// mriSrc runs the erubi gem once over all cases, returning each src.
func mriSrc(t *testing.T, cases []oracleCase) []string {
	t.Helper()
	in, err := json.Marshal(cases)
	if err != nil {
		t.Fatalf("marshal cases: %v", err)
	}
	const script = `
require 'erubi'
require 'erubi/capture_end'
require 'json'
cases = JSON.parse($stdin.read)
out = cases.map do |c|
  opts = c['opts'].to_s.empty? ? {} : eval("{#{c['opts']}}")
  klass = c['klass'] == 'CaptureEndEngine' ? Erubi::CaptureEndEngine : Erubi::Engine
  klass.new(c['tpl'], **opts).src
end
$stdout.write(JSON.generate(out))
`
	cmd := exec.Command("ruby", "-e", script)
	cmd.Stdin = bytes.NewReader(in)
	outBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("ruby oracle failed: %v", errDetail(err))
	}
	var out []string
	if err := json.Unmarshal(outBytes, &out); err != nil {
		t.Fatalf("unmarshal oracle output: %v\nraw=%s", err, outBytes)
	}
	return out
}

// errDetail augments an exec error with the subprocess stderr, if available.
func errDetail(err error) string {
	if ee, ok := err.(*exec.ExitError); ok {
		return err.Error() + ": " + string(ee.Stderr)
	}
	return err.Error()
}

// renderCase is a template whose compiled source is safely evaluable given the
// listed locals, for end-to-end rendered-output comparison.
type renderCase struct {
	tpl  string
	vars map[string]string // local name -> ruby literal
}

var renderCases = []renderCase{
	{"Hello <%= name %>!", map[string]string{"name": `"World"`}},
	{"<% 3.times do |i| %><%= i %><% end %>", nil},
	{"sum=<%= a + b %>", map[string]string{"a": "2", "b": "5"}},
	{"a<%%b%%>c<%= x %>", map[string]string{"x": `"!"`}},
	{"<%# hidden %>shown <%= y %>", map[string]string{"y": "42"}},
	{"top\n<% if flag %>on<% else %>off<% end %>\nbot", map[string]string{"flag": "true"}},
	{"list:\n<% items.each do |it| -%>\n- <%= it %>\n<% end -%>\n", map[string]string{"items": `["x","y","z"]`}},
	{"héllo <%= who %> 😀", map[string]string{"who": `"monde"`}},
	{"<%= tag %>", map[string]string{"tag": `"<a href='x'>&</a>"`}},
	{"<%== tag %>", map[string]string{"tag": `"<b>&</b>"`}},
}

// TestDifferentialRenderAgainstMRI proves the emitted source renders identically
// to the erubi gem's own rendering, escape on and off, by eval'ing both under
// Ruby with ::Erubi in scope.
func TestDifferentialRenderAgainstMRI(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not on PATH; skipping render oracle")
	}
	for _, escape := range []bool{false, true} {
		opts := Options{}
		rubyOpts := ""
		if escape {
			opts.Escape = Bool(true)
			rubyOpts = "escape: true"
		}
		for _, rc := range renderCases {
			ours := NewEngine(rc.tpl, opts).Src()
			oursRendered := mriEval(t, ours, rc.vars)
			theirs := mriRender(t, rc.tpl, rubyOpts, rc.vars)
			if oursRendered != theirs {
				t.Errorf("RENDER mismatch escape=%v tpl=%q\n ours=%q\n mri =%q",
					escape, rc.tpl, oursRendered, theirs)
			}
		}
	}
}

// mriRender compiles+evals via the erubi gem itself.
func mriRender(t *testing.T, tpl, rubyOpts string, vars map[string]string) string {
	t.Helper()
	script := `require 'erubi'; ` + setupLocals(vars) +
		`opts = ARGV[0].empty? ? {} : eval("{#{ARGV[0]}}"); ` +
		`src = Erubi::Engine.new($stdin.binmode.read.force_encoding("UTF-8"), **opts).src; ` +
		`$stdout.binmode.write(eval(src))`
	return runRuby(t, tpl, script, rubyOpts)
}

// mriEval evals our already-compiled source under Ruby with ::Erubi available.
func mriEval(t *testing.T, src string, vars map[string]string) string {
	t.Helper()
	script := `require 'erubi'; ` + setupLocals(vars) +
		`$stdout.binmode.write(eval($stdin.binmode.read.force_encoding("UTF-8")))`
	return runRuby(t, src, script)
}

func setupLocals(vars map[string]string) string {
	var b strings.Builder
	for k, v := range vars {
		b.WriteString(k + " = " + v + "; ")
	}
	return b.String()
}

func runRuby(t *testing.T, stdin, script string, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"-e", script, "--"}, args...)
	cmd := exec.Command("ruby", cmdArgs...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ruby failed: %v\nstdin=%q", errDetail(err), stdin)
	}
	return string(out)
}
