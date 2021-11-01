package fxt

import (
	"fmt"
	"io"
	"strings"
)

func Fprintfln(w io.Writer, tmpl string, a ...interface{}) {
	fmt.Fprintf(w, tmpl+"\n", a...)
}

func Printfln(tmpl string, a ...interface{}) {
	fmt.Printf(tmpl+"\n", a...)
}

func Printqln(a ...interface{}) {
	var v []string
	for _, a := range a {
		v = append(v, fmt.Sprintf("%q", a))
	}
	fmt.Println(strings.Join(v, " "))
}

func Printpvln(a ...interface{}) {
	var v []string
	for _, a := range a {
		v = append(v, fmt.Sprintf("%+v", a))
	}
	fmt.Println(strings.Join(v, " "))
}
