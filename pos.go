package gojq

import (
	"fmt"
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
//	    // → "myfile.jq:1:8: function not defined: undefined_func/0"
//	}
//
//	iter := code.Run(input)
//	for {
//	    v, ok := iter.Next()
//	    if !ok { break }
//	    if err, ok := v.(error); ok {
//	        log.Fatal(gojq.FormatErrorAt("myfile.jq", src, err))
//	        // → "myfile.jq:1:1: expected a string for object key but got: null"
//	    }
//	}
type PositionError interface {
	error
	// Position returns the byte offset in the query source where the error
	// occurred. Pass this value together with the original query string to
	// [LineColumn] to obtain a human-readable line and column number.
	Position() int
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
	if pe, ok := err.(PositionError); ok {
		line, col := LineColumn(src, pe.Position())
		return fmt.Sprintf("%d:%d: %s", line, col+1, err)
	}
	return err.Error()
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
//	// → "myfile.jq:3:5: function not defined: undefined_func/0"
//
//	iter := code.Run(input)
//	for {
//	    v, ok := iter.Next()
//	    if !ok { break }
//	    if err, ok := v.(error); ok {
//	        log.Fatal(gojq.FormatErrorAt("myfile.jq", string(src), err))
//	        // → "myfile.jq:5:12: expected a string for object key but got: null"
//	    }
//	}
//
// When err has no position info, it returns "fname: message".
func FormatErrorAt(fname, src string, err error) string {
	if pe, ok := err.(PositionError); ok {
		line, col := LineColumn(src, pe.Position())
		return fmt.Sprintf("%s:%d:%d: %s", fname, line, col+1, err)
	}
	return fname + ": " + err.Error()
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
