package main

import (
	"bytes"
	"errors"
	"testing"
)

func TestBoundedWriter(t *testing.T) {
	var out bytes.Buffer
	w := &boundedWriter{w: &out, remaining: 4}
	if n, err := w.Write([]byte("abc")); n != 3 || err != nil {
		t.Fatalf("write: %d %v", n, err)
	}
	if n, err := w.Write([]byte("de")); n != 0 || !errors.Is(err, errSize) {
		t.Fatalf("over limit: %d %v", n, err)
	}
	if out.String() != "abc" || w.remaining != 1 {
		t.Fatal("overflow changed file or budget")
	}
	if n, err := w.Write([]byte("d")); n != 1 || err != nil {
		t.Fatalf("exact limit: %d %v", n, err)
	}
}
