package explain

// maxChunkBytes bounds how many bytes of joined diff text a single map-phase
// chunk may carry, so each chunk call stays within a sane LLM request size.
const maxChunkBytes = 32 * 1024

// maxParallelCalls caps how many map-phase chunk calls run concurrently, so
// a large changeset cannot open an unbounded number of simultaneous LLM
// requests.
const maxParallelCalls = 4

// fileDiff pairs a repo-relative path with its already-fetched diff text.
type fileDiff struct {
	path string
	diff string
}

// planChunks greedily packs files, in input order, into groups whose total
// diff byte length stays at or under maxChunkBytes, so each group can be
// sent as one map-phase chunk request. A file whose own diff already
// exceeds maxChunkBytes is placed alone in its own group: it is never split
// across groups or truncated. Order is preserved both within each group and
// across the returned groups. An empty input returns nil, and an input that
// fits entirely within maxChunkBytes returns a single group, so a caller can
// cheaply detect "no chunking needed" via len(groups) <= 1.
func planChunks(files []fileDiff) [][]fileDiff {
	if len(files) == 0 {
		return nil
	}

	groups := make([][]fileDiff, 0, len(files))
	var current []fileDiff
	var currentBytes int

	flush := func() {
		if len(current) > 0 {
			groups = append(groups, current)
			current = nil
			currentBytes = 0
		}
	}

	for _, f := range files {
		size := len(f.diff)
		if size > maxChunkBytes {
			flush()
			groups = append(groups, []fileDiff{f})
			continue
		}
		if len(current) > 0 && currentBytes+size > maxChunkBytes {
			flush()
		}
		current = append(current, f)
		currentBytes += size
	}
	flush()

	return groups
}
