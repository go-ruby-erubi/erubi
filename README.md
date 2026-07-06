<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-erubi/brand/main/social/go-ruby-erubi-erubi.png" alt="go-ruby-erubi/erubi" width="720"></p>

# erubi — go-ruby-erubi

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-erubi.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of the [erubi](https://github.com/jeremyevans/erubi)
gem** — **Rails' default ERB template engine** and the modern, frozen-string
successor to erubis. It is a template-to-**Ruby-source** compiler: given an ERB
template it emits the exact Ruby source that erubi's `Erubi::Engine#src` emits,
matching the erubi gem (1.13.1, MRI 4.0.5) **byte-for-byte**, so a downstream
Ruby evaluation renders identically to Rails.

It is the erubi backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby) (feeding
actionview's rendering seam via a later rbgo binding), but is a **standalone,
reusable** module with no dependency on the Ruby runtime.

> **What it is — and isn't.** Compiling a template to Ruby source (tag scanning,
> trim rules, the `<%% %%>` literals, frozen-string handling, the chained-append
> optimization, the capture-block variant) is fully deterministic and needs **no
> interpreter**, so it lives here as pure Go. The final `eval(src, binding)` that
> produces the rendered string **does** need a Ruby interpreter and stays in the
> consumer (e.g. rbgo) — this library compiles, the host evaluates. The escape
> helper `::Erubi.h` that the emitted source calls is likewise a host-supplied
> runtime helper; `HTMLEscape` documents and mirrors its semantics.

## Features

Faithful port of erubi's `lib/erubi.rb` `Engine#initialize` and
`lib/erubi/capture_end.rb`, validated against the `erubi` gem on every supported
platform:

- **All tag kinds** — `<% code %>` (execute), `<%= expr %>` (output, auto-escaped
  when `Escape`), `<%== expr %>` (the inverted-escape output), `<%# comment %>`,
  and the `<%% %%>` literal escapes.
- **Exact trim rules** — erubi's `<% %>` / `<%- -%>` leading-indent and
  trailing-newline handling (`Trim`, default on), the fiddly part, byte-matched.
- **Every option** — `escape` / `escape_html`, `escapefunc`, `trim`, `bufvar` /
  `outvar`, `bufval`, `freeze` (the `# frozen_string_literal: true` comment),
  `freeze_template_literals` (the `.freeze` suffix), `chain_appends`, `ensure`,
  `preamble` / `postamble`, `filename`, `regexp`, `literal_prefix` /
  `literal_postfix`, `src`.
- **`CaptureEndEngine`** — the `<%|= ... %> ... <%| end %>` block-capture variant
  Rails uses for `form_with`-style helpers, with `escape_capture` and
  `yield_returns_buffer`.
- **`::Erubi.h` semantics** — `HTMLEscape` (`&<>"'` → entities, `'` → `&#39;`),
  the escape the emitted source calls at render time.

CGO-free, dependency-free, **100% test coverage**, `gofmt` + `go vet` clean, and
green across the six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le,
s390x) plus WebAssembly.

## Install

```sh
go get github.com/go-ruby-erubi/erubi
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/go-ruby-erubi/erubi"
)

func main() {
	e := erubi.NewEngine("Hello <%= name %>!\n", erubi.Options{})
	fmt.Printf("%q\n", e.Src())
	// "_buf = ::String.new; _buf << 'Hello '.freeze; _buf << ( name ).to_s; _buf << '!\n'.freeze;\n_buf.to_s\n"
	//
	// Hand e.Src() to a Ruby interpreter with ::Erubi in scope:
	//   eval(src, binding)  ->  "Hello World!\n"

	// Auto-HTML-escaping <%= %> (Rails' default), via ::Erubi.h:
	esc := erubi.NewEngine("<p><%= body %></p>", erubi.Options{Escape: erubi.Bool(true)})
	fmt.Printf("%q\n", esc.Src())

	fmt.Println(erubi.HTMLEscape(`<a href="x">it's</a>`))
	// &lt;a href=&quot;x&quot;&gt;it&#39;s&lt;/a&gt;
}
```

Capture blocks (`Erubi::CaptureEndEngine`), the form Rails uses for `form_with`:

```go
c := erubi.NewCaptureEndEngine("<%|= form do %><%= field %><%| end %>", erubi.Options{})
fmt.Println(c.Src())
```

## API

```go
type Options struct {
	// Tri-state booleans: nil = erubi default. Use erubi.Bool to set.
	Escape                 *bool // auto-escape <%= %>; invert <%= %>/<%== %>
	EscapeHTML             *bool // lower-priority alias for Escape
	Trim                   *bool // trim rules (default true)
	FreezeTemplateLiterals *bool // .freeze suffix on text chunks (default true)

	Freeze       bool // # frozen_string_literal: true magic comment
	ChainAppends bool // chain _buf << a << b
	Ensure       bool // wrap in begin/ensure restoring the buffer var

	EscapeFunc     string // default "::Erubi.h" / "__erubi.h"
	BufVar, OutVar string // buffer var name; default "_buf"
	BufVal         string // buffer initial value; default "::String.new"
	Filename       string
	Preamble       string // default "<bufvar> = <bufval>;"
	Postamble      string // default "<bufvar>.to_s\n"
	LiteralPrefix  string // default "<%"
	LiteralPostfix string // default "%>"
	Src            string // initial source string
	Regexp         *regexp.Regexp // default DefaultRegexp

	// CaptureEndEngine only:
	EscapeCapture      *bool // default = Escape
	YieldReturnsBuffer bool
}

func Bool(b bool) *bool

// NewEngine compiles a template, mirroring Erubi::Engine.new.
func NewEngine(input string, opts Options) *Engine

type Engine struct{ /* ... */ }
func (e *Engine) Src() string      // the compiled Ruby source
func (e *Engine) Filename() string
func (e *Engine) BufVar() string

// NewCaptureEndEngine mirrors Erubi::CaptureEndEngine.new (block capture).
func NewCaptureEndEngine(input string, opts Options) *CaptureEndEngine

func HTMLEscape(s string) string // ::Erubi.h (ERB::Escape#html_escape)
```

## Tests & coverage

The suite includes a **differential oracle**: a wide template corpus (every tag
kind, the literals, all trim situations, capture blocks, multiline/quoted/unicode
bodies) is compiled both here and by the `erubi` gem across a matrix of option
combinations, comparing `Erubi::Engine#src` / `CaptureEndEngine#src`
**byte-for-byte**. A second oracle eval's our emitted source under Ruby and
compares the **rendered** output to the gem's, escape on and off.

```sh
gem install erubi
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

The oracle tests skip themselves where `ruby` is not on `PATH` (e.g. the qemu
arch and wasm lanes), so the cross-arch builds still validate the compiler.

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-erubi/erubi authors.

## WebAssembly

Being pure Go (CGO=0), erubi is **pure logic** and compiles to **WebAssembly** —
both `GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm`
(WASI). CI builds both targets on every push, alongside the six 64-bit
native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```
