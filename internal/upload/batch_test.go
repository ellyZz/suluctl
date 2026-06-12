package upload

import (
	"fmt"
	"testing"

	"github.com/ellyZz/suluctl/internal/scan"
)

func fakeFiles(n int, size int64) []scan.FileState {
	out := make([]scan.FileState, n)
	for i := range out {
		out[i] = scan.FileState{Path: fmt.Sprintf("f%03d", i), Size: size}
	}
	return out
}

func TestBatchesByCount(t *testing.T) {
	b := Batches(fakeFiles(250, 10))
	if len(b) != 3 || len(b[0]) != 100 || len(b[1]) != 100 || len(b[2]) != 50 {
		t.Errorf("got %d batches: %d/%d/...", len(b), len(b[0]), len(b[1]))
	}
}

func TestBatchesByBytes(t *testing.T) {
	// 5 files of 45 MB (under the 50 MB solo cap): 45*4=180 ≤ 190, +45 → 225 > 190 → split 4+1
	b := Batches(fakeFiles(5, 45<<20))
	if len(b) != 2 || len(b[0]) != 4 || len(b[1]) != 1 {
		t.Errorf("got %v", lens(b))
	}
}

func TestOversizeFileGoesAlone(t *testing.T) {
	// a file over the server's 50 MB per-file cap must never share a request:
	// Spring rejects the WHOLE multipart request when any part exceeds the cap
	files := []scan.FileState{
		{Path: "a", Size: 10},
		{Path: "huge", Size: 60 << 20},
		{Path: "b", Size: 10},
	}
	b := Batches(files)
	if len(b) != 3 || len(b[1]) != 1 || b[1][0].Path != "huge" {
		t.Errorf("huge file must ride alone, got %v", lens(b))
	}
}

func TestEmptyInput(t *testing.T) {
	if b := Batches(nil); len(b) != 0 {
		t.Errorf("got %v", lens(b))
	}
}

func TestBoundaryExactlySoloCapSharesBatch(t *testing.T) {
	// strict >: a file of exactly SoloFileBytes must SHARE a batch
	files := []scan.FileState{
		{Path: "a", Size: SoloFileBytes},
		{Path: "b", Size: 10},
	}
	if b := Batches(files); len(b) != 1 || len(b[0]) != 2 {
		t.Errorf("exactly-cap file must share, got %v", lens(b))
	}
}

func TestBoundaryExactlyMaxBatchBytesStaysOneBatch(t *testing.T) {
	files := fakeFiles(4, 45<<20)
	files = append(files, scan.FileState{Path: "last", Size: MaxBatchBytes - 4*(45<<20)})
	if b := Batches(files); len(b) != 1 || len(b[0]) != 5 {
		t.Errorf("exact byte-cap sum must stay one batch, got %v", lens(b))
	}
}

func lens(b [][]scan.FileState) []int {
	out := make([]int, len(b))
	for i, x := range b {
		out[i] = len(x)
	}
	return out
}
