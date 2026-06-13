package initscaffold

import "testing"

func TestDetectJavaBasePackage(t *testing.T) {
	t.Run("shallowest wins", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "src/test/java/com/acme/deep/nest/FooTest.java", "package com.acme.deep.nest;\nclass FooTest{}")
		write(t, dir, "src/test/java/com/acme/BarTest.java", "package com.acme;\nclass BarTest{}")
		if got := DetectJavaBasePackage(dir); got != "com.acme" {
			t.Fatalf("got %q, want com.acme", got)
		}
	})
	t.Run("fallback sulu", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "README.md", "no java here")
		if got := DetectJavaBasePackage(dir); got != "sulu" {
			t.Fatalf("got %q, want sulu", got)
		}
	})
}
