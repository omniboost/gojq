package gojq_test

import (
	"strings"
	"testing"

	"github.com/omniboost/gojq"
)

// TestLineColumn verifies that LineColumn correctly converts byte offsets to
// 1-based line and 0-based column values, including multi-line strings and
// Unicode characters.
func TestLineColumn(t *testing.T) {
	tests := []struct {
		src    string
		offset int
		line   int
		col    int
	}{
		{"abc", 0, 1, 0},
		{"abc", 1, 1, 1},
		{"abc", 3, 1, 3},
		{"abc\ndef", 4, 2, 0},
		{"abc\ndef", 7, 2, 3},
		// Multi-line: offset at second line
		{"line1\nline2\nline3", 6, 2, 0},
		{"line1\nline2\nline3", 11, 2, 5},
		{"line1\nline2\nline3", 12, 3, 0},
		// Unicode: emoji is 4 bytes but the column is measured in Unicode
		// code points, so offset 2 (right before the emoji) gives column 2,
		// and offset 6 (right after the 4-byte emoji) gives column 3.
		{"ab\U0001F600cd", 2, 1, 2},
		{"ab\U0001F600cd", 6, 1, 3},
		// Clamping: negative offset → 0
		{"abc", -5, 1, 0},
		// Clamping: offset beyond end → end
		{"abc", 100, 1, 3},
		// Windows line endings: \r\n counts as one newline
		{"abc\r\ndef", 5, 2, 0},
		{"abc\r\ndef", 8, 2, 3},
		// Bare \r counts as one newline
		{"abc\rdef", 4, 2, 0},
		{"abc\rdef", 7, 2, 3},
		// Mixed: \r\n then \r
		{"a\r\nb\rc", 4, 2, 1},
		{"a\r\nb\rc", 5, 3, 0},
	}
	for _, tc := range tests {
		line, col := gojq.LineColumn(tc.src, tc.offset)
		if line != tc.line || col != tc.col {
			t.Errorf("LineColumn(%q, %d) = (%d, %d), want (%d, %d)",
				tc.src, tc.offset, line, col, tc.line, tc.col)
		}
	}
}

// TestFormatError verifies that FormatError prefixes line:col when the error
// implements PositionError, and falls back to err.Error() otherwise.
func TestFormatError(t *testing.T) {
	// Compile error: function not defined
	src := ".foo | undefined_func | .bar"
	q, _ := gojq.Parse(src)
	_, compErr := gojq.Compile(q)
	if compErr == nil {
		t.Fatal("expected a compile error")
	}
	got := gojq.FormatError(src, compErr)
	if !strings.HasPrefix(got, "1:") {
		t.Errorf("FormatError compile error should start with line:col, got: %q", got)
	}
	if !strings.Contains(got, "function not defined") {
		t.Errorf("FormatError compile error should contain the message, got: %q", got)
	}

	// Error without position: falls back to plain err.Error()
	haltErr := &gojq.HaltError{}
	got = gojq.FormatError(src, haltErr)
	if got != haltErr.Error() {
		t.Errorf("FormatError without position should return err.Error() = %q, got: %q", haltErr.Error(), got)
	}
}

// TestFormatErrorAt verifies that FormatErrorAt produces "fname:line:col: msg"
// and falls back to "fname: msg" when there is no position info.
func TestFormatErrorAt(t *testing.T) {
	src := ".foo | undefined_func | .bar"
	fname := "myfile.jq"
	q, _ := gojq.Parse(src)
	_, compErr := gojq.Compile(q)
	if compErr == nil {
		t.Fatal("expected a compile error")
	}

	got := gojq.FormatErrorAt(fname, src, compErr)
	if !strings.HasPrefix(got, fname+":") {
		t.Errorf("FormatErrorAt should start with fname:, got: %q", got)
	}
	if !strings.Contains(got, "function not defined") {
		t.Errorf("FormatErrorAt should contain the message, got: %q", got)
	}

	// Error without position: falls back to "fname: message"
	plainErr := &gojq.HaltError{}
	got = gojq.FormatErrorAt(fname, src, plainErr)
	if !strings.HasPrefix(got, fname+": ") {
		t.Errorf("FormatErrorAt without position should be %q: ..., got: %q", fname, got)
	}
}

// TestPositionError_CompileErrors verifies that compile errors implement
// PositionError and return correct byte offsets.
func TestPositionError_CompileErrors(t *testing.T) {
	tests := []struct {
		src        string
		wantPrefix string // substring that must appear before the error offset
	}{
		// function not defined: offset points at the function name
		{".foo | undefined_func | .bar", "undefined_func"},
		// variable not defined: offset points at the $var token
		{"$undeclared", "$undeclared"},
	}
	for _, tc := range tests {
		q, err := gojq.Parse(tc.src)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		_, compErr := gojq.Compile(q)
		if compErr == nil {
			t.Fatalf("expected compile error for %q", tc.src)
		}
		pe, ok := compErr.(gojq.PositionError)
		if !ok {
			t.Fatalf("compile error for %q does not implement PositionError: %T", tc.src, compErr)
		}
		pos := pe.Position()
		if pos < 0 || pos > len(tc.src) {
			t.Errorf("Position() = %d for %q: out of range", pos, tc.src)
		}
		idx := strings.Index(tc.src, tc.wantPrefix)
		if idx < 0 {
			t.Fatalf("wantPrefix %q not found in src %q", tc.wantPrefix, tc.src)
		}
		if pos != idx {
			t.Errorf("Position() = %d for %q, want %d (index of %q)", pos, tc.src, idx, tc.wantPrefix)
		}
	}
}

// TestPositionError_RuntimeErrors verifies that common runtime errors implement
// PositionError and point at the right location in the query source.
func TestPositionError_RuntimeErrors(t *testing.T) {
	tests := []struct {
		src        string
		input      any
		wantErrMsg string
		wantAtStr  string // substring of src that the offset should point at
	}{
		// Object key not a string: offset points at '{'
		{"{(null): 1}", map[string]any{}, "expected a string for object key", "{"},
		// Iterator on non-iterable string: offset points at '['
		{".[]", "hello", "cannot iterate over", "["},
		// Iterator on null: offset points at '[' (null is not iterable)
		{".[]", nil, "cannot iterate over", "["},
		// Iterator on null after a pipe: offset points at '[' in the second .[]
		{".foo | .[]", map[string]any{"foo": nil}, "cannot iterate over", "["},
		// Iterator on null via intermediate step: offset points at '[' at the right position
		{". | .[]", nil, "cannot iterate over", "["},
		// Index non-object: offset points at '.foo'
		{".foo", 128, "expected an object", ".foo"},
		// tonumber on non-numeric string: offset points at 'tonumber'
		{`"hello" | tonumber`, nil, "tonumber cannot be applied", "tonumber"},
		// floor on string: offset points at 'floor'
		{`"a" | floor`, nil, "floor cannot be applied", "floor"},
		// keys on non-object/array: offset points at 'keys'
		{"1 | keys", nil, "keys cannot be applied", "keys"},
		// arithmetic: cannot multiply: offset points at '*'
		{"null * -1", nil, "cannot multiply", "*"},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			q, err := gojq.Parse(tc.src)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			code, err := gojq.Compile(q)
			if err != nil {
				t.Fatalf("compile %q: %v", tc.src, err)
			}
			iter := code.Run(tc.input)
			for {
				v, ok := iter.Next()
				if !ok {
					t.Fatalf("no error emitted for %q", tc.src)
				}
				rErr, ok := v.(error)
				if !ok {
					continue
				}
				if !strings.Contains(rErr.Error(), tc.wantErrMsg) {
					t.Fatalf("got error %q, want it to contain %q", rErr.Error(), tc.wantErrMsg)
				}
				pe, ok := rErr.(gojq.PositionError)
				if !ok {
					t.Fatalf("runtime error %T does not implement PositionError", rErr)
				}
				pos := pe.Position()
				idx := strings.Index(tc.src, tc.wantAtStr)
				if idx < 0 {
					t.Fatalf("wantAtStr %q not found in src %q", tc.wantAtStr, tc.src)
				}
				if pos != idx {
					t.Errorf("Position() = %d for %q, want %d (index of %q)", pos, tc.src, idx, tc.wantAtStr)
				}
				return
			}
		})
	}
}

// TestPositionError_BuiltinJQFunctions verifies that errors originating inside
// builtin jq functions (those defined in builtin.jq and compiled lazily, such
// as map, group_by, sort_by, etc.) produce PositionErrors pointing at the
// function call site in the user's query — not at an offset inside the builtin's
// own source code.
//
// The builtinDepth mechanism suppresses offset recording inside lazily-compiled
// builtin jq function bodies, and wrapRuntimeError walks the scope stack to find
// the nearest user-code call-site offset instead.
func TestPositionError_BuiltinJQFunctions(t *testing.T) {
	// All of these should produce a PositionError pointing at the function name
	// in the user's query. Before the fix, group_by (and similar jq-defined
	// builtins) would produce either no PositionError or an out-of-range offset
	// from the builtin source.
	withPositionTests := []struct {
		src        string
		input      any
		wantErrMsg string
		wantAtStr  string
	}{
		// jq-defined builtins (compiled from builtin.jq)
		{"group_by(.)", "hello", "cannot iterate over", "group_by"},
		{"sort_by(.)", "hello", "cannot iterate over", "sort_by"},
		{"min_by(.)", "hello", "cannot iterate over", "min_by"},
		{"max_by(.)", "hello", "cannot iterate over", "max_by"},
		{"unique_by(.)", "hello", "cannot iterate over", "unique_by"},
		{"map(.)", "hello", "cannot iterate over", "map"},
		{"map(tojson)", "hello", "cannot iterate over", "map"},
		{"[map(.)]", "hello", "cannot iterate over", "map"},
		// errors from inside the arg to a jq builtin should point at the
		// argument, not at the builtin name
		{"map(tonumber)", []any{1, 2, "three"}, "tonumber cannot be applied", "tonumber"},
		// internal Go functions called directly by user code — unchanged behaviour
		{"from_entries", "hello", "from_entries cannot be applied", "from_entries"},
		{". | from_entries", "hello", "from_entries cannot be applied", "from_entries"},
		{"unique", "hello", "unique cannot be applied", "unique"},
		{"sort", "hello", "sort cannot be applied", "sort"},
	}
	for _, tc := range withPositionTests {
		t.Run("with_position/"+tc.src, func(t *testing.T) {
			q, err := gojq.Parse(tc.src)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			code, err := gojq.Compile(q)
			if err != nil {
				t.Fatalf("compile %q: %v", tc.src, err)
			}
			iter := code.Run(tc.input)
			for {
				v, ok := iter.Next()
				if !ok {
					t.Fatalf("no error emitted for %q", tc.src)
				}
				rErr, ok := v.(error)
				if !ok {
					continue
				}
				if !strings.Contains(rErr.Error(), tc.wantErrMsg) {
					t.Fatalf("got error %q, want it to contain %q", rErr.Error(), tc.wantErrMsg)
				}
				pe, ok := rErr.(gojq.PositionError)
				if !ok {
					t.Fatalf("runtime error %T does not implement PositionError for %q", rErr, tc.src)
				}
				pos := pe.Position()
				if pos < 0 || pos >= len(tc.src) {
					t.Errorf("Position() = %d is out of range [0, %d) for src %q",
						pos, len(tc.src), tc.src)
					return
				}
				idx := strings.Index(tc.src, tc.wantAtStr)
				if idx < 0 {
					t.Fatalf("wantAtStr %q not found in src %q", tc.wantAtStr, tc.src)
				}
				if pos != idx {
					t.Errorf("Position() = %d for %q, want %d (index of %q in src)",
						pos, tc.src, idx, tc.wantAtStr)
				}
				return
			}
		})
	}
}

// TestPositionError_TryCatch verifies that errors caught by try-catch do NOT
// get wrapped with position info — they must remain as plain values so that
// catch can process them.
func TestPositionError_TryCatch(t *testing.T) {
	tests := []struct {
		src   string
		input any
	}{
		{`try tonumber catch "caught: \(.)"`, "hello"},
		{`try error catch .`, "some-error"},
		{`try ("foo" | error) catch .`, nil},
		{`[.[] | tonumber?]`, []any{"1", "not-a-number", "3"}},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			q, err := gojq.Parse(tc.src)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			code, err := gojq.Compile(q)
			if err != nil {
				t.Fatalf("compile %q: %v", tc.src, err)
			}
			iter := code.Run(tc.input)
			for {
				v, ok := iter.Next()
				if !ok {
					break
				}
				if rErr, ok := v.(error); ok {
					t.Errorf("try-catch query %q emitted an error: %v", tc.src, rErr)
				}
			}
		})
	}
}

// TestPositionError_Halt verifies that halt/halt_error are NOT wrapped with
// position info and continue to work as HaltErrors.
func TestPositionError_Halt(t *testing.T) {
	q, _ := gojq.Parse("0, halt, 1")
	code, _ := gojq.Compile(q)
	iter := code.Run(nil)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			if _, ok := err.(*gojq.HaltError); !ok {
				t.Errorf("expected *HaltError, got %T: %v", err, err)
			}
			break
		}
	}
}
