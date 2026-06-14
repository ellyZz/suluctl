package syncids

import (
	"strings"
	"testing"
)

const pySrc = `import allure
from sulu_pytest import sulu_test


def test_create_pet_returns_http_200(created_pet):
    assert created_pet.status_code == 200


@sulu_test(id="PET-9")
def test_already_linked():
    pass
`

func TestPytestParse(t *testing.T) {
	f := &pytestFramework{}
	refs, _ := f.Parse("/x/tests/test_pet_create.py", []byte(pySrc))
	if len(refs) != 2 {
		t.Fatalf("want 2, got %d", len(refs))
	}
	if refs[0].FullName != "tests.test_pet_create#test_create_pet_returns_http_200" || refs[0].HasID {
		t.Fatalf("ref0 = %+v", refs[0])
	}
	if !refs[1].HasID {
		t.Fatal("ref1 must be detected as already linked")
	}
}

func TestPytestWrite(t *testing.T) {
	f := &pytestFramework{}
	refs, _ := f.Parse("/x/tests/test_pet_create.py", []byte(pySrc))
	out, changed, _ := f.Write([]byte(pySrc), refs[0], "PET-6")
	s := string(out)
	if !changed || !strings.Contains(s, "@sulu_test(id=\"PET-6\")\ndef test_create_pet_returns_http_200") {
		t.Fatalf("decorator not inserted above def:\n%s", s)
	}
	if !strings.Contains(s, "from sulu_pytest import sulu_test") {
		t.Fatal("import missing")
	}
}
