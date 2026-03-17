package gojq

import "strconv"

// ValueError is an interface for errors with a value for internal function.
// Return an error implementing this interface when you want to catch error
// values (not error messages) by try-catch, just like built-in error function.
// Refer to [WithFunction] to add a custom internal function.
type ValueError interface {
	error
	Value() any
}

type expectedObjectError struct {
	v any
}

func (err *expectedObjectError) Error() string {
	return "expected an object but got: " + typeErrorPreview(err.v)
}

type expectedArrayError struct {
	v any
}

func (err *expectedArrayError) Error() string {
	return "expected an array but got: " + typeErrorPreview(err.v)
}

type iteratorError struct {
	v any
}

func (err *iteratorError) Error() string {
	return "cannot iterate over: " + typeErrorPreview(err.v)
}

type arrayIndexNegativeError struct {
	v int
}

func (err *arrayIndexNegativeError) Error() string {
	return "array index should not be negative: " + Preview(err.v)
}

type arrayIndexTooLargeError struct {
	v any
}

func (err *arrayIndexTooLargeError) Error() string {
	return "array index too large: " + Preview(err.v)
}

type repeatStringTooLargeError struct {
	s string
	n float64
}

func (err *repeatStringTooLargeError) Error() string {
	return "repeat string result too large: " + Preview(err.s) + " * " + Preview(err.n)
}

type objectKeyNotStringError struct {
	v any
}

func (err *objectKeyNotStringError) Error() string {
	return "expected a string for object key but got: " + typeErrorPreview(err.v)
}

type arrayIndexNotNumberError struct {
	v any
}

func (err *arrayIndexNotNumberError) Error() string {
	return "expected a number for indexing an array but got: " + typeErrorPreview(err.v)
}

type stringIndexNotNumberError struct {
	v any
}

func (err *stringIndexNotNumberError) Error() string {
	return "expected a number for indexing a string but got: " + typeErrorPreview(err.v)
}

type expectedStartEndError struct {
	v any
}

func (err *expectedStartEndError) Error() string {
	return `expected "start" and "end" for slicing but got: ` + typeErrorPreview(err.v)
}

type lengthMismatchError struct{}

func (*lengthMismatchError) Error() string {
	return "length mismatch"
}

type inputNotAllowedError struct{}

func (*inputNotAllowedError) Error() string {
	return "input(s)/0 is not allowed"
}

type funcNotFoundError struct {
	f *Func
}

func (err *funcNotFoundError) Error() string {
	return "function not defined: " + err.f.Name + "/" + strconv.Itoa(len(err.f.Args))
}

// QueryParseErrorOffset returns the byte offset of the function name token.
// Deprecated: use Position instead.
func (err *funcNotFoundError) QueryParseErrorOffset() int {
	return err.f.Offset
}

// Position implements [PositionError].
func (err *funcNotFoundError) Position() int {
	return err.f.Offset
}

type func0TypeError struct {
	name string
	v    any
}

func (err *func0TypeError) Error() string {
	return err.name + " cannot be applied to: " + typeErrorPreview(err.v)
}

type func1TypeError struct {
	name string
	v, w any
}

func (err *func1TypeError) Error() string {
	return err.name + "(" + Preview(err.w) + ") cannot be applied to: " + typeErrorPreview(err.v)
}

type func2TypeError struct {
	name    string
	v, w, x any
}

func (err *func2TypeError) Error() string {
	return err.name + "(" + Preview(err.w) + "; " + Preview(err.x) + ") cannot be applied to: " + typeErrorPreview(err.v)
}

type func0WrapError struct {
	name string
	v    any
	err  error
}

func (err *func0WrapError) Error() string {
	return err.name + " cannot be applied to " + Preview(err.v) + ": " + err.err.Error()
}

type func1WrapError struct {
	name string
	v, w any
	err  error
}

func (err *func1WrapError) Error() string {
	return err.name + "(" + Preview(err.w) + ") cannot be applied to " + Preview(err.v) + ": " + err.err.Error()
}

type func2WrapError struct {
	name    string
	v, w, x any
	err     error
}

func (err *func2WrapError) Error() string {
	return err.name + "(" + Preview(err.w) + "; " + Preview(err.x) + ") cannot be applied to " + Preview(err.v) + ": " + err.err.Error()
}

type ExitCodeError struct {
	Message any
	Code    int
}

func (err *ExitCodeError) Error() string {
	// Check if the message is an error
	if s, ok := err.Message.(error); ok {
		return "error: " + s.Error()
	}

	// Check if the message is a string
	if s, ok := err.Message.(string); ok {
		return "error: " + s
	}

	// If not, tru to marshal the message
	return "error: " + jsonMarshal(err.Message)
}

func (err *ExitCodeError) Value() any {
	return err.Message
}

func (err *ExitCodeError) Unwrap() error {
	// Check if the message is an error and return it
	if err, ok := err.Message.(error); ok {
		return err
	}

	return nil
}

func (err *ExitCodeError) ExitCode() int {
	return err.Code
}

// HaltError is an error emitted by halt and halt_error functions.
// It implements [ValueError], and if the value is nil, discard the error
// and stop the iteration. Consider a query like "1, halt, 2";
// the first value is 1, and the second value is a HaltError with nil value.
// You might think the iterator should not emit an error this case, but it
// should so that we can recognize the halt error to stop the outer loop
// of iterating input values; echo 1 2 3 | gojq "., halt".
type HaltError ExitCodeError

func (err *HaltError) Error() string {
	return "halt " + (*ExitCodeError)(err).Error()
}

// Value returns the value of the error. This implements [ValueError],
// but halt error is not catchable by try-catch.
func (err *HaltError) Value() any {
	return (*ExitCodeError)(err).Value()
}

// ExitCode returns the exit code of the error.
func (err *HaltError) ExitCode() int {
	return (*ExitCodeError)(err).ExitCode()
}

type flattenDepthError struct {
	v float64
}

func (err *flattenDepthError) Error() string {
	return "flatten depth should not be negative: " + Preview(err.v)
}

type joinTypeError struct {
	v any
}

func (err *joinTypeError) Error() string {
	return "join cannot be applied to an array including: " + typeErrorPreview(err.v)
}

type timeArrayError struct{}

func (*timeArrayError) Error() string {
	return "expected an array of 8 numbers"
}

type unaryTypeError struct {
	name string
	v    any
}

func (err *unaryTypeError) Error() string {
	return "cannot " + err.name + ": " + typeErrorPreview(err.v)
}

type binopTypeError struct {
	name string
	l, r any
}

func (err *binopTypeError) Error() string {
	return "cannot " + err.name + ": " + typeErrorPreview(err.l) + " and " + typeErrorPreview(err.r)
}

type zeroDivisionError struct {
	l, r any
}

func (err *zeroDivisionError) Error() string {
	return "cannot divide " + typeErrorPreview(err.l) + " by: " + typeErrorPreview(err.r)
}

type zeroModuloError struct {
	l, r any
}

func (err *zeroModuloError) Error() string {
	return "cannot modulo " + typeErrorPreview(err.l) + " by: " + typeErrorPreview(err.r)
}

type formatNotFoundError struct {
	n string
}

func (err *formatNotFoundError) Error() string {
	return "format not defined: " + err.n
}

type formatRowError struct {
	typ string
	v   any
}

func (err *formatRowError) Error() string {
	return "@" + err.typ + " cannot format an array including: " + typeErrorPreview(err.v)
}

type tooManyVariableValuesError struct{}

func (*tooManyVariableValuesError) Error() string {
	return "too many variable values provided"
}

type expectedVariableError struct {
	n string
}

func (err *expectedVariableError) Error() string {
	return "variable defined but not bound: " + err.n
}

type variableNotFoundError struct {
	n      string
	offset int
}

func (err *variableNotFoundError) Error() string {
	return "variable not defined: " + err.n
}

// QueryParseErrorOffset returns the byte offset of the variable token.
// Deprecated: use Position instead.
func (err *variableNotFoundError) QueryParseErrorOffset() int {
	return err.offset
}

// Position implements [PositionError].
func (err *variableNotFoundError) Position() int {
	return err.offset
}

type variableNameError struct {
	n string
}

func (err *variableNameError) Error() string {
	return "invalid variable name: " + err.n
}

type breakError struct {
	n string
	v any
}

func (err *breakError) Error() string {
	return "label not defined: " + err.n
}

func (*breakError) ExitCode() int {
	return 3
}

type tryEndError struct {
	err error
}

func (err *tryEndError) Error() string {
	return err.err.Error()
}

type invalidPathError struct {
	v any
}

func (err *invalidPathError) Error() string {
	return "invalid path against: " + typeErrorPreview(err.v)
}

type invalidPathIterError struct {
	v any
}

func (err *invalidPathIterError) Error() string {
	return "invalid path on iterating against: " + typeErrorPreview(err.v)
}

// stackFrame represents one frame in a runtime error's call chain.
type stackFrame struct {
	offset int
	fname  string // empty for main query
	source string // empty for main query
}

// runtimeError wraps a runtime error with a source byte offset so the CLI can
// show file/line information when available. When the error originates inside
// an imported module, fname and source identify the module file.
type runtimeError struct {
	err    error
	offset int
	fname  string // file path of the source (empty for main query)
	source string // source text of the file (empty for main query)
	frames []stackFrame // call chain from nearest caller to outermost
}

func (e *runtimeError) Error() string {
	return e.err.Error()
}

func (e *runtimeError) Unwrap() error {
	return e.err
}

// QueryParseErrorOffset returns the byte offset in the source where the
// runtime error originated. Deprecated: use Position instead.
func (e *runtimeError) QueryParseErrorOffset() int {
	return e.offset
}

// Position implements [PositionError], returning the byte offset where the
// runtime error occurred. Use [LineColumn] to convert it to line/column.
func (e *runtimeError) Position() int {
	return e.offset
}

// SourceFile implements [SourcePositionError]. It returns the file path of the
// module where the error occurred, or an empty string for errors in the main query.
func (e *runtimeError) SourceFile() string {
	return e.fname
}

// SourceText implements [SourcePositionError]. It returns the full source text
// of the module where the error occurred, for use with [LineColumn].
func (e *runtimeError) SourceText() string {
	return e.source
}

// CallStack implements [StackTraceError]. It returns the call frames from the
// nearest caller to the outermost, or nil if no call chain is available.
func (e *runtimeError) CallStack() []StackFrame {
	if len(e.frames) == 0 {
		return nil
	}
	result := make([]StackFrame, len(e.frames))
	for i, f := range e.frames {
		result[i] = StackFrame{
			Offset:     f.offset,
			SourceFile: f.fname,
			SourceText: f.source,
		}
	}
	return result
}

type queryParseError struct {
	fname, contents string
	err             error
}

func (err *queryParseError) QueryParseError() (string, string, error) {
	return err.fname, err.contents, err.err
}

func (err *queryParseError) Error() string {
	return "invalid query: " + err.fname + ": " + err.err.Error()
}

type jsonParseError struct {
	fname, contents string
	err             error
}

func (err *jsonParseError) JSONParseError() (string, string, error) {
	return err.fname, err.contents, err.err
}

func (err *jsonParseError) Error() string {
	return "invalid json: " + err.fname + ": " + err.err.Error()
}

func typeErrorPreview(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case Iter:
		return "gojq.Iter"
	default:
		return TypeOf(v) + " (" + Preview(v) + ")"
	}
}
