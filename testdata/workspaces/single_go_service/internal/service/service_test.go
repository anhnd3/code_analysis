package service

import "testing"

func TestHandle(t *testing.T) {
	got := Handle("x")
	if got == "" {
		t.Fatal("expected non-empty output")
	}
}
