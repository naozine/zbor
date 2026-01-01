package asr

// AlignmentDiffItem represents a single character in the alignment diff
type AlignmentDiffItem struct {
	Char string `json:"char"` // The character
	Op   string `json:"op"`   // "match", "insert", or "delete"
}

// AlignResult contains the aligned tokens and diff information
type AlignResult struct {
	Tokens       []Token             // Aligned tokens with timestamps
	Segments     []Segment           // Redistributed segments
	RawText      string              // Original Whisper output
	Diff         []AlignmentDiffItem // Character-by-character diff
	OriginalText string              // Original text from tokens
}

// AlignTokensWithText aligns Whisper text with original tokens, preserving timestamps
// where characters match and interpolating timestamps for new characters.
//
// This is useful when:
// - Original tokens have accurate timestamps (from SenseVoice/ReazonSpeech)
// - Whisper has better text recognition but no timestamps
// - We want to combine both: Whisper's text + original timestamps
func AlignTokensWithText(originalTokens []Token, whisperText string) []Token {
	if len(originalTokens) == 0 || whisperText == "" {
		return nil
	}

	// Build original text and character-to-token mapping
	originalRunes := []rune{}
	runeToToken := []int{} // maps each rune index to original token index
	for i, token := range originalTokens {
		for range []rune(token.Text) {
			originalRunes = append(originalRunes, []rune(token.Text)...)
			for range []rune(token.Text) {
				runeToToken = append(runeToToken, i)
			}
			break // only process once per token
		}
	}

	// Rebuild properly
	originalRunes = []rune{}
	runeToToken = []int{}
	for i, token := range originalTokens {
		tokenRunes := []rune(token.Text)
		for _, r := range tokenRunes {
			originalRunes = append(originalRunes, r)
			runeToToken = append(runeToToken, i)
		}
	}

	whisperRunes := []rune(whisperText)

	// Compute LCS and alignment
	alignment := computeAlignment(originalRunes, whisperRunes)

	// Build new tokens based on alignment
	return buildAlignedTokens(originalTokens, runeToToken, whisperRunes, alignment)
}

// alignmentOp represents an alignment operation
type alignmentOp int

const (
	opMatch  alignmentOp = iota // Characters match, keep original timestamp
	opInsert                    // Character added in whisper, interpolate timestamp
	opDelete                    // Character removed from original, skip
)

// alignmentEntry represents one step in the alignment
type alignmentEntry struct {
	op           alignmentOp
	origIdx      int  // index in original (-1 for insert)
	whisperIdx   int  // index in whisper (-1 for delete)
	whisperRune  rune // the character from whisper (for match/insert)
}

// computeAlignment uses LCS-based algorithm to align two rune sequences
func computeAlignment(original, whisper []rune) []alignmentEntry {
	m, n := len(original), len(whisper)

	// DP table for LCS
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	// Fill DP table
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if original[i-1] == whisper[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				dp[i][j] = max(dp[i-1][j], dp[i][j-1])
			}
		}
	}

	// Backtrack to find alignment
	var alignment []alignmentEntry
	i, j := m, n

	for i > 0 || j > 0 {
		if i > 0 && j > 0 && original[i-1] == whisper[j-1] {
			// Match
			alignment = append(alignment, alignmentEntry{
				op:          opMatch,
				origIdx:     i - 1,
				whisperIdx:  j - 1,
				whisperRune: whisper[j-1],
			})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			// Insert (character in whisper but not in original)
			alignment = append(alignment, alignmentEntry{
				op:          opInsert,
				origIdx:     -1,
				whisperIdx:  j - 1,
				whisperRune: whisper[j-1],
			})
			j--
		} else {
			// Delete (character in original but not in whisper)
			alignment = append(alignment, alignmentEntry{
				op:         opDelete,
				origIdx:    i - 1,
				whisperIdx: -1,
			})
			i--
		}
	}

	// Reverse alignment (we built it backwards)
	for left, right := 0, len(alignment)-1; left < right; left, right = left+1, right-1 {
		alignment[left], alignment[right] = alignment[right], alignment[left]
	}

	return alignment
}

// anchor represents a timestamp reference point for interpolation
type anchor struct {
	whisperIdx int
	time       float32
	duration   float32
}

// buildAlignedTokens creates new tokens based on alignment
func buildAlignedTokens(originalTokens []Token, runeToToken []int, whisperRunes []rune, alignment []alignmentEntry) []Token {
	if len(alignment) == 0 {
		return nil
	}

	// First pass: collect timestamp anchors from matched characters
	var anchors []anchor

	for _, entry := range alignment {
		if entry.op == opMatch && entry.origIdx >= 0 && entry.origIdx < len(runeToToken) {
			tokenIdx := runeToToken[entry.origIdx]
			if tokenIdx < len(originalTokens) {
				anchors = append(anchors, anchor{
					whisperIdx: entry.whisperIdx,
					time:       originalTokens[tokenIdx].StartTime,
					duration:   originalTokens[tokenIdx].Duration,
				})
			}
		}
	}

	// If no anchors, fall back to uniform distribution
	if len(anchors) == 0 {
		if len(originalTokens) == 0 {
			return nil
		}
		startTime := originalTokens[0].StartTime
		lastToken := originalTokens[len(originalTokens)-1]
		endTime := lastToken.StartTime + lastToken.Duration
		return distributeUniformly(whisperRunes, float64(startTime), float64(endTime))
	}

	// Second pass: build tokens with interpolated timestamps
	var result []Token

	for _, entry := range alignment {
		if entry.op == opDelete {
			continue // Skip deleted characters
		}

		char := string(entry.whisperRune)
		timestamp := interpolateTimestamp(entry.whisperIdx, anchors)

		result = append(result, Token{
			Text:      char,
			StartTime: timestamp,
			Duration:  estimateDuration(entry.whisperIdx, anchors, len(whisperRunes)),
		})
	}

	return result
}

// interpolateTimestamp calculates timestamp for a whisper character index
func interpolateTimestamp(whisperIdx int, anchors []anchor) float32 {
	if len(anchors) == 0 {
		return 0
	}

	// Find surrounding anchors
	var prevAnchor, nextAnchor *anchor

	for i := range anchors {
		if anchors[i].whisperIdx <= whisperIdx {
			prevAnchor = &anchors[i]
		}
		if anchors[i].whisperIdx >= whisperIdx && nextAnchor == nil {
			nextAnchor = &anchors[i]
		}
	}

	// Exact match
	if prevAnchor != nil && prevAnchor.whisperIdx == whisperIdx {
		return prevAnchor.time
	}

	// Only previous anchor available
	if nextAnchor == nil && prevAnchor != nil {
		// Extrapolate forward
		if len(anchors) >= 2 {
			last := anchors[len(anchors)-1]
			secondLast := anchors[len(anchors)-2]
			if last.whisperIdx > secondLast.whisperIdx {
				rate := (last.time - secondLast.time) / float32(last.whisperIdx-secondLast.whisperIdx)
				return last.time + rate*float32(whisperIdx-last.whisperIdx)
			}
		}
		return prevAnchor.time + prevAnchor.duration
	}

	// Only next anchor available
	if prevAnchor == nil && nextAnchor != nil {
		// Extrapolate backward
		if len(anchors) >= 2 {
			first := anchors[0]
			second := anchors[1]
			if second.whisperIdx > first.whisperIdx {
				rate := (second.time - first.time) / float32(second.whisperIdx-first.whisperIdx)
				return first.time - rate*float32(first.whisperIdx-whisperIdx)
			}
		}
		return nextAnchor.time
	}

	// Interpolate between anchors
	if prevAnchor != nil && nextAnchor != nil {
		if prevAnchor.whisperIdx == nextAnchor.whisperIdx {
			return prevAnchor.time
		}
		ratio := float32(whisperIdx-prevAnchor.whisperIdx) / float32(nextAnchor.whisperIdx-prevAnchor.whisperIdx)
		return prevAnchor.time + ratio*(nextAnchor.time-prevAnchor.time)
	}

	return 0
}

// estimateDuration estimates duration for a character
func estimateDuration(whisperIdx int, anchors []anchor, totalChars int) float32 {
	if len(anchors) == 0 || totalChars == 0 {
		return 0.1 // Default duration
	}

	// Calculate average duration from anchors
	var totalDuration float32
	for _, a := range anchors {
		totalDuration += a.duration
	}
	avgDuration := totalDuration / float32(len(anchors))

	// Use average duration, but cap it
	if avgDuration < 0.01 {
		avgDuration = 0.1
	}
	if avgDuration > 1.0 {
		avgDuration = 0.3
	}

	return avgDuration
}

// distributeUniformly creates tokens with uniform timestamp distribution
func distributeUniformly(runes []rune, startTime, endTime float64) []Token {
	if len(runes) == 0 {
		return nil
	}

	duration := endTime - startTime
	charDuration := duration / float64(len(runes))

	tokens := make([]Token, len(runes))
	for i, r := range runes {
		tokens[i] = Token{
			Text:      string(r),
			StartTime: float32(startTime + float64(i)*charDuration),
			Duration:  float32(charDuration),
		}
	}

	return tokens
}

// AlignTokensForSegments aligns Whisper text with original tokens across multiple segments,
// then redistributes aligned tokens back to segment boundaries
func AlignTokensForSegments(originalTokens []Token, whisperText string, segments []Segment, startIdx, endIdx int) ([]Token, []Segment) {
	result := AlignTokensForSegmentsWithDiff(originalTokens, whisperText, segments, startIdx, endIdx)
	return result.Tokens, result.Segments
}

// AlignTokensForSegmentsWithDiff aligns Whisper text with original tokens and returns full diff information
func AlignTokensForSegmentsWithDiff(originalTokens []Token, whisperText string, segments []Segment, startIdx, endIdx int) *AlignResult {
	result := &AlignResult{
		RawText: whisperText,
	}

	// Step 1: Collect tokens from target segments
	var segmentTokens []Token
	for _, token := range originalTokens {
		for i := startIdx; i <= endIdx && i < len(segments); i++ {
			seg := segments[i]
			if float64(token.StartTime) >= seg.StartTime && float64(token.StartTime) < seg.EndTime {
				segmentTokens = append(segmentTokens, token)
				break
			}
		}
	}

	// Handle edge case: include tokens at segment end boundary
	if endIdx < len(segments) {
		lastSeg := segments[endIdx]
		for _, token := range originalTokens {
			if float64(token.StartTime) >= lastSeg.EndTime && float64(token.StartTime) <= lastSeg.EndTime+0.01 {
				segmentTokens = append(segmentTokens, token)
			}
		}
	}

	// Build original text
	for _, token := range segmentTokens {
		result.OriginalText += token.Text
	}

	// Step 2: Compute alignment and build diff
	originalRunes := []rune(result.OriginalText)
	whisperRunes := []rune(whisperText)
	alignment := computeAlignment(originalRunes, whisperRunes)

	// Build diff from alignment
	for _, entry := range alignment {
		switch entry.op {
		case opMatch:
			result.Diff = append(result.Diff, AlignmentDiffItem{
				Char: string(entry.whisperRune),
				Op:   "match",
			})
		case opInsert:
			result.Diff = append(result.Diff, AlignmentDiffItem{
				Char: string(entry.whisperRune),
				Op:   "insert",
			})
		case opDelete:
			if entry.origIdx >= 0 && entry.origIdx < len(originalRunes) {
				result.Diff = append(result.Diff, AlignmentDiffItem{
					Char: string(originalRunes[entry.origIdx]),
					Op:   "delete",
				})
			}
		}
	}

	// Step 3: Align tokens with Whisper text
	alignedTokens := AlignTokensWithText(segmentTokens, whisperText)
	if len(alignedTokens) == 0 {
		return result
	}

	// Step 4: Redistribute aligned tokens to segments by duration ratio
	// This is similar to MergeSegmentsByRatio - we distribute tokens proportionally
	// to segment duration to handle gaps between segments
	var totalDuration float64
	for i := startIdx; i <= endIdx && i < len(segments); i++ {
		seg := segments[i]
		duration := seg.EndTime - seg.StartTime
		if duration > 0 {
			totalDuration += duration
		}
	}

	// If all segments have zero duration, distribute evenly
	numSegments := endIdx - startIdx + 1
	if numSegments > len(segments)-startIdx {
		numSegments = len(segments) - startIdx
	}

	var finalTokens []Token
	newSegments := make([]Segment, 0, numSegments)
	tokenIndex := 0

	for i := startIdx; i <= endIdx && i < len(segments); i++ {
		seg := segments[i]
		duration := seg.EndTime - seg.StartTime

		var segText string
		var tokenCount int

		if totalDuration > 0 && duration > 0 {
			// Distribute by duration ratio
			ratio := duration / totalDuration
			tokenCount = int(float64(len(alignedTokens)) * ratio)
			// Ensure at least 1 token per segment if tokens remain
			if tokenCount == 0 && tokenIndex < len(alignedTokens) {
				tokenCount = 1
			}
		} else if totalDuration == 0 || duration == 0 {
			// Zero duration segment(s): distribute remaining tokens evenly
			segIdx := i - startIdx
			remainingSegs := numSegments - segIdx
			remainingTokens := len(alignedTokens) - tokenIndex
			if remainingSegs > 0 && remainingTokens > 0 {
				tokenCount = remainingTokens / remainingSegs
				if tokenCount == 0 {
					tokenCount = 1
				}
			}
		}

		// Last segment gets all remaining tokens
		if i == endIdx {
			tokenCount = len(alignedTokens) - tokenIndex
		}

		// Assign tokens to this segment and adjust timestamps
		for j := 0; j < tokenCount && tokenIndex < len(alignedTokens); j++ {
			token := alignedTokens[tokenIndex]
			segText += token.Text

			// Recalculate timestamp to fit within segment
			var newStartTime float32
			if duration > 0 && tokenCount > 0 {
				tokenRatio := float64(j) / float64(tokenCount)
				newStartTime = float32(seg.StartTime + duration*tokenRatio)
			} else {
				newStartTime = float32(seg.StartTime)
			}

			finalTokens = append(finalTokens, Token{
				Text:      token.Text,
				StartTime: newStartTime,
				Duration:  float32(duration / float64(max(tokenCount, 1))),
			})
			tokenIndex++
		}

		newSegments = append(newSegments, Segment{
			Text:      segText,
			StartTime: seg.StartTime,
			EndTime:   seg.EndTime,
		})
	}

	result.Tokens = finalTokens
	result.Segments = newSegments

	return result
}
