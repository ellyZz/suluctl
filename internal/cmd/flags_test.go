package cmd

import "testing"

func TestStringListAccumulates(t *testing.T) {
	var s stringList
	_ = s.Set("smoke")
	_ = s.Set("nightly")
	if len(s) != 2 || s[0] != "smoke" || s[1] != "nightly" {
		t.Errorf("got %v", s)
	}
}

func TestKVMapParses(t *testing.T) {
	m := kvMap{}
	if err := m.Set("BRANCH=main"); err != nil {
		t.Fatal(err)
	}
	if err := m.Set("OPTS=a=b"); err != nil { // value may contain '='
		t.Fatal(err)
	}
	if m["BRANCH"] != "main" || m["OPTS"] != "a=b" {
		t.Errorf("got %v", m)
	}
	if err := m.Set("noequals"); err == nil {
		t.Error("want error for missing '='")
	}
	if err := m.Set("=v"); err == nil {
		t.Error("want error for empty key")
	}
}
