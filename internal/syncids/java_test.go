package syncids

import (
	"strings"
	"testing"
)

const javaSrc = `package petstore;

import org.testng.annotations.Test;

public class PetCreateTests {

    @Test(description = "x")
    public void createPet_returnsHttp200() {
    }

    @Test
    @SuluTest(id = "PET-9")
    public void already_linked() {
    }
}
`

func TestJavaParse(t *testing.T) {
	f := &javaFramework{pkg: "petstore"}
	refs, err := f.Parse("/x/PetCreateTests.java", []byte(javaSrc))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("want 2 refs, got %d", len(refs))
	}
	if refs[0].FullName != "petstore.PetCreateTests.createPet_returnsHttp200" || refs[0].HasID {
		t.Fatalf("ref0 = %+v", refs[0])
	}
	if refs[1].FullName != "petstore.PetCreateTests.already_linked" || !refs[1].HasID {
		t.Fatalf("ref1 = %+v", refs[1])
	}
}

func TestJavaWrite_insertsAnnotationAndImport(t *testing.T) {
	f := &javaFramework{pkg: "petstore"}
	refs, _ := f.Parse("/x/PetCreateTests.java", []byte(javaSrc))
	out, changed, err := f.Write([]byte(javaSrc), refs[0], "PET-6")
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	s := string(out)
	if !strings.Contains(s, `@SuluTest(id = "PET-6")`) {
		t.Fatalf("annotation not inserted:\n%s", s)
	}
	if !strings.Contains(s, "import petstore.annotation.SuluTest;") {
		t.Fatal("import not inserted (or wrong FQN)")
	}
	// Idempotent: an already-id'd method is not rewritten.
	if _, changed2, _ := f.Write(out, refs[1], "PET-9"); changed2 {
		t.Fatal("already-linked method must not change")
	}
}
