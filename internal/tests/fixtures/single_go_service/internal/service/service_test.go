package service

import "testing"

func TestHandle(t *testing.T) {
	if Handle("ok") == "" {
		t.Fatal("expected non-empty result")
	}
}
