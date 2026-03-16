package main

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/omniboost/gojq"
)

// clearOffsets zeroes all fields named "Offset" in the AST recursively.
// Offsets are byte positions in the source string and differ when a function
// definition is re-parsed in isolation vs. as part of the full builtin.jq file.
func clearOffsets(v reflect.Value) {
	switch v.Kind() {
	case reflect.Ptr:
		if !v.IsNil() {
			clearOffsets(v.Elem())
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			if t.Field(i).Name == "Offset" {
				v.Field(i).SetInt(0)
			} else {
				clearOffsets(v.Field(i))
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			clearOffsets(v.Index(i))
		}
	}
}

func main() {
	cnt, err := os.ReadFile("builtin.jq")
	if err != nil {
		panic(err)
	}
	q, err := gojq.Parse(string(cnt))
	if err != nil {
		panic(err)
	}
	fds := make(map[string][]*gojq.FuncDef)
	for _, fd := range q.FuncDefs {
		fds[fd.Name] = append(fds[fd.Name], fd)
	}
	count := len(fds)
	names, i := make([]string, count), 0
	for n := range fds {
		names[i] = n
		i++
	}
	sort.Strings(names)
	for _, n := range names {
		var sb strings.Builder
		for _, fd := range fds[n] {
			fmt.Fprintf(&sb, "%s ", fd)
		}
		q, err := gojq.Parse(sb.String())
		if err != nil {
			panic(fmt.Sprintf("%s: %s", err, sb.String()))
		}
		// Clear Offset fields before comparing. Each FuncDef in fds[n] was parsed
		// as part of the full builtin.jq file, so its Offset values reflect byte
		// positions within that file. The re-parsed q.FuncDefs were parsed from an
		// isolated string, so their Offset values start from 0. Offsets are only
		// used for error reporting and are not part of the semantic AST, so we
		// zero them out on both sides before the structural equality check.
		clearOffsets(reflect.ValueOf(q.FuncDefs))
		clearOffsets(reflect.ValueOf(fds[n]))
		if !reflect.DeepEqual(q.FuncDefs, fds[n]) {
			fmt.Printf("failed: %s: %s %s\n", n, q.FuncDefs, fds[n])
			continue
		}
		fmt.Printf("ok: %s: %s\n", n, sb.String())
		count--
	}
	if count > 0 {
		os.Exit(1)
	}
}
