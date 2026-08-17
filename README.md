
[![Build][build-badge]][build-link]
[![Quality][codeql-badge]][codeql-link]
[![Coverage][coveralls-badge]][coveralls-link]
[![Coverage][coverage-badge]][coverage-link]
[![Quality][quality-badge]][quality-link]
[![Report][report-badge]][report-link]
[![License][license-badge]][license-link]
[![Libraries][libs-badge]][libs-link]
[![Docs][docs-badge]][docs-link]
<!--
[![Security][security-badge]][security-link]
-->

[build-badge]: <https://github.com/tkrop/go-json/actions/workflows/build.yaml/badge.svg>
[build-link]: <https://github.com/tkrop/go-json/actions/workflows/build.yaml>

[codeql-badge]: <https://github.com/tkrop/go-json/actions/workflows/github-code-scanning/codeql/badge.svg?branch=main>
[codeql-link]: <https://github.com/tkrop/go-json/actions/workflows/github-code-scanning/codeql>

[coveralls-badge]: <https://coveralls.io/repos/github/tkrop/go-json/badge.svg?branch=main>
[coveralls-link]: <https://coveralls.io/github/tkrop/go-json?branch=main>

[coverage-badge]: <https://app.codacy.com/project/badge/Coverage/b2bb898346ae4bb4be6414cd6dfe4932>
[coverage-link]: <https://app.codacy.com/gh/tkrop/go-json/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_coverage>

[quality-badge]: <https://app.codacy.com/project/badge/Grade/b2bb898346ae4bb4be6414cd6dfe4932>
[quality-link]: <https://app.codacy.com/gh/tkrop/go-json/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_grade>

[report-badge]: <https://goreportcard.com/badge/github.com/tkrop/go-json>
[report-link]: <https://goreportcard.com/report/github.com/tkrop/go-json>

[license-badge]: <https://img.shields.io/badge/License-MIT-green.svg>
[license-link]: <https://opensource.org/licenses/MIT>

[libs-badge]: https://img.shields.io/librariesio/release/github/tkrop/go-json
[libs-link]: https://libraries.io/github/tkrop/go-json

[docs-badge]: <https://pkg.go.dev/badge/github.com/tkrop/go-json.svg>
[docs-link]: <https://pkg.go.dev/github.com/tkrop/go-json>

<!--
[release-badge]: <https://img.shields.io/github/release/tkrop/go-json.svg>
[release-link]: <https://github.com/tkrop/go-json/releases>

[security-badge]: https://snyk.io/test/github/tkrop/go-json/main/badge.svg
[security-link]: https://snyk.io/test/github/tkrop/go-json
-->

# Relaxed JSON5 parser

This is a relaxed and extended [JSON5][json5] parser that is heavily inspired
by the great work of Dave Chaney [pkg/json][json-pkg] and his article about
[Building a high-performance JSON parser][json-hp].

**Warning:** This work is not meant as a drop in replacement for the default
[encoding/json][json-enc] parser, and even if it currently provides a visibly
compatible interface it produces many essential but also subtle differences.

I have undertaken this journey in a trial to make it fit for fast parsing of
very short default configuration scripts and tags out of pure curiosity and
the lack of working alternatives in `go`. While the parser is now nearly
production ready, I am not sure yet where this journey ends.

Currently, the parser supports the [JSON5][json5] specification with braces,
brackets, commas, colons, and all other [JSON5 features][json5-features],
including:

* Single and double quoted keys in objects.
* Escape sequences in strings, including `\n`, `\t`, `\\`, etc.
* Single-line (`//`) and multi-line (`/* ... */`) comments.
* Integer, decimal, and hexadecimal numbers (e.g., `0x1E`).
* Unicode escape sequences in strings (e.g., `\u{1F600}`, `\U0X1F4A9`).

Besides, the parser also supports the following extra _relaxed_ and
_extended_ features:

* Unquoted string keys and values in objects and arrays, that are not reserved
  keywords (`true`, `false`, `null`, `NaN`, `Infinity`), and do not contain any
  leading or trailing whitespace or special characters.
* Complex numbers in values (e.g., `1+2i`).

Based on these additional features, the input can end up looking more like a
[`yaml`][yaml] document instead of a strict [JSON5][json5] document, but the
parser will still interpret all elements correctly. While the _relaxed_ and
_extended_ parsing is the default, these features can be disabled, so that the
parser will only accept strict [JSON5][json5] or [JSON][json] documents.

**Note:** The _relaxed_ and _extended_ `JSON5` parsing mode is absolutely
forgiving and never fails, but may produce invalid token series for the
`Decoder`.


<!--
For further optimizations I should have a look at [goccy/go-json][goccy].

[json-goccy]: <https://github.com/goccy/go-json> (high quality, optimized)
[json-shoobyban]: <https://github.com/shoobyban/json5> (active, docuemented)
[json-ojg]: <https://github.com/ohler55/ojg> (++ coverage, ++ features)
[json-compare]: <https://github.com/ohler55/compare-go-json> (=> ojg)

[json-awesome]: <https://github.com/avelino/awesome-go#json>

[json-furukawa]: <https://github.com/yosuke-furukawa/json5> (=> titanous)
[json-titanous]: <https://github.com/titanous/json5> (interesting => decoder)
[json-flynn]: <https://github.com/flynn/json5> (= titanous)
[json-json5]: <https://github.com/json5/json5-go> (dead - no value)
[json-barney]: <https://github.com/barney-ci/go-json5> (read => json)

[json-blog]: <https://www.cockroachlabs.com/blog/high-performance-json-parsing>
-->

[json]: <https://www.json.org/json-en.html>
[json5]: <https://spec.json5.org/>
[json5-features]: <https://spec.json5.org/#summary-of-features>
[json-pkg]: <https://github.com/pkg/json>
[json-enc]: <https://pkg.go.dev/encoding/json>
[json-hp]: <https://dave.cheney.net/high-performance-json.html>


## Architecture

The architecture of the JSON parser is based on the following high level
components:

* The `Reader` abstraction allows to read the JSON input data either from an
  `io.Reader` or directly from a static buffer while tracking the current
  scanner position. When using the `io.Reader`, the `Reader` can work together
  in two different modes:

  1. In a breathing mode (default), where it keeps a dynamic input buffer
     primarily containing the current token and its surrounding context, and
  2. In a growing mode where, it keeps the entire input data in memory and
     allows to access any part of it at any time.

* The `Scanner` abstraction allows to efficiently scan the input data provided
  by the `Reader` into tokens. The `Scanner` is coming in two main flavors with
  and without tracking of line and character position, as well as in multiple
  sub flavors for strict, relaxed, and extended `JSON5` and strict `JSON`
  parsing (strict `JSON` is not implemented yet).

* The `Printer` abstraction allows to consume a stream of tokens directly as
  provided by the `Scanner` back into a identical output byte stream providing
  a valid strict, relaxed, or extended `JSON5` document with proper indentation
  and comments (not implemented yet).

* The `Decoder` abstraction allows to directly decode the stream of tokens into
  Go objects using reflection. The `Decoder` comes in two flavors supporting a
  _native_ and a _precise_ type decoding using primitive or precise composite
  Go types.

  **Note:** Contrary to the `encoding/json` package, the `Decoder` requires by
  default exact names. You can enable case-insensitive matching by setting the
  `CaseIgnore` mode.

* The `Encoder` abstraction allows to encode Go objects into a stream of tokens
  that can be consumed by the `Printer` to produce a valid `JSON`, `JSON5`, or
  relaxed/extended `JSON5` document (not implemented yet).

* The `Filter` abstraction allows to dynamically filter a stream of tokens
  provided by the `Scanner` according to a specified filter function. The
  default filters allow to skip comments, whitespace, and other tokens that
  are not matching a specific `JSON`, `JSON5`, or relaxed/extended `JSON5`
  document standard (not implemented yet).

* The `Parser` abstraction allows to validate the stream of tokens provided by
  the `Scanner` and - if requested - to build an abstract syntax tree, that can
  be used for analysis and processing (not implemented yet).


## Future plans

The following features are planned for the future, but not yet implemented:

* Split the `Reader`, `Scanner`, and `Decoder` into separate packages, so that
  they can be used independently and reused in other projects, and advance
  tests to public interface testing.

* Create a non-releasing `Reader`, that allows to access the underlying data
  without releasing the buffer to enable permanent zero-copy decoding of the
  data (partially done).

* Create specialized `Decoder` implementations for different parsing modes and
  different default value types, i.e. big.Int and big.Float vs int64 and
  float64 (_precise_ vs _native_).

* Create an _extended_ and _relaxed_ [JSON5][json5]/[JSON][json] `Emitter`
  supporting the token parsing events of the _relaxed_ and _extended_
  [JSON5][json5] `Scanner` to output a patched or unchanged [JSON5][json5]
  data without ever creating a decoded object.

* Create a `Parser` that can parse JSON5 data into an abstract syntax tree,
  allowing for more advanced manipulation and analysis of the JSON5 structure.

* Create an `Encoder` that can encode Go objects into minimal _relaxed_ and
  _extended_ [JSON5][json5] output, that can be decoded by the `Decoder`
  without loss of information.

* Create [JSONPatch][json-patch] support to allow for efficient, on-the-fly
  patching of JSON data while scanning, parsing, or decoding it.


Open questions:

* Should we eliminate defensive error handling in the `Decoder` that can not
  happen due to the `Scanner` implementation and just panic in these cases?

[json-patch]: <https://datatracker.ietf.org/doc/html/rfc6902>


## Building

This project is using a custom build system called [go-make][go-make], that
provides default targets for most common tasks. Makefile rules are generated
based on the project structure and files for common tasks, to initialize,
build, test, and run the components in this repository.

To get started, run one of the following commands.

```bash
make help
make show-targets
```

Read the [go-make manual][go-make-man] for more information about targets
and configuration options.

**Not:** [go-make][go-make] installs `pre-commit` and `commit-msg`
[hooks][git-hooks] calling `make commit` to enforce successful testing and
linting and `make git-verify message` to validate whether the commit message
is following the [conventional commit][convent-commit] best practice.

[go-make]: <https://github.com/tkrop/go-make>
[go-make-man]: <https://github.com/tkrop/go-make/blob/main/MANUAL.md>
[git-hooks]: <https://git-scm.com/book/en/v2/Customizing-Git-Git-Hooks>
[convent-commit]: <https://www.conventionalcommits.org/en/v1.0.0/>


## Terms of usage

This software is open source under the MIT license. You can use it without
restrictions and liabilities. Please give it a star, so that I know. If the
project has more than 25 Stars, I will introduce semantic versions `v1`.


## Contributing

If you like to contribute, please create an issue and/or pull request with a
proper description of your proposal or contribution. I will review it and
provide feedback on it as fast as possible.


## Disclaimer

This software is developed with the help of AI following the highest human
standards. All actions executed by AI are carefully reviewed, counter-checked,
and corrected with the highest human standards and quality goals in mind. No
AI generate code is allowed to be merged or released without a careful human
reviews to prevent systematic degeneration of coding standards and code
quality.


## Acknowledgements

This work is inspired by the great work of Dave Chaney [pkg/json][json-pkg] and
his article about [Building a high-performance JSON parser][json-hp].
