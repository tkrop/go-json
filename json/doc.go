// Package json implements a high-performance JSON5 parser.
//
// The parser is designed for speed and low memory usage, and it supports
// JSON5 features such as comments and trailing commas.
//
//nolint:godoclint // Mock package generates docs.
package json

//revive:disable:line-length-limit // go:generate line length.

//go:generate mockgen -package=json -destination=mock_scanner_test.go -source=scanner.go

//revive:enable:line-length-limit
