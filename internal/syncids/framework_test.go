package syncids

import (
	"testing"

	"github.com/ellyZz/suluctl/internal/initscaffold"
)

func TestForReturnsAdapterPerKind(t *testing.T) {
	for _, k := range []string{"testng", "junit5", "pytest", "playwright", "xunit"} {
		if For(initscaffold.Kind(k), Options{}) == nil {
			t.Fatalf("no adapter for %s", k)
		}
	}
	if For(initscaffold.Kind("nope"), Options{}) != nil {
		t.Fatal("unknown kind should return nil")
	}
}
