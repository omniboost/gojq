package gojq

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// PositionError is implemented by errors that carry a source byte offset.
// Both compile-time errors (e.g. "function not defined") and runtime errors
// (e.g. "expected an object but got") implement this interface when position
// information is available.
//
// Use [FormatError] or [FormatErrorAt] for the simplest way to include
// position in error messages, or use [LineColumn] for full control:
//
//	query, _ := gojq.Parse(src)
//	code, err := gojq.Compile(query)
//	if err != nil {
//	    log.Fatal(gojq.FormatErrorAt("myfile.jq", src, err))
//	    // → "/home/user/myfile.jq:1:8: function not defined: undefined_func/0"
//	}
//
//	iter := code.Run(input)
//	for {
//	    v, ok := iter.Next()
//	    if !ok { break }
//	    if err, ok := v.(error); ok {
//	        log.Fatal(gojq.FormatErrorAt("myfile.jq", src, err))
//	        // → "/home/user/myfile.jq:1:1: expected a string for object key but got: null"
//	    }
//	}
type PositionError interface {
	error
	// Position returns the byte offset in the query source where the error
	// occurred. Pass this value together with the original query string to
	// [LineColumn] to obtain a human-readable line and column number.
	Position() int
}

// StackFrame represents one frame in a runtime error's call stack trace.
type StackFrame struct {
	// Offset is the byte offset in the source where the call occurred.
	Offset int
	// SourceFile is the file path, or empty for the main query.
	SourceFile string
	// SourceText is the full source text, or empty for the main query.
	SourceText string
}

// SourcePositionError extends [PositionError] with source file information.
// Runtime errors that occur inside imported modules implement this interface,
// allowing callers to identify which file the error originated in and to
// compute accurate line/column positions using the module's own source text.
//
//	iter := code.Run(input)
//	for {
//	    v, ok := iter.Next()
//	    if !ok { break }
//	    if err, ok := v.(error); ok {
//	        if spe, ok := err.(gojq.SourcePositionError); ok && spe.SourceFile() != "" {
//	            line, col := gojq.LineColumn(spe.SourceText(), spe.Position())
//	            log.Fatalf("%s:%d:%d: %s", spe.SourceFile(), line, col+1, err)
//	        } else {
//	            log.Fatal(gojq.FormatErrorAt("myfile.jq", src, err))
//	        }
//	    }
//	}
type SourcePositionError interface {
	PositionError
	// SourceFile returns the file path of the source where the error occurred,
	// or an empty string for errors in the main query.
	SourceFile() string
	// SourceText returns the full source text of the file where the error
	// occurred, for use with [LineColumn].
	SourceText() string
}

// StackTraceError extends [SourcePositionError] with a call stack trace.
// The error location is available via Position/SourceFile/SourceText (inherited
// from [SourcePositionError]). [StackTraceError.CallStack] returns the chain of call sites
// leading to the error, from nearest caller to outermost.
type StackTraceError interface {
	SourcePositionError
	// CallStack returns the call frames from the nearest caller to the outermost.
	// Returns nil when no call chain information is available.
	CallStack() []StackFrame
}

// FormatError returns a human-readable error message. If err implements
// [PositionError] (which all compile-time and common runtime errors from this
// package do), the message is prefixed with the 1-based line and column number
// computed from src, e.g. "1:8: function not defined: undefined_func/0".
// Otherwise it returns err.Error() unchanged.
//
// Use [FormatErrorAt] to include a filename in the output.
//
//	code, err := gojq.Compile(query)
//	if err != nil { log.Fatal(gojq.FormatError(src, err)) }
//
//	iter := code.Run(input)
//	for {
//	    v, ok := iter.Next()
//	    if !ok { break }
//	    if err, ok := v.(error); ok {
//	        log.Fatal(gojq.FormatError(src, err))
//	    }
//	}
func FormatError(src string, err error) string {
	var msg string
	if spe, ok := err.(SourcePositionError); ok && spe.SourceFile() != "" {
		line, col := LineColumn(spe.SourceText(), spe.Position())
		msg = fmt.Sprintf("%s:%d:%d: %s", absPath(spe.SourceFile()), line, col+1, err)
	} else if pe, ok := err.(PositionError); ok {
		line, col := LineColumn(src, pe.Position())
		msg = fmt.Sprintf("%d:%d: %s", line, col+1, err)
	} else {
		return err.Error()
	}
	if ste, ok := err.(StackTraceError); ok {
		msg += formatCallStack(src, "", ste.CallStack())
	}
	return msg
}

// FormatErrorAt is like [FormatError] but also includes fname in the output,
// producing messages of the form "fname:line:col: message". This matches the
// standard Go/compiler error format and is suitable for errors from queries
// loaded from named files:
//
//	src, _ := os.ReadFile("myfile.jq")
//	query, _ := gojq.Parse(string(src))
//
//	code, err := gojq.Compile(query)
//	if err != nil { log.Fatal(gojq.FormatErrorAt("myfile.jq", string(src), err)) }
//	// → "/home/user/myfile.jq:3:5: function not defined: undefined_func/0"
//
//	iter := code.Run(input)
//	for {
//	    v, ok := iter.Next()
//	    if !ok { break }
//	    if err, ok := v.(error); ok {
//	        log.Fatal(gojq.FormatErrorAt("myfile.jq", string(src), err))
//	        // → "/home/user/myfile.jq:5:12: expected a string for object key but got: null"
//	    }
//	}
//
// When err has no position info, it returns "absPath(fname): message".
func FormatErrorAt(fname, src string, err error) string {
	fname = absPath(fname)
	var msg string
	if spe, ok := err.(SourcePositionError); ok && spe.SourceFile() != "" {
		line, col := LineColumn(spe.SourceText(), spe.Position())
		msg = fmt.Sprintf("%s:%d:%d: %s", absPath(spe.SourceFile()), line, col+1, err)
	} else if pe, ok := err.(PositionError); ok {
		line, col := LineColumn(src, pe.Position())
		msg = fmt.Sprintf("%s:%d:%d: %s", fname, line, col+1, err)
	} else {
		return fname + ": " + err.Error()
	}
	if ste, ok := err.(StackTraceError); ok {
		msg += formatCallStack(src, fname, ste.CallStack())
	}
	return msg
}

// formatCallStack formats call stack frames as "\n    ↳ fname:line:col" lines.
func formatCallStack(mainSrc, mainFname string, frames []StackFrame) string {
	if len(frames) == 0 {
		return ""
	}
	var b strings.Builder
	for _, f := range frames {
		fname, src := absPath(f.SourceFile), f.SourceText
		if f.SourceFile == "" {
			fname, src = mainFname, mainSrc
		}
		if fname == "" {
			fname = "<query>"
		}
		line, col := LineColumn(src, f.Offset)
		fmt.Fprintf(&b, "\n    ↳ %s:%d:%d", fname, line, col+1)
	}
	return b.String()
}

// absPath resolves a file path to absolute. If the path is empty or
// resolution fails, the original path is returned unchanged.
func absPath(path string) string {
	if path == "" {
		return path
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// LineColumn converts a byte offset in src into a 1-based line number and a
// 0-based column number (measured in Unicode code points). It is designed to
// be used together with [PositionError.Position].
//
// Line endings are normalized: both bare `\r` and `\r\n` sequences count as a
// single newline, matching the behavior of the CLI's source-context display.
func LineColumn(src string, offset int) (line, column int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(src) {
		offset = len(src)
	}
	before := src[:offset]
	line = 1
	lastNLEnd := -1 // byte index of the character after the most recent newline
	for i := 0; i < len(before); i++ {
		switch before[i] {
		case '\r':
			line++
			// treat \r\n as a single newline
			if i+1 < len(before) && before[i+1] == '\n' {
				i++
			}
			lastNLEnd = i + 1
		case '\n':
			line++
			lastNLEnd = i + 1
		}
	}
	if lastNLEnd < 0 {
		column = utf8.RuneCountInString(before)
	} else {
		column = utf8.RuneCountInString(before[lastNLEnd:])
	}
	return
}
