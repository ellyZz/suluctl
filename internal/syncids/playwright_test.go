package syncids

import (
	"strings"
	"testing"
)

// Lines 1-6 precede the test() call so it lands on line 7, column 1.
const pwSrc = `import { expect, test } from './support/sulu';

const X = 1;



test('Create pet returns HTTP 200', async ({ request }) => {
  expect(1).toBe(1);
});
`

func TestPlaywrightParse(t *testing.T) {
	f := &playwrightFramework{testDir: "tests"}
	refs, _ := f.Parse("/repo/tests/pet-create.spec.ts", []byte(pwSrc))
	if len(refs) != 1 {
		t.Fatalf("want 1, got %d", len(refs))
	}
	if refs[0].FullName != "pet-create.spec.ts:7:1" {
		t.Fatalf("fullName = %s", refs[0].FullName)
	}
}

func TestPlaywrightWrite(t *testing.T) {
	f := &playwrightFramework{testDir: "tests"}
	refs, _ := f.Parse("/repo/tests/pet-create.spec.ts", []byte(pwSrc))
	out, changed, _ := f.Write([]byte(pwSrc), refs[0], "PET-6")
	if !changed || !strings.Contains(string(out), `annotation: { type: 'sulu', description: 'PET-6' }`) {
		t.Fatalf("annotation not inserted:\n%s", out)
	}
}
