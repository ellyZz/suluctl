package syncids

import (
	"strings"
	"testing"
)

const csSrc = `using Sulu.XUnit;

namespace PetstoreApiTests.Tests;

public class UserLoginLogoutTests
{
    [Fact, SuluTest("")]
    public async Task Login_Returns200() { }

    [Fact, SuluTest("PET-9")]
    public async Task Logout_Returns200() { }
}
`

func TestXunitParse(t *testing.T) {
	f := &xunitFramework{}
	refs, _ := f.Parse("/x/UserLoginLogoutTests.cs", []byte(csSrc))
	if len(refs) != 2 {
		t.Fatalf("want 2, got %d", len(refs))
	}
	if refs[0].FullName != "PetstoreApiTests.Tests.UserLoginLogoutTests.Login_Returns200" {
		t.Fatalf("fullName = %s", refs[0].FullName)
	}
	if !refs[0].EmptyID || refs[0].HasID {
		t.Fatalf("ref0 should be EmptyID placeholder: %+v", refs[0])
	}
	if !refs[1].HasID {
		t.Fatal("ref1 has a real id")
	}
}

func TestXunitWrite_fillsEmptyOnly(t *testing.T) {
	f := &xunitFramework{}
	refs, _ := f.Parse("/x/UserLoginLogoutTests.cs", []byte(csSrc))
	out, changed, _ := f.Write([]byte(csSrc), refs[0], "PET-1")
	if !changed || !strings.Contains(string(out), `SuluTest("PET-1")`) {
		t.Fatalf("empty placeholder not filled:\n%s", out)
	}
	if _, changed2, _ := f.Write([]byte(csSrc), refs[1], "PET-X"); changed2 {
		t.Fatal("real id must not be overwritten")
	}
}
