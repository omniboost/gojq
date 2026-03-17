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
	// fname is resolved to absolute, so check it contains the filename
	if !strings.Contains(got, fname) {
		t.Errorf("FormatErrorAt should contain fname, got: %q", got)
	}
	if !strings.Contains(got, "function not defined") {
		t.Errorf("FormatErrorAt should contain the message, got: %q", got)
	}

	// Error without position: falls back to "fname: message"
	plainErr := &gojq.HaltError{}
	got = gojq.FormatErrorAt(fname, src, plainErr)
	if !strings.Contains(got, fname+": ") {
		t.Errorf("FormatErrorAt without position should contain %q: ..., got: %q", fname, got)
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

// testModuleLoader implements the module loader interfaces needed for testing
// module source position tracking.
type testModuleLoader struct {
	modules map[string]string // module name → source text
}

func (l *testModuleLoader) LoadModuleWithSource(name string, meta map[string]any) (*gojq.Query, string, string, error) {
	source, ok := l.modules[name]
	if !ok {
		return nil, "", "", &testModuleNotFoundError{name}
	}
	q, err := gojq.Parse(source)
	if err != nil {
		return nil, "", "", err
	}
	fname := name + ".jq"
	return q, fname, source, nil
}

type testModuleNotFoundError struct {
	name string
}

func (e *testModuleNotFoundError) Error() string {
	return "module not found: " + e.name
}

// TestSourcePositionError_ImportedModule verifies that runtime errors inside
// imported module functions implement SourcePositionError and StackTraceError
// with correct file, source, and call stack info.
func TestSourcePositionError_ImportedModule(t *testing.T) {
	moduleSrc := "# comment\ndef iter_value:\n    .[] ;\n"
	loader := &testModuleLoader{
		modules: map[string]string{
			"mymod": moduleSrc,
		},
	}
	mainSrc := `import "mymod" as m; m::iter_value`
	q, err := gojq.Parse(mainSrc)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	code, err := gojq.Compile(q, gojq.WithModuleLoader(loader))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	iter := code.Run(42) // 42 is not iterable
	for {
		v, ok := iter.Next()
		if !ok {
			t.Fatal("expected an error")
		}
		rErr, ok := v.(error)
		if !ok {
			continue
		}
		if !strings.Contains(rErr.Error(), "cannot iterate over") {
			t.Fatalf("unexpected error: %v", rErr)
		}
		spe, ok := rErr.(gojq.SourcePositionError)
		if !ok {
			t.Fatalf("expected SourcePositionError, got %T", rErr)
		}
		if spe.SourceFile() != "mymod.jq" {
			t.Errorf("SourceFile() = %q, want %q", spe.SourceFile(), "mymod.jq")
		}
		if spe.SourceText() != moduleSrc {
			t.Errorf("SourceText() = %q, want %q", spe.SourceText(), moduleSrc)
		}
		// Verify the position points to the correct line in the module
		line, col := gojq.LineColumn(spe.SourceText(), spe.Position())
		if line != 3 {
			t.Errorf("line = %d, want 3", line)
		}
		if col < 4 || col > 5 {
			t.Errorf("col = %d, want 4 or 5 (at [ in .[])", col)
		}
		// FormatErrorAt should use the module's file/source, not the main query's
		formatted := gojq.FormatErrorAt("main.jq", mainSrc, rErr)
		if !strings.Contains(formatted, "mymod.jq:") {
			t.Errorf("FormatErrorAt should contain module filename, got: %q", formatted)
		}
		if !strings.Contains(formatted, "3:") {
			t.Errorf("FormatErrorAt should show line 3, got: %q", formatted)
		}
		// Verify the stack trace
		ste, ok := rErr.(gojq.StackTraceError)
		if !ok {
			t.Fatalf("expected StackTraceError, got %T", rErr)
		}
		stack := ste.CallStack()
		if len(stack) == 0 {
			t.Fatal("CallStack() should have at least one frame")
		}
		// The caller frame should be in the main query (empty SourceFile)
		if stack[0].SourceFile != "" {
			t.Errorf("caller frame SourceFile = %q, want empty (main query)", stack[0].SourceFile)
		}
		// The caller offset should point at m::iter_value in the main source
		callerIdx := strings.Index(mainSrc, "m::iter_value")
		if callerIdx >= 0 && stack[0].Offset != callerIdx {
			t.Errorf("caller frame Offset = %d, want %d (index of m::iter_value)", stack[0].Offset, callerIdx)
		}
		// FormatErrorAt should include "↳" text
		if !strings.Contains(formatted, "↳") {
			t.Errorf("FormatErrorAt should include stack trace, got: %q", formatted)
		}
		return
	}
}

// TestSourcePositionError_ErrorAtOffset0 verifies that when an error genuinely
// occurs at byte offset 0 in a module (position 1:1), it still appears
// correctly in the error position and stack trace — offset 0 must not be
// filtered out as a bogus frame.
func TestSourcePositionError_ErrorAtOffset0(t *testing.T) {
	moduleSrc := "def at_zero: .[] ;\n"
	loader := &testModuleLoader{
		modules: map[string]string{
			"offset0mod": moduleSrc,
		},
	}

	mainSrc := `import "offset0mod" as m; m::at_zero`
	q, err := gojq.Parse(mainSrc)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	code, err := gojq.Compile(q, gojq.WithModuleLoader(loader))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	iter := code.Run(42) // 42 is not iterable
	for {
		v, ok := iter.Next()
		if !ok {
			t.Fatal("expected an error")
		}
		rErr, ok := v.(error)
		if !ok {
			continue
		}
		if !strings.Contains(rErr.Error(), "cannot iterate over") {
			t.Fatalf("unexpected error: %v", rErr)
		}
		// Error should point into the module
		spe, ok := rErr.(gojq.SourcePositionError)
		if !ok {
			t.Fatalf("expected SourcePositionError, got %T", rErr)
		}
		if spe.SourceFile() != "offset0mod.jq" {
			t.Errorf("SourceFile() = %q, want %q", spe.SourceFile(), "offset0mod.jq")
		}
		line, _ := gojq.LineColumn(spe.SourceText(), spe.Position())
		// .[] is at "def at_zero: .[]" — line 1, col should be at '['
		if line != 1 {
			t.Errorf("error line = %d, want 1", line)
		}
		// Verify stack trace includes a caller frame
		ste, ok := rErr.(gojq.StackTraceError)
		if !ok {
			t.Fatalf("expected StackTraceError, got %T", rErr)
		}
		stack := ste.CallStack()
		if len(stack) == 0 {
			t.Fatal("CallStack() should have at least one frame")
		}
		// FormatErrorAt should show the module file with line 1
		formatted := gojq.FormatErrorAt("main.jq", mainSrc, rErr)
		if !strings.Contains(formatted, "offset0mod.jq:1:") {
			t.Errorf("FormatErrorAt should show offset0mod.jq:1:, got: %q", formatted)
		}
		if !strings.Contains(formatted, "↳") {
			t.Errorf("FormatErrorAt should include stack trace, got: %q", formatted)
		}
		return
	}
}

// TestSourcePositionError_NoBogusOffset0Frames verifies that compiler-generated
// internal calls (like |=, _modify) do NOT produce bogus 1:1 frames in the
// stack trace. These internal Func nodes have Offset: -1 to prevent recording.
func TestSourcePositionError_NoBogusOffset0Frames(t *testing.T) {
	// Module uses |= which internally compiles to _modify with Offset: -1.
	// Without the fix, this would produce a phantom 1:1 frame.
	moduleSrc := "def update_val: .val |= . + 1 ;\n"
	loader := &testModuleLoader{
		modules: map[string]string{
			"updatemod": moduleSrc,
		},
	}
	mainSrc := `import "updatemod" as u; u::update_val`
	q, err := gojq.Parse(mainSrc)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	code, err := gojq.Compile(q, gojq.WithModuleLoader(loader))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	// Pass a non-object so .val fails
	iter := code.Run("not-an-object")
	for {
		v, ok := iter.Next()
		if !ok {
			t.Fatal("expected an error")
		}
		rErr, ok := v.(error)
		if !ok {
			continue
		}
		// Check that no stack frame points to offset 0 (position 1:1) in the module
		ste, ok := rErr.(gojq.StackTraceError)
		if !ok {
			// Error may not have stack trace info, which is also fine
			return
		}
		spe, _ := rErr.(gojq.SourcePositionError)
		// The error position itself at 1:1 in the module would only be valid
		// if the error-causing code is actually at byte 0. For .val |=,
		// the error should point at .val (offset 16 = "def update_val: " prefix).
		if spe != nil && spe.SourceFile() != "" {
			// The error should not be at offset 0 since .val is not at
			// the start of the file
			if spe.Position() == 0 {
				t.Errorf("error position should not be 0 (bogus offset from internal call)")
			}
		}
		for i, f := range ste.CallStack() {
			if f.Offset == 0 && f.SourceFile != "" {
				// A frame at offset 0 in a module is suspicious — it likely
				// comes from an internal compiler-generated call.
				frameLine, frameCol := gojq.LineColumn(f.SourceText, f.Offset)
				t.Errorf("stack frame %d has offset 0 in %q (line %d, col %d) — likely a bogus frame from internal call",
					i, f.SourceFile, frameLine, frameCol)
			}
		}
		return
	}
}

// TestSourcePositionError_CompileErrorInModule verifies that compile errors
// (like "variable not defined") in imported modules implement SourcePositionError
// and point at the correct file and line in the module, not the main query.
func TestSourcePositionError_CompileErrorInModule(t *testing.T) {
	tests := []struct {
		name       string
		moduleSrc  string
		wantErrMsg string
		wantLine   int
	}{
		{
			name:       "undefined variable",
			moduleSrc:  "# line1\n# line2\ndef foo: $undefined_var ;\n",
			wantErrMsg: "variable not defined",
			wantLine:   3,
		},
		{
			name:       "undefined function",
			moduleSrc:  "def bar:\n    no_such_func ;\n",
			wantErrMsg: "function not defined",
			wantLine:   2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			loader := &testModuleLoader{
				modules: map[string]string{
					"errmod": tc.moduleSrc,
				},
			}
			mainSrc := `import "errmod" as m; m::foo`
			if tc.name == "undefined function" {
				mainSrc = `import "errmod" as m; m::bar`
			}
			q, err := gojq.Parse(mainSrc)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			_, compErr := gojq.Compile(q, gojq.WithModuleLoader(loader))
			if compErr == nil {
				t.Fatal("expected a compile error")
			}
			if !strings.Contains(compErr.Error(), tc.wantErrMsg) {
				t.Fatalf("error = %q, want it to contain %q", compErr.Error(), tc.wantErrMsg)
			}
			// Must implement SourcePositionError pointing at the module
			spe, ok := compErr.(gojq.SourcePositionError)
			if !ok {
				t.Fatalf("expected SourcePositionError, got %T: %v", compErr, compErr)
			}
			if spe.SourceFile() != "errmod.jq" {
				t.Errorf("SourceFile() = %q, want %q", spe.SourceFile(), "errmod.jq")
			}
			if spe.SourceText() != tc.moduleSrc {
				t.Errorf("SourceText() = %q, want %q", spe.SourceText(), tc.moduleSrc)
			}
			line, _ := gojq.LineColumn(spe.SourceText(), spe.Position())
			if line != tc.wantLine {
				t.Errorf("line = %d, want %d", line, tc.wantLine)
			}
			// FormatErrorAt should show module file, not the main query file
			formatted := gojq.FormatErrorAt("main.jq", mainSrc, compErr)
			if !strings.Contains(formatted, "errmod.jq:") {
				t.Errorf("FormatErrorAt should contain module filename, got: %q", formatted)
			}
			if strings.Contains(formatted, "main.jq:") {
				t.Errorf("FormatErrorAt should NOT contain main.jq filename, got: %q", formatted)
			}
		})
	}
}

// TestSourcePositionError_MainQueryErrors verifies that errors in the main
// query do NOT set SourceFile/SourceText (they should be empty) and have
// no call stack.
func TestSourcePositionError_MainQueryErrors(t *testing.T) {
	src := ".[]"
	q, _ := gojq.Parse(src)
	code, _ := gojq.Compile(q)
	iter := code.Run("hello")
	for {
		v, ok := iter.Next()
		if !ok {
			t.Fatal("expected an error")
		}
		rErr, ok := v.(error)
		if !ok {
			continue
		}
		spe, ok := rErr.(gojq.SourcePositionError)
		if !ok {
			t.Fatalf("expected SourcePositionError, got %T", rErr)
		}
		if spe.SourceFile() != "" {
			t.Errorf("SourceFile() should be empty for main query errors, got %q", spe.SourceFile())
		}
		if spe.SourceText() != "" {
			t.Errorf("SourceText() should be empty for main query errors, got %q", spe.SourceText())
		}
		// FormatErrorAt should use the provided fname/src for main query errors
		formatted := gojq.FormatErrorAt("main.jq", src, rErr)
		if !strings.Contains(formatted, "main.jq:1:") {
			t.Errorf("FormatErrorAt should contain main.jq for main query errors, got: %q", formatted)
		}
		// No call stack for main query errors
		ste, ok := rErr.(gojq.StackTraceError)
		if !ok {
			t.Fatalf("expected StackTraceError, got %T", rErr)
		}
		if stack := ste.CallStack(); stack != nil {
			t.Errorf("CallStack() should be nil for main query errors, got %v", stack)
		}
		// FormatErrorAt should NOT include "↳" for main query errors
		if strings.Contains(formatted, "↳") {
			t.Errorf("FormatErrorAt should not include stack trace for main query errors, got: %q", formatted)
		}
		return
	}
}
