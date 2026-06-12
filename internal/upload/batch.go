package upload

import "github.com/ellyZz/suluctl/internal/scan"

const (
	MaxBatchFiles = 100
	MaxBatchBytes = 190 << 20 // stays under the server's 200 MB multipart request cap
	// SoloFileBytes mirrors the server's per-file cap default (SULU_IMPORT_MAX_FILE_MB=50).
	// Spring's servlet-level max-file-size rejects the ENTIRE multipart request when any
	// single part exceeds it, so a potentially-oversize file must never share a batch —
	// alone, its rejection is isolated to that one file (spec §6).
	SoloFileBytes = 50 << 20
)

// Batches splits files into upload requests of ≤MaxBatchFiles and ≤MaxBatchBytes;
// any file larger than SoloFileBytes rides alone in its own batch.
func Batches(files []scan.FileState) [][]scan.FileState {
	var out [][]scan.FileState
	var cur []scan.FileState
	var curBytes int64
	flush := func() {
		if len(cur) > 0 {
			out = append(out, cur)
			cur, curBytes = nil, 0
		}
	}
	for _, f := range files {
		if f.Size > SoloFileBytes {
			flush()
			out = append(out, []scan.FileState{f})
			continue
		}
		if len(cur) >= MaxBatchFiles || curBytes+f.Size > MaxBatchBytes {
			flush()
		}
		cur = append(cur, f)
		curBytes += f.Size
	}
	flush()
	return out
}
