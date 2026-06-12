package cmd

import (
	"errors"
	"testing"

	"github.com/ellyZz/suluctl/internal/api"
	"github.com/ellyZz/suluctl/internal/upload"
)

func TestIsolatableFileErrorMatrix(t *testing.T) {
	oversize := int64(upload.SoloFileBytes + 1)
	normal := int64(1024)
	cases := []struct {
		name string
		err  error
		size int64
		want bool
	}{
		{"network never isolated", errors.New("dial tcp: refused"), oversize, false},
		{"402 never isolated", &api.APIError{Status: 402}, oversize, false},
		{"409 never isolated", &api.APIError{Status: 409}, normal, false},
		{"500 normal size = outage", &api.APIError{Status: 500}, normal, false},
		{"429 normal size = outage", &api.APIError{Status: 429}, normal, false},
		{"500 oversize = the oversize rejection", &api.APIError{Status: 500}, oversize, true},
		{"413 any size isolated", &api.APIError{Status: 413}, normal, true},
	}
	for _, c := range cases {
		if got := isolatableFileError(c.err, c.size); got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}
