# Issues discovered in JSON5 handling v0.0.1

While the `Scanner` and `Decoder` aim to support JSON5 the latest version
has some serious flaws. In particular, the `Strict` JSON5 mode incorrectly
implements the basic JSON5 rules for quoted strings and line breaks, as well
as the correct handling of the limitations on identifier keys. In addition,
the `Json` mode has stricter requirements that are not fully aligned with
standard JSON.


## Reject unescaped line breaks in quoted strings outside Relaxed mode

With SetMode(Strict), a quoted string containing a literal LF or CR still
decodes successfully, although JSON5 prohibits unescaped line breaks. Validate
quoted content before normalization when Relaxed is disabled, including in Json
mode. Apply the same rule to the position-tracking and alternative quoted
scanners.

Is this claim correct and matching the JSON5 standard?

**Yes, for literal LF and CR.** The claim is correct about both the
implementation and the JSON5 requirement, with two qualifications.

- `Strict` is documented to follow JSON5 in `common.go:22`.
- `quoted` and `quoted_` currently copy literal line breaks normally in
  `scanner.go:1899` and `scanner.go:1971`.
- `quotedx` and `quoted_x` do likewise in `scanner_ext.go:70` and
  `scanner_ext.go:204`.
- Tests explicitly encode the incorrect acceptance of literal LF under `Strict`
  for single- and double-quoted strings in `scanner_test.go:1564` and
  `scanner_test.go:1726`.

The [JSON5 specification](https://spec.json5.org/#strings) excludes line
terminators from unescaped string characters. Both LF and CR must therefore be
rejected in `Strict` and `Json` modes.


### Qualifications

1. JSON5 permits escaped line continuations: `\` followed by LF, CR, CRLF,
   U+2028, or U+2029. The backslash and terminator are omitted from the
   resulting string. The proposed fix should preserve this behavior.
2. JSON5 specifically permits unescaped U+2028 and U+2029, unlike LF and CR,
   although it recommends a parser warning. Therefore, do not broadly reject
   every character recognized as a `uspace` line terminator.
3. `Json` should reject literal LF/CR even if combined with the `Relaxed` bit.
   A condition such as `s.mode&Relaxed == 0 || s.mode&Json != 0` would express
   that precedence.
4. A complete conformance fix should also handle JSON5 line continuation
   correctly. Currently, an actual newline after `\` falls through to the
   “unknown escape” path and preserves the backslash/newline instead of
   removing both.

So the AI’s recommendation is sound specifically for bare LF/CR, and it should
cover `quoted`, `quoted_`, `quotedx`, and `quoted_x`. The phrase “all unescaped
line breaks” needs narrowing to avoid incorrectly rejecting U+2028/U+2029.


## Accept JSON5 identifier keys in Strict mode

Unquoted IdentifierName property keys are valid JSON5, so SetMode(Strict)
should accept {foo:1} and {true:1}. This Relaxed-only gate, together with
the scanner's Relaxed-only plain-token fallback, rejects both valid
documents. Add context-aware identifier scanning for ObjectKey and accept
valid JSON5 identifiers outside Json mode without enabling relaxed unquoted
values.

Seems I got the quoting rules wrong for object keys also in the documentation.
Does JSON5 by default supports unquoted string values? or did I get this also
wrong. Please enumerate all my quoting mistakes. It also looks like I missed
the restriction on allowed key characters. Can you elaborate on how they
differ from the content strings?


The other AI is correct about the key behavior. JSON5 supports unquoted
**identifier keys** in normal JSON5 mode, but it does not support unquoted
string values.


### Correct Syntax Rules

| Position | Strict JSON5 | Relaxed extension |
| --- | --- | --- |
| Object key | Quoted string or IdentifierName | Arbitrary plain key forms |
| String value | Must be single- or double-quoted | May be an unquoted value |
| Keyword value | `true`, `false`, `null` are literals, not strings | Same |
| `Infinity` / `NaN` | Numeric values, not strings | Same |

Thus:

```text
{foo: 1}       // valid JSON5
{true: 1}      // valid JSON5: reserved words are valid property names
{"foo": 1}     // valid JSON5
{'foo': 1}     // valid JSON5

{1: 2}         // invalid JSON5 unless relaxed
{foo-bar: 1}   // invalid JSON5 unless relaxed
{foo: hello}   // invalid JSON5 unless relaxed
```

Bare `true` in value position means the boolean `true`; it is not the
string `"true"`.


### Your Quoting Mistakes

1. **Identifier keys were omitted from the JSON5 feature list.**

   `README.md:74` lists only single- and double-quoted keys. It should
   additionally list unquoted IdentifierName keys.

2. **Standard identifier keys were classified as Relaxed.**

   `common.go:25`, `decoder.go:124`, and `README.md:129` associate unquoted
   object keys with Relaxed mode.

   Strict JSON5 already permits a restricted subset of unquoted keys. Relaxed
   should instead mean keys beyond that subset: numeric keys, hyphenated keys,
   keys with internal spaces, and similar custom syntax.

3. **The bare-string bullet combines two different features.**

   `README.md:83` says “Unquoted string keys and values” are relaxed features.
    For values this is correct, but for keys only non-IdentifierName forms are
    relaxed.

4. **The claimed reserved-key restriction is wrong.**

   JSON5 object keys use IdentifierName, which permits reserved words.
   Therefore, all of these are valid JSON5 keys:

   ```text
   {true: 1, false: 2, null: 3, Infinity: 4, NaN: 5}
   ```

   They only have special meanings in value position.

   The Relaxed decoder tests currently accept an even broader set, including
   numeric and complex-looking keys, in `decoder_test.go:994`.

5. **The phrase “special characters” is too broad to describe the key grammar.**

   Relaxed bare text is scanned until a JSON delimiter in `scanner.go:2038`. It
   can consequently accept spaces within text and various punctuation, subject
   to the delimiter behavior. That is not an appropriate description of JSON5
   IdentifierName.

6. **The documented Unicode escape examples are not JSON5 syntax.**

   `README.md:78` shows:

   ```text
   \u{1F600}
   \U0X1F4A9
   ```

   Neither is standard JSON5 Unicode escaping. JSON5 uses `\uXXXX`, or a
   surrogate pair for characters outside the BMP:

   ```text
   "\uD83D\uDE00"
   ```

   The implementation’s `\uXXXX` handling matches that model in
   `scanner.go:2254`. A literal UTF-8 emoji is also valid in a quoted string.


### Identifier Keys Versus String Content

A JSON5 IdentifierName is not “arbitrary text without quotes.” Its first
character must be:

- A Unicode letter
- `$`
- `_`
- A Unicode escape resolving to one of the valid starting characters

Subsequent characters may additionally include:

- Decimal digits
- Unicode combining marks
- Unicode connector punctuation
- `U+200C` zero-width non-joiner
- `U+200D` zero-width joiner
- Appropriate Unicode escapes

Examples:

```text
{foo: 1}
{_foo: 1}
{$foo: 1}
{café: 1}
{caf\u00e9: 1}
{true: 1}
```

Invalid unless quoted:

```text
{1st: 1}
{foo-bar: 1}
{foo.bar: 1}
{first name: 1}
```

Quoted string keys and string values use the string grammar, allowing digits,
spaces, punctuation, keywords, and other content without identifier
restrictions:

```text
{"1st": 1}
{"foo-bar": 1}
{"first name": 1}
{"": 1}
```

They may use either quote type. Their surrounding quote must be escaped, as
must backslashes and applicable control characters; the other quote type does
not need escaping.


### Required Implementation Direction

In Strict JSON5 mode:

- Scan `ObjectKey` using IdentifierName rules.
- Accept resulting identifier tokens regardless of whether the general scanner
  classified them as `Plain`, `True`, `False`, `Null`, `Infinity`, `NaN`, or an
  identifier-like ambiguity.
- Continue accepting single- and double-quoted keys.
- Reject scalar-looking keys such as `1` and `0x10`.
- Do not route arbitrary `Plain` values through Strict decoding.
- Preserve the stricter `Json` rule that object keys must be double-quoted.

The current Relaxed-only branches in `decoder.go:208` are therefore part of the
bug. The scanner must also avoid blindly allowing arbitrary plain text as a
Strict key: context-aware identifier validation is important.


## Reject JSON5 syntax when decoding in Json mode

The new Json checks reject single-quoted/plain strings, but other JSON5-only
syntax still succeeds: SetMode(Json) accepts 0x10, [NaN,], and /*comment*/true.
In particular, comments are already discarded by scanner.skip, so the decoder
cannot enforce their prohibition afterward. Add mode-aware lexical validation
and delimiter-state checks so Json actually rejects non-JSON input.

While this is certainly also a right claim, it is currently a limitation not
fully enforced and instead marked as insufficiently tested. Will keep this
comment open for future addressing.


## Keep case-insensitive aliases separate from exact field names

Once any field enables case:ignore, this folded lookup also matches
case-sensitive primary names. For a struct with an ordinary ID field and a
different field tagged json:",case:ignore", Lookup("id") folds to "ID" and
incorrectly selects the ordinary field. Keep folded aliases separate from exact
primary names, or track whether the selected field permits folding, including
in the delimiter-normalized path.

While this is a fair point, it makes the component much more heavy weighted and
slower. Will keep this open as technical debt to be addressed later.
