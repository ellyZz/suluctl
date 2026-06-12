package cmd

import (
	"fmt"
	"strings"
)

// stringList is a repeatable string flag (--tag a --tag b).
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// kvMap is a repeatable K=V flag (--env-var BRANCH=main).
type kvMap map[string]string

func (m *kvMap) String() string { return fmt.Sprintf("%v", map[string]string(*m)) }
func (m *kvMap) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	if !ok || k == "" {
		return fmt.Errorf("expected K=V, got %q", v)
	}
	if *m == nil {
		*m = kvMap{}
	}
	(*m)[k] = val
	return nil
}
