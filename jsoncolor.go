package jsoncolor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/amterp/color"
)

// DefaultFormatter is the default configuration used for colorizing JSON output
// by the package-level Marshal, MarshalIndent, and NewEncoder functions.
// Users can modify its fields before calling those functions or provide their
// own Formatter instance.
var DefaultFormatter = &Formatter{}

// Marshal works like encoding/json.Marshal but colorizes the resulting JSON
// output using the DefaultFormatter. Output is not indented.
func Marshal(v interface{}) ([]byte, error) {
	// Delegates to MarshalIndent with no prefix and no indent string.
	return MarshalIndent(v, "", "")
}

// MarshalIndent works like encoding/json.MarshalIndent but colorizes the
// resulting JSON output using the DefaultFormatter.
func MarshalIndent(v interface{}, prefix, indent string) ([]byte, error) {
	// Delegates to the version that allows specifying a custom formatter.
	return MarshalIndentWithFormatter(v, prefix, indent, DefaultFormatter)
}

// MarshalWithFormatter works like Marshal but uses the provided Formatter `f`
// for colorization rules.
// Note: This function does not indent its output; the Prefix and Indent fields
// within the provided Formatter `f` are ignored.
// Note: This function always enables HTML escaping; the EscapeHTML field
// within the provided Formatter `f` is ignored (and forced to true). To disable
// HTML escaping, use an Encoder and call SetEscapeHTML(false) on it.
func MarshalWithFormatter(v interface{}, f *Formatter) ([]byte, error) {
	// Delegates to MarshalIndentWithFormatter with no prefix and no indent string.
	// The indentation settings in `f` are effectively ignored here.
	return MarshalIndentWithFormatter(v, "", "", f)
}

// MarshalIndentWithFormatter works like MarshalIndent but uses the provided Formatter `f`
// for colorization rules.
// Note: The `prefix` and `indent` arguments provided to this function override
// the Prefix and Indent fields within the Formatter `f`, which are ignored.
// Note: This function always enables HTML escaping; the EscapeHTML field
// within the provided Formatter `f` is ignored (and forced to true). To disable
// HTML escaping, use an Encoder and call SetEscapeHTML(false) on it.
func MarshalIndentWithFormatter(v interface{}, prefix, indent string, f *Formatter) ([]byte, error) {
	// Create a buffer to hold the colorized JSON output.
	buf := &bytes.Buffer{}

	// Create an encoder specifically for this operation, associated with the buffer
	// and the provided formatter.
	enc := NewEncoderWithFormatter(buf, f)

	// Apply the specific indentation requested for this call, overriding any
	// defaults in the formatter `f`.
	enc.SetIndent(prefix, indent)

	// Ensure HTML characters are escaped. This is consistent with encoding/json's
	// MarshalIndent behavior and overrides the formatter's EscapeHTML setting.
	// Users wanting no escaping must use the Encoder directly.
	enc.SetEscapeHTML(true) // This is forced for Marshal* functions.

	// Perform the encoding and colorization.
	// `false` indicates not to add a trailing newline, matching encoding/json.MarshalIndent.
	err := enc.encode(v, false)
	if err != nil {
		return nil, err
	}

	// Return the contents of the buffer.
	return buf.Bytes(), nil
}

// Encoder works like encoding/json.Encoder but writes colorized JSON output
// to the underlying stream using a specified Formatter.
type Encoder struct {
	w io.Writer  // The output writer stream.
	f *Formatter // The configuration for colorization and indentation.
}

// NewEncoder creates a new Encoder that writes colorized JSON to `w`
// using the DefaultFormatter.
func NewEncoder(w io.Writer) *Encoder {
	return NewEncoderWithFormatter(w, DefaultFormatter)
}

// NewEncoderWithFormatter creates a new Encoder that writes colorized JSON to `w`
// using the provided Formatter `f`.
// Note: The initial value of `f.EscapeHTML` is ignored upon creation; HTML escaping
// is enabled by default. To disable it, call SetEscapeHTML(false) on the
// returned Encoder instance after creation.
func NewEncoderWithFormatter(w io.Writer, f *Formatter) *Encoder {
	if f == nil {
		panic("jsoncolor: cannot create Encoder with a nil Formatter")
	}
	// Clone the formatter to avoid modifying the original one passed by the caller,
	// especially since we forcefully set EscapeHTML below.
	clonedFormatter := f.clone()
	// Default behavior for encoders is to escape HTML, consistent with encoding/json.
	// The user can override this via SetEscapeHTML().
	clonedFormatter.setEscapeHTML(true)
	return &Encoder{
		w: w,
		f: clonedFormatter,
	}
}

// Encode writes the colorized JSON encoding of `v` to the Encoder's writer stream,
// followed by a newline character. This mimics the behavior of encoding/json.Encoder.Encode.
func (enc *Encoder) Encode(v interface{}) error {
	// `true` indicates that a trailing newline should be added after the JSON object.
	return enc.encode(v, true)
}

// SetIndent configures the Encoder to indent output, similar to
// encoding/json.Encoder.SetIndent. It sets the line prefix and the
// indentation string for each level. Setting empty strings disables indentation.
func (enc *Encoder) SetIndent(prefix, indent string) {
	enc.f.setIndent(prefix, indent)
}

// SetEscapeHTML specifies whether problematic HTML characters (<, >, &)
// should be escaped within JSON strings. The default is true. This mimics
// encoding/json.Encoder.SetEscapeHTML.
func (enc *Encoder) SetEscapeHTML(on bool) {
	enc.f.setEscapeHTML(on)
}

// encode is the internal method that performs the core logic:
// 1. Marshal the input `v` to standard JSON bytes.
// 2. Format (colorize and indent) those bytes to the Encoder's writer.
// The `terminateWithNewline` flag controls whether a final newline is added.
func (enc *Encoder) encode(v interface{}, terminateWithNewline bool) error {
	// Step 1: Get the standard, non-colorized JSON representation.
	plainJSONBytes, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("jsoncolor: failed to marshal input to standard JSON: %w", err)
	}

	// Step 2: Format the plain JSON bytes by adding colors and indentation.
	// This involves parsing the plain JSON and rewriting it with decorations.
	err = enc.f.format(enc.w, plainJSONBytes, terminateWithNewline)
	if err != nil {
		return fmt.Errorf("jsoncolor: failed to format/colorize JSON: %w", err)
	}

	return nil
}

// frame represents the state within a nested JSON structure (object or array)
// during the formatting process. It helps manage indentation and context
// (e.g., whether the next token is an object key or value).
type frame struct {
	object bool // True if the current frame represents a JSON object ({...}).
	field  bool // True if the next token expected within an object is a value (after key:). False if a key is expected.
	array  bool // True if the current frame represents a JSON array ([...]).
	empty  bool // True if the object or array is empty (e.g., {} or []).
	indent int  // The indentation level for this frame.
}

// inArray returns true if the current frame is a JSON array.
func (f *frame) inArray() bool {
	if f == nil {
		return false
	}
	return f.array
}

// inObject returns true if the current frame is a JSON object.
func (f *frame) inObject() bool {
	if f == nil {
		return false
	}
	return f.object
}

// inArrayOrObject returns true if the current frame is either a JSON array or object.
func (f *frame) inArrayOrObject() bool {
	if f == nil {
		return false
	}
	return f.object || f.array
}

// inField returns true if the formatter is inside an object and expects a value token
// (i.e., it has just processed the field name and the colon).
func (f *frame) inField() bool {
	if f == nil {
		return false
	}
	// Only true if we are in an object AND expecting a value (field=true).
	return f.object && f.field
}

// toggleField flips the state within an object frame between expecting a field name
// (field=false) and expecting a field value (field=true).
func (f *frame) toggleField() {
	if f == nil {
		return
	}
	// This should only be called when f.object is true.
	f.field = !f.field
}

// isEmpty returns true if the current frame represents an empty object or array.
func (f *frame) isEmpty() bool {
	if f == nil {
		return false
	}
	return (f.object || f.array) && f.empty
}

// SprintfFuncer is an interface wrapper around the `color` package's functionality.
// It defines a method that returns a function suitable for colorizing strings
// using fmt.Sprintf-style formatting. This allows different color settings
// (defined by types implementing this interface, like color.Color) to be used
// interchangeably by the Formatter.
type SprintfFuncer interface {
	// SprintfFunc returns a function that takes a format string and arguments
	// (like fmt.Sprintf) and returns the resulting string wrapped in the
	// appropriate ANSI color escape codes defined by the SprintfFuncer implementation.
	SprintfFunc() func(format string, a ...interface{}) string
}

// Default color settings using the `color` package.
// Users can override these by creating their own Formatter instance.
var (
	// DefaultSpaceColor defines the color for whitespace (spaces, newlines, tabs) used in indentation. Default is no color.
	DefaultSpaceColor = color.New()
	// DefaultCommaColor defines the color for the comma ',' separating elements. Default is bold.
	DefaultCommaColor = color.New(color.Bold)
	// DefaultColonColor defines the color for the colon ':' separating keys and values in objects. Default is bold.
	DefaultColonColor = color.New(color.Bold)
	// DefaultObjectColor defines the color for object delimiters '{' and '}'. Default is bold.
	DefaultObjectColor = color.New(color.Bold)
	// DefaultArrayColor defines the color for array delimiters '[' and ']'. Default is bold.
	DefaultArrayColor = color.New(color.Bold)
	// DefaultFieldQuoteColor defines the color for the quotes '"' surrounding object field names (keys). Default is bold blue.
	DefaultFieldQuoteColor = color.New(color.FgBlue, color.Bold)
	// DefaultFieldColor defines the color for the text of object field names (keys). Default is bold blue.
	DefaultFieldColor = color.New(color.FgBlue, color.Bold)
	// DefaultStringQuoteColor defines the color for the quotes '"' surrounding string values. Default is green.
	DefaultStringQuoteColor = color.New(color.FgGreen)
	// DefaultStringColor defines the color for the text of string values. Default is green.
	DefaultStringColor = color.New(color.FgGreen)
	// DefaultTrueColor defines the color for the boolean value 'true'. Default is no color.
	DefaultTrueColor = color.New()
	// DefaultFalseColor defines the color for the boolean value 'false'. Default is no color.
	DefaultFalseColor = color.New()
	// DefaultNumberColor defines the color for number values (integers, floats). Default is no color.
	DefaultNumberColor = color.New()
	// DefaultNullColor defines the color for the 'null' value. Default is bold black (often appears gray).
	DefaultNullColor = color.New(color.FgBlack, color.Bold)

	// DefaultPrefix is the string prepended to each indented line when indentation is enabled. Default is empty.
	DefaultPrefix = ""
	// DefaultIndent is the string used for each level of indentation when indentation is enabled. Default is two spaces.
	DefaultIndent = "  "
)

// Formatter holds the configuration for colorizing and indenting JSON output.
// A zero value Formatter{} will use all the default colors and indentation settings.
type Formatter struct {
	// Color configuration for different JSON elements. If a field is nil, the
	// corresponding Default*Color defined above will be used.
	SpaceColor       SprintfFuncer
	CommaColor       SprintfFuncer
	ColonColor       SprintfFuncer
	ObjectColor      SprintfFuncer
	ArrayColor       SprintfFuncer
	FieldQuoteColor  SprintfFuncer
	FieldColor       SprintfFuncer
	StringQuoteColor SprintfFuncer
	StringColor      SprintfFuncer
	TrueColor        SprintfFuncer
	FalseColor       SprintfFuncer
	NumberColor      SprintfFuncer
	NullColor        SprintfFuncer

	// Prefix is a string added before the indentation on each new line.
	// Only used if Indent is also non-empty.
	Prefix string
	// Indent is the string used for each level of indentation (e.g., "  " or "\t").
	// If empty, output is compact (no indentation or unnecessary whitespace).
	Indent string

	// EscapeHTML specifies whether problematic HTML characters (<, >, &)
	// should be escaped inside JSON quoted strings.
	// Note: This setting is primarily respected by the Encoder's Encode method.
	// The package-level Marshal* functions always enable HTML escaping, overriding this field.
	EscapeHTML bool
}

// NewFormatter creates a new Formatter instance initialized with default values
// (which means all color fields are nil, causing fallback to Default* colors).
func NewFormatter() *Formatter {
	return &Formatter{}
}

// clone creates a shallow copy of the Formatter. This is used internally
// to prevent modification of user-provided formatters when settings like
// EscapeHTML are forcefully overridden (e.g., in NewEncoderWithFormatter).
func (f *Formatter) clone() *Formatter {
	// Create a new Formatter and copy the field values from the original.
	g := *f
	return &g
}

// setIndent updates the Prefix and Indent fields of the Formatter.
// Used internally by Encoder.SetIndent.
func (f *Formatter) setIndent(prefix, indent string) {
	f.Prefix = prefix
	f.Indent = indent
}

// setEscapeHTML updates the EscapeHTML field of the Formatter.
// Used internally by Encoder.SetEscapeHTML.
func (f *Formatter) setEscapeHTML(on bool) {
	f.EscapeHTML = on
}

// Format takes existing, valid JSON data in `src` and writes a colorized
// version to `dst` according to the Formatter's settings.
// It does not add a trailing newline.
func (f *Formatter) Format(dst io.Writer, src []byte) error {
	// Create a state machine for formatting and execute it.
	// `false` means do not add a trailing newline.
	return newFormatterState(f, dst).format(dst, src, false)
}

// format is the internal method used by both Formatter.Format and Encoder.encode.
// It creates and runs the formatting state machine.
func (f *Formatter) format(dst io.Writer, src []byte, terminateWithNewline bool) error {
	// Create a state object initialized with this formatter's settings and the destination writer.
	formatterState := newFormatterState(f, dst)
	// Process the source JSON bytes and write the formatted output.
	return formatterState.format(dst, src, terminateWithNewline)
}

// Helper methods to get the appropriate SprintfFuncer, falling back to defaults if nil.
func (f *Formatter) spaceColor() SprintfFuncer {
	if f.SpaceColor != nil {
		return f.SpaceColor
	}
	return DefaultSpaceColor
}
func (f *Formatter) commaColor() SprintfFuncer {
	if f.CommaColor != nil {
		return f.CommaColor
	}
	return DefaultCommaColor
}
func (f *Formatter) colonColor() SprintfFuncer {
	if f.ColonColor != nil {
		return f.ColonColor
	}
	return DefaultColonColor
}
func (f *Formatter) objectColor() SprintfFuncer {
	if f.ObjectColor != nil {
		return f.ObjectColor
	}
	return DefaultObjectColor
}
func (f *Formatter) arrayColor() SprintfFuncer {
	if f.ArrayColor != nil {
		return f.ArrayColor
	}
	return DefaultArrayColor
}
func (f *Formatter) fieldQuoteColor() SprintfFuncer {
	if f.FieldQuoteColor != nil {
		return f.FieldQuoteColor
	}
	return DefaultFieldQuoteColor
}
func (f *Formatter) fieldColor() SprintfFuncer {
	if f.FieldColor != nil {
		return f.FieldColor
	}
	return DefaultFieldColor
}
func (f *Formatter) stringQuoteColor() SprintfFuncer {
	if f.StringQuoteColor != nil {
		return f.StringQuoteColor
	}
	return DefaultStringQuoteColor
}
func (f *Formatter) stringColor() SprintfFuncer {
	if f.StringColor != nil {
		return f.StringColor
	}
	return DefaultStringColor
}
func (f *Formatter) trueColor() SprintfFuncer {
	if f.TrueColor != nil {
		return f.TrueColor
	}
	return DefaultTrueColor
}
func (f *Formatter) falseColor() SprintfFuncer {
	if f.FalseColor != nil {
		return f.FalseColor
	}
	return DefaultFalseColor
}
func (f *Formatter) numberColor() SprintfFuncer {
	if f.NumberColor != nil {
		return f.NumberColor
	}
	return DefaultNumberColor
}
func (f *Formatter) nullColor() SprintfFuncer {
	if f.NullColor != nil {
		return f.NullColor
	}
	return DefaultNullColor
}

// formatterState holds the transient state during the process of formatting
// (parsing and colorizing) a JSON byte slice.
type formatterState struct {
	compact bool     // True if indentation is disabled (Prefix and Indent are empty).
	indent  string   // Cached indentation string (repeated f.Indent) to avoid recomputation.
	frames  []*frame // Stack tracking nesting level and context (object/array, key/value).

	// Pre-bound printing functions that include the colorization logic
	// based on the Formatter settings provided to newFormatterState.
	printSpace  func(s string, force bool) // Prints whitespace (handles compact mode). `force` ignores compact mode (used for final newline).
	printComma  func()                     // Prints a colorized comma.
	printColon  func()                     // Prints a colorized colon.
	printObject func(json.Delim)           // Prints a colorized object delimiter ({ or }).
	printArray  func(json.Delim)           // Prints a colorized array delimiter ([ or ]).
	printField  func(k string) error       // Prints a colorized object field name (key), including quotes. Handles string escaping.
	printString func(s string) error       // Prints a colorized string value, including quotes. Handles string escaping.
	printBool   func(b bool)               // Prints a colorized boolean value.
	printNumber func(n json.Number)        // Prints a colorized number value.
	printNull   func()                     // Prints a colorized null value.
	printIndent func()                     // Prints the current indentation (prefix + indent).
}

// newFormatterState creates and initializes a formatterState based on the
// provided Formatter configuration `f` and output writer `dst`.
// It captures the necessary color functions and sets up the initial state.
func newFormatterState(f *Formatter, dst io.Writer) *formatterState {
	// Retrieve the SprintfFunc for each color type, falling back to defaults.
	sprintfSpace := f.spaceColor().SprintfFunc()
	sprintfComma := f.commaColor().SprintfFunc()
	sprintfColon := f.colonColor().SprintfFunc()
	sprintfObject := f.objectColor().SprintfFunc()
	sprintfArray := f.arrayColor().SprintfFunc()
	sprintfFieldQuote := f.fieldQuoteColor().SprintfFunc()
	sprintfField := f.fieldColor().SprintfFunc()
	sprintfStringQuote := f.stringQuoteColor().SprintfFunc()
	sprintfString := f.stringColor().SprintfFunc()
	sprintfTrue := f.trueColor().SprintfFunc()
	sprintfFalse := f.falseColor().SprintfFunc()
	sprintfNumber := f.numberColor().SprintfFunc()
	sprintfNull := f.nullColor().SprintfFunc()

	// Helper function to properly encode a Go string into a JSON string payload
	// (handling escapes like \", \n, \t, etc.) and potentially HTML escapes (<, >, &)
	// based on the formatter's EscapeHTML setting.
	// It uses a temporary json.Encoder to achieve this.
	encodeString := func(s string) (string, error) {
		buf := bytes.NewBuffer(make([]byte, 0, len(s)+3)) // Preallocate buffer slightly larger than string
		enc := json.NewEncoder(buf)

		// Check if the standard library's json.Encoder supports SetEscapeHTML (Go 1.7+).
		// Use an interface assertion for compatibility with older Go versions.
		type setEscapeHTMLer interface {
			SetEscapeHTML(bool)
		}
		if se, ok := interface{}(enc).(setEscapeHTMLer); ok {
			// Apply the formatter's HTML escape setting.
			se.SetEscapeHTML(f.EscapeHTML)
		} else if f.EscapeHTML {
			// If SetEscapeHTML is not available but HTML escaping is requested,
			// we cannot fulfill the request using the standard encoder alone.
			// Note: The original code implicitly relied on the standard Marshal which *does*
			// escape HTML by default, so this case might indicate a slight behavioral
			// difference if used with Go < 1.7 AND f.EscapeHTML=true AND Marshal* functions.
			// However, the Marshal* functions *force* enc.SetEscapeHTML(true) later,
			// mitigating this for those entry points. Direct Formatter.Format calls
			// with Go < 1.7 and EscapeHTML=true might not escape HTML here.
			// For safety, we won't try to manually escape here, preserving original behavior.
		}

		// Use the encoder to encode the *Go string* `s` into a *JSON string literal*.
		// Note: We pass a pointer `&s` because json.Encoder.Encode expects an interface{},
		// and encoding the string directly might lead to different behavior (e.g., base64).
		err := enc.Encode(&s)
		if err != nil {
			return "", fmt.Errorf("internal error encoding string segment: %w", err)
		}

		// enc.Encode adds quotes and a newline. We need only the content *between* the quotes.
		sbuf := buf.Bytes()
		// Basic sanity check: must be at least `"..."\n` or `""\n`
		if len(sbuf) < 3 {
			return "", fmt.Errorf("internal error encoding string segment: result too short")
		}
		// Strip leading quote and trailing quote + newline.
		return string(sbuf[1 : len(sbuf)-2]), nil
	}

	// Initialize the formatter state.
	fs := &formatterState{
		// Indentation is disabled if both Prefix and Indent are empty.
		compact: len(f.Prefix) == 0 && len(f.Indent) == 0,
		indent:  "", // Indent cache starts empty.
		// Start with a base frame representing the top level. Indent level 0.
		frames: []*frame{{indent: 0}},

		// Define the print functions, capturing the sprintf functions and the writer.
		printComma: func() {
			fmt.Fprint(dst, sprintfComma(","))
		},
		printColon: func() {
			fmt.Fprint(dst, sprintfColon(":"))
		},
		printObject: func(t json.Delim) { // t is '{' or '}'
			fmt.Fprint(dst, sprintfObject(t.String()))
		},
		printArray: func(t json.Delim) { // t is '[' or ']'
			fmt.Fprint(dst, sprintfArray(t.String()))
		},
		printField: func(k string) error {
			// Encode the raw key string to handle escapes correctly.
			escapedKey, err := encodeString(k)
			if err != nil {
				return err
			}
			// Print quote, key text, quote using field colors.
			fmt.Fprint(dst, sprintfFieldQuote(`"`))
			fmt.Fprint(dst, sprintfField("%s", escapedKey))
			fmt.Fprint(dst, sprintfFieldQuote(`"`))
			return nil
		},
		printString: func(s string) error {
			// Encode the raw value string to handle escapes correctly.
			escapedValue, err := encodeString(s)
			if err != nil {
				return err
			}
			// Print quote, string text, quote using string value colors.
			fmt.Fprint(dst, sprintfStringQuote(`"`))
			fmt.Fprint(dst, sprintfString("%s", escapedValue))
			fmt.Fprint(dst, sprintfStringQuote(`"`))
			return nil
		},
		printBool: func(b bool) {
			if b {
				fmt.Fprint(dst, sprintfTrue("%v", b)) // Use %v for standard "true"
			} else {
				fmt.Fprint(dst, sprintfFalse("%v", b)) // Use %v for standard "false"
			}
		},
		printNumber: func(n json.Number) {
			fmt.Fprint(dst, sprintfNumber("%v", n)) // Use %v for standard number format
		},
		printNull: func() {
			fmt.Fprint(dst, sprintfNull("null"))
		},
	}

	// printSpace needs access to the `fs.compact` field, so define it after fs init.
	fs.printSpace = func(s string, force bool) {
		// Only print space if not in compact mode, or if forced (e.g., final newline).
		if fs.compact && !force {
			return
		}
		fmt.Fprint(dst, sprintfSpace(s))
	}

	// printIndent needs access to formatter `f` and the state `fs`, define it last.
	fs.printIndent = func() {
		// Don't indent if in compact mode.
		if fs.compact {
			return
		}
		// Print the prefix string, if any.
		if len(f.Prefix) > 0 {
			// Note: Prefix itself is not colorized by `sprintfSpace`.
			fmt.Fprint(dst, f.Prefix)
		}
		// Get the current indentation level from the frame stack.
		currentIndentLevel := fs.frame().indent
		if currentIndentLevel > 0 {
			// Calculate required length of the indentation string (e.g., level 2 * "  " = 4 chars).
			requiredIndentLen := len(f.Indent) * currentIndentLevel
			// Cache the repeated indent string if it's not long enough.
			// This avoids repeated string concatenation/building.
			if len(fs.indent) < requiredIndentLen {
				fs.indent = strings.Repeat(f.Indent, currentIndentLevel)
			}
			// Print the correctly sized slice of the cached indent string, applying space color.
			fmt.Fprint(dst, sprintfSpace(fs.indent[:requiredIndentLen]))
		}
	}

	return fs
}

// frame returns the current (top-most) frame from the stack.
func (fs *formatterState) frame() *frame {
	return fs.frames[len(fs.frames)-1]
}

// enterFrame pushes a new frame onto the stack when an opening delimiter
// ('{' or '[') is encountered. It increments the indentation level.
// `empty` indicates if the new object/array is known to be empty (e.g., {} or []).
func (fs *formatterState) enterFrame(t json.Delim, empty bool) *frame {
	// New indentation level is one greater than the current frame's level.
	newIndentLevel := fs.frames[len(fs.frames)-1].indent + 1
	newFrame := &frame{
		object: t == json.Delim('{'), // Set true if '{'
		array:  t == json.Delim('['), // Set true if '['
		indent: newIndentLevel,
		empty:  empty, // Mark if known to be empty from the start
		// `field` defaults to false (expecting key in object, irrelevant in array).
	}
	fs.frames = append(fs.frames, newFrame)
	return newFrame
}

// leaveFrame pops the current frame from the stack when a closing delimiter
// ('}' or ']') is encountered. It returns the frame that becomes the current one.
func (fs *formatterState) leaveFrame() *frame {
	fs.frames = fs.frames[:len(fs.frames)-1]
	return fs.frame()
}

// formatToken processes a single JSON token (delimiter, string, number, bool, null)
// and calls the appropriate `print*` function to write the colorized output.
func (fs *formatterState) formatToken(t json.Token) error {
	switch value := t.(type) {
	case json.Delim:
		// Delimiters '{', '}', '[', ']'
		if value == json.Delim('{') || value == json.Delim('}') {
			fs.printObject(value)
		} else {
			fs.printArray(value)
		}
	case json.Number:
		// Numeric literal
		fs.printNumber(value)
	case string:
		// String literal - check context to see if it's a key or value
		if fs.frame().inObject() && !fs.frame().inField() {
			// Inside an object ({) and expecting a key (field=false)
			return fs.printField(value)
		}
		// Otherwise, it's a string value (in array or after colon in object)
		return fs.printString(value)
	case bool:
		// Boolean literal 'true' or 'false'
		fs.printBool(value)
	case nil:
		// Null literal 'null'
		fs.printNull()
	default:
		// Should not happen with standard JSON tokens
		return fmt.Errorf("jsoncolor: unknown token type %T encountered", t)
	}
	return nil
}

// format drives the core JSON parsing and colorized printing process.
// It reads the input `src` using a json.Decoder, token by token,
// and writes the formatted, colorized output to `dst`.
// It maintains state using the `formatterState` (fs) to manage indentation,
// context (object key vs value), and spacing (commas, newlines).
func (fs *formatterState) format(dst io.Writer, src []byte, terminateWithNewline bool) error {
	// Use a standard JSON decoder.
	dec := json.NewDecoder(bytes.NewReader(src))
	// UseNumber ensures numbers retain their original string representation.
	dec.UseNumber()

	// currentFrame represents the current nesting context (top-level, object, array, etc.).
	currentFrame := fs.frame()

	// Loop through each token from the JSON input.
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break // End of JSON input.
		}
		if err != nil {
			return fmt.Errorf("jsoncolor: error decoding input JSON: %w", err)
		}

		// Check if more tokens exist at the current nesting level. Important for comma placement.
		hasMoreTokens := dec.More()
		// Determine if a comma is needed *after* processing the current token.
		needsCommaAfter := currentFrame.inArrayOrObject() && hasMoreTokens

		// --- Process based on token type: Delimiter or Value/Key ---

		// Is the token a delimiter ({, }, [, ])?
		if delim, ok := token.(json.Delim); ok {
			// Is it an opening delimiter?
			if delim == json.Delim('{') || delim == json.Delim('[') {
				// --- Handle Opening Delimiter ({ or [) ---
				// Decide spacing/indentation *before* the delimiter.
				// The primary case for adding space here is removed because
				// the space after a colon is handled later. We only need to
				// handle indentation when the delimiter starts a new line.
				if !currentFrame.inObject() {
					// If NOT inside an object (e.g., top-level container or inside an array),
					// print standard indentation.
					fs.printIndent()
				}

				// Print the colorized opening delimiter.
				err = fs.formatToken(delim)
				// If the container isn't empty, add a newline after the opener.
				if hasMoreTokens {
					fs.printSpace("\n", false)
				}
				// Descend into the new container, updating the current frame context.
				// Mark if the new container is empty based on whether tokens follow immediately.
				currentFrame = fs.enterFrame(delim, !hasMoreTokens)

			} else {
				// --- Handle Closing Delimiter (} or ]) ---
				// Check if the container being closed was empty (e.g., {} or []).
				isClosingEmptyContainer := currentFrame.isEmpty()
				// Ascend back to the parent container context.
				currentFrame = fs.leaveFrame()

				// Add indentation *before* the closing delimiter, unless it was an empty container.
				if !isClosingEmptyContainer {
					fs.printIndent()
				}
				// Print the colorized closing delimiter.
				err = fs.formatToken(delim)
				// Add a comma *after* the closing delimiter if required by the parent context.
				if needsCommaAfter {
					fs.printComma()
				}
				// Add a newline *after* the closing delimiter if we are still nested within another container.
				if len(fs.frames) > 1 { // > 1 means not back at the top level.
					fs.printSpace("\n", false)
				}
			}
		} else { // Token is not a delimiter, so it's a value (string, number, bool, null) or an object key.
			// --- Handle Value or Object Key ---
			// Determine if indentation is needed *before* this token.
			shouldIndent := currentFrame.inArray()
			// Special handling for strings to distinguish keys from values.
			if _, isString := token.(string); isString {
				// Indent string values within objects, but not object keys.
				// Also indent strings in arrays (covered by initial `inArray` check).
				shouldIndent = !currentFrame.inObject() || currentFrame.inField()
			}

			if shouldIndent {
				fs.printIndent()
			}

			// Print the colorized token. `formatToken` internally distinguishes keys and values.
			err = fs.formatToken(token)

			// --- Post-Token Formatting (Colon or Comma/Newline) ---
			if currentFrame.inField() {
				// If `inField` is true *now*, it means `formatToken` just processed an object *key*.
				// Therefore, print the required colon after the key, followed by a space (respecting compact mode).
				fs.printColon()
				fs.printSpace(" ", false) // Add space *only* after colon: "key": value
			} else {
				// If `formatToken` processed an array element or an object *value*.
				// Add a comma if needed *after* the element/value.
				if needsCommaAfter {
					fs.printComma()
				}
				// Add a newline if still nested.
				if len(fs.frames) > 1 {
					fs.printSpace("\n", false)
				}
			}
		} // End handling Delimiter vs Value/Key

		// If we are inside an object, toggle the state between expecting a key (`field`=false)
		// and expecting a value (`field`=true). This runs *after* processing the token
		// and its potential colon/comma follower for the current iteration.
		if currentFrame.inObject() {
			currentFrame.toggleField()
		}

		// Check for errors from printing functions.
		if err != nil {
			return err
		}
	} // End token processing loop

	// Add a final newline if requested (e.g., by Encoder.Encode).
	if terminateWithNewline {
		fs.printSpace("\n", true) // Force newline even in compact mode.
	}

	return nil
}
