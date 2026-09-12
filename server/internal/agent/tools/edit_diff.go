package tools

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type matchedEdit struct {
	editIndex   int
	matchIndex  int
	matchLength int
	newText     string
}

type lineSpan struct {
	start int
	end   int
}

type diffOperation struct {
	kind  byte
	lines []string
}

func detectLineEnding(content string) string {
	lf := strings.Index(content, "\n")
	crlf := strings.Index(content, "\r\n")
	if lf == -1 || crlf == -1 || crlf >= lf {
		return "\n"
	}
	return "\r\n"
}

func normalizeToLF(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func restoreLineEndings(text string, ending string) string {
	if ending == "\r\n" {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

func normalizeForFuzzyMatch(text string) string {
	text = norm.NFKC.String(text)
	lines := strings.Split(text, "\n")
	for index := range lines {
		lines[index] = strings.TrimRightFunc(lines[index], unicode.IsSpace)
	}
	text = strings.Join(lines, "\n")

	replacer := strings.NewReplacer(
		"\u2018", "'", "\u2019", "'", "\u201a", "'", "\u201b", "'",
		"\u201c", `"`, "\u201d", `"`, "\u201e", `"`, "\u201f", `"`,
		"\u2010", "-", "\u2011", "-", "\u2012", "-", "\u2013", "-",
		"\u2014", "-", "\u2015", "-", "\u2212", "-",
		"\u00a0", " ", "\u2002", " ", "\u2003", " ", "\u2004", " ",
		"\u2005", " ", "\u2006", " ", "\u2007", " ", "\u2008", " ",
		"\u2009", " ", "\u200a", " ", "\u202f", " ", "\u205f", " ",
		"\u3000", " ",
	)
	return replacer.Replace(text)
}

func splitLinesWithEndings(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.SplitAfter(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func getLineSpans(content string) []lineSpan {
	lines := splitLinesWithEndings(content)
	spans := make([]lineSpan, 0, len(lines))
	offset := 0
	for _, line := range lines {
		spans = append(spans, lineSpan{start: offset, end: offset + len(line)})
		offset += len(line)
	}
	return spans
}

func applyReplacements(content string, replacements []matchedEdit, offset int) string {
	result := content
	for index := len(replacements) - 1; index >= 0; index-- {
		replacement := replacements[index]
		start := replacement.matchIndex - offset
		result = result[:start] + replacement.newText + result[start+replacement.matchLength:]
	}
	return result
}

func applyReplacementsPreservingUnchangedLines(
	originalContent string,
	baseContent string,
	replacements []matchedEdit,
) (string, error) {
	originalLines := splitLinesWithEndings(originalContent)
	baseLines := getLineSpans(baseContent)
	if len(originalLines) != len(baseLines) {
		return "", fmt.Errorf("cannot preserve unchanged lines because the base content has a different line count")
	}

	type replacementGroup struct {
		startLine    int
		endLine      int
		replacements []matchedEdit
	}
	groups := make([]replacementGroup, 0, len(replacements))
	for _, replacement := range replacements {
		replacementStart := replacement.matchIndex
		replacementEnd := replacement.matchIndex + replacement.matchLength
		startLine := -1
		for index, line := range baseLines {
			if replacementStart >= line.start && replacementStart < line.end {
				startLine = index
				break
			}
		}
		if startLine == -1 {
			return "", fmt.Errorf("replacement range is outside the base content")
		}
		endLine := startLine
		for endLine < len(baseLines) && baseLines[endLine].end < replacementEnd {
			endLine++
		}
		if endLine >= len(baseLines) {
			return "", fmt.Errorf("replacement range is outside the base content")
		}
		endLine++

		if len(groups) > 0 && startLine < groups[len(groups)-1].endLine {
			current := &groups[len(groups)-1]
			current.endLine = max(current.endLine, endLine)
			current.replacements = append(current.replacements, replacement)
		} else {
			groups = append(groups, replacementGroup{
				startLine:    startLine,
				endLine:      endLine,
				replacements: []matchedEdit{replacement},
			})
		}
	}

	var result strings.Builder
	originalLineIndex := 0
	for _, group := range groups {
		result.WriteString(strings.Join(originalLines[originalLineIndex:group.startLine], ""))
		startOffset := baseLines[group.startLine].start
		endOffset := baseLines[group.endLine-1].end
		result.WriteString(applyReplacements(baseContent[startOffset:endOffset], group.replacements, startOffset))
		originalLineIndex = group.endLine
	}
	result.WriteString(strings.Join(originalLines[originalLineIndex:], ""))
	return result.String(), nil
}

func applyEditsToNormalizedContent(
	normalizedContent string,
	edits []EditReplacement,
	path string,
) (string, string, error) {
	normalizedEdits := make([]EditReplacement, len(edits))
	for index, edit := range edits {
		normalizedEdits[index] = EditReplacement{
			OldText: normalizeToLF(edit.OldText),
			NewText: normalizeToLF(edit.NewText),
		}
		if normalizedEdits[index].OldText == "" {
			if len(edits) == 1 {
				return "", "", fmt.Errorf("oldText must not be empty in %s.", path)
			}
			return "", "", fmt.Errorf("edits[%d].oldText must not be empty in %s.", index, path)
		}
	}

	usedFuzzy := false
	for _, edit := range normalizedEdits {
		if !strings.Contains(normalizedContent, edit.OldText) &&
			strings.Contains(normalizeForFuzzyMatch(normalizedContent), normalizeForFuzzyMatch(edit.OldText)) {
			usedFuzzy = true
			break
		}
	}
	replacementBase := normalizedContent
	if usedFuzzy {
		replacementBase = normalizeForFuzzyMatch(normalizedContent)
	}

	matches := make([]matchedEdit, 0, len(edits))
	for index, edit := range normalizedEdits {
		oldText := edit.OldText
		if usedFuzzy {
			oldText = normalizeForFuzzyMatch(oldText)
		}
		matchIndex := strings.Index(replacementBase, oldText)
		if matchIndex == -1 {
			if len(edits) == 1 {
				return "", "", fmt.Errorf(
					"Could not find the exact text in %s. The old text must match exactly including all whitespace and newlines.",
					path,
				)
			}
			return "", "", fmt.Errorf(
				"Could not find edits[%d] in %s. The oldText must match exactly including all whitespace and newlines.",
				index,
				path,
			)
		}
		occurrences := strings.Count(normalizeForFuzzyMatch(replacementBase), normalizeForFuzzyMatch(oldText))
		if occurrences > 1 {
			if len(edits) == 1 {
				return "", "", fmt.Errorf(
					"Found %d occurrences of the text in %s. The text must be unique. Please provide more context to make it unique.",
					occurrences,
					path,
				)
			}
			return "", "", fmt.Errorf(
				"Found %d occurrences of edits[%d] in %s. Each oldText must be unique. Please provide more context to make it unique.",
				occurrences,
				index,
				path,
			)
		}
		matches = append(matches, matchedEdit{
			editIndex:   index,
			matchIndex:  matchIndex,
			matchLength: len(oldText),
			newText:     edit.NewText,
		})
	}

	for left := 0; left < len(matches); left++ {
		for right := left + 1; right < len(matches); right++ {
			if matches[right].matchIndex < matches[left].matchIndex {
				matches[left], matches[right] = matches[right], matches[left]
			}
		}
	}
	for index := 1; index < len(matches); index++ {
		previous := matches[index-1]
		current := matches[index]
		if previous.matchIndex+previous.matchLength > current.matchIndex {
			return "", "", fmt.Errorf(
				"edits[%d] and edits[%d] overlap in %s. Merge them into one edit or target disjoint regions.",
				previous.editIndex,
				current.editIndex,
				path,
			)
		}
	}

	newContent := ""
	var err error
	if usedFuzzy {
		newContent, err = applyReplacementsPreservingUnchangedLines(normalizedContent, replacementBase, matches)
	} else {
		newContent = applyReplacements(replacementBase, matches, 0)
	}
	if err != nil {
		return "", "", err
	}
	if normalizedContent == newContent {
		if len(edits) == 1 {
			return "", "", fmt.Errorf(
				"No changes made to %s. The replacement produced identical content. This might indicate an issue with special characters or the text not existing as expected.",
				path,
			)
		}
		return "", "", fmt.Errorf("No changes made to %s. The replacements produced identical content.", path)
	}
	return normalizedContent, newContent, nil
}

func splitDiffLines(content string) []string {
	return splitLinesWithEndings(content)
}

func computeLineDiff(oldContent string, newContent string) []diffOperation {
	oldLines := splitDiffLines(oldContent)
	newLines := splitDiffLines(newContent)
	return diffLineSequences(oldLines, newLines)
}

func diffLineSequences(oldLines []string, newLines []string) []diffOperation {
	if len(oldLines) == 0 {
		return newDiffOperation('+', newLines)
	}
	if len(newLines) == 0 {
		return newDiffOperation('-', oldLines)
	}
	prefixLength := 0
	for prefixLength < len(oldLines) &&
		prefixLength < len(newLines) &&
		oldLines[prefixLength] == newLines[prefixLength] {
		prefixLength++
	}
	suffixLength := 0
	for suffixLength < len(oldLines)-prefixLength &&
		suffixLength < len(newLines)-prefixLength &&
		oldLines[len(oldLines)-1-suffixLength] == newLines[len(newLines)-1-suffixLength] {
		suffixLength++
	}
	if prefixLength > 0 || suffixLength > 0 {
		oldMiddleEnd := len(oldLines) - suffixLength
		newMiddleEnd := len(newLines) - suffixLength
		return mergeDiffOperations(
			newDiffOperation(' ', oldLines[:prefixLength]),
			diffLineSequences(oldLines[prefixLength:oldMiddleEnd], newLines[prefixLength:newMiddleEnd]),
			newDiffOperation(' ', oldLines[oldMiddleEnd:]),
		)
	}
	if len(oldLines) == 1 {
		for index, line := range newLines {
			if oldLines[0] == line {
				return mergeDiffOperations(
					newDiffOperation('+', newLines[:index]),
					newDiffOperation(' ', oldLines),
					newDiffOperation('+', newLines[index+1:]),
				)
			}
		}
		return mergeDiffOperations(newDiffOperation('-', oldLines), newDiffOperation('+', newLines))
	}

	middle := len(oldLines) / 2
	leftLengths := lcsPrefixLengths(oldLines[:middle], newLines)
	rightLengths := lcsSuffixLengths(oldLines[middle:], newLines)
	split := 0
	best := -1
	for index := 0; index <= len(newLines); index++ {
		score := leftLengths[index] + rightLengths[index]
		if score > best {
			best = score
			split = index
		}
	}
	leftLengths = nil
	rightLengths = nil
	return mergeDiffOperations(
		diffLineSequences(oldLines[:middle], newLines[:split]),
		diffLineSequences(oldLines[middle:], newLines[split:]),
	)
}

func lcsPrefixLengths(left []string, right []string) []int {
	previous := make([]int, len(right)+1)
	for _, leftLine := range left {
		current := make([]int, len(right)+1)
		for index, rightLine := range right {
			if leftLine == rightLine {
				current[index+1] = previous[index] + 1
			} else {
				current[index+1] = max(previous[index+1], current[index])
			}
		}
		previous = current
	}
	return previous
}

func lcsSuffixLengths(left []string, right []string) []int {
	next := make([]int, len(right)+1)
	for leftIndex := len(left) - 1; leftIndex >= 0; leftIndex-- {
		current := make([]int, len(right)+1)
		for rightIndex := len(right) - 1; rightIndex >= 0; rightIndex-- {
			if left[leftIndex] == right[rightIndex] {
				current[rightIndex] = next[rightIndex+1] + 1
			} else {
				current[rightIndex] = max(next[rightIndex], current[rightIndex+1])
			}
		}
		next = current
	}
	return next
}

func newDiffOperation(kind byte, lines []string) []diffOperation {
	if len(lines) == 0 {
		return nil
	}
	return []diffOperation{{kind: kind, lines: append([]string(nil), lines...)}}
}

func mergeDiffOperations(groups ...[]diffOperation) []diffOperation {
	merged := make([]diffOperation, 0)
	for _, group := range groups {
		for _, operation := range group {
			if len(operation.lines) == 0 {
				continue
			}
			if len(merged) > 0 && merged[len(merged)-1].kind == operation.kind {
				merged[len(merged)-1].lines = append(merged[len(merged)-1].lines, operation.lines...)
			} else {
				merged = append(merged, operation)
			}
		}
	}
	return merged
}

func displayDiffLine(line string) string {
	return strings.TrimSuffix(line, "\n")
}

func generateDiffString(oldContent string, newContent string, contextLines int) (string, *int) {
	operations := computeLineDiff(oldContent, newContent)
	oldLineNumber, newLineNumber := 1, 1
	maxLineNumber := max(len(splitDiffLines(oldContent)), len(splitDiffLines(newContent)))
	width := len(fmt.Sprint(max(1, maxLineNumber)))
	lastWasChange := false
	var firstChangedLine *int
	output := make([]string, 0)

	for index, operation := range operations {
		if operation.kind == '+' || operation.kind == '-' {
			if firstChangedLine == nil {
				firstChangedLine = intPointer(newLineNumber)
			}
			for _, line := range operation.lines {
				line = displayDiffLine(line)
				if operation.kind == '+' {
					output = append(output, fmt.Sprintf("+%*d %s", width, newLineNumber, line))
					newLineNumber++
				} else {
					output = append(output, fmt.Sprintf("-%*d %s", width, oldLineNumber, line))
					oldLineNumber++
				}
			}
			lastWasChange = true
			continue
		}

		nextIsChange := index+1 < len(operations) &&
			(operations[index+1].kind == '+' || operations[index+1].kind == '-')
		lines := operation.lines
		switch {
		case lastWasChange && nextIsChange && len(lines) <= contextLines*2:
			for _, line := range lines {
				output = append(output, fmt.Sprintf(" %*d %s", width, oldLineNumber, displayDiffLine(line)))
				oldLineNumber++
				newLineNumber++
			}
		case lastWasChange && nextIsChange:
			for _, line := range lines[:contextLines] {
				output = append(output, fmt.Sprintf(" %*d %s", width, oldLineNumber, displayDiffLine(line)))
				oldLineNumber++
				newLineNumber++
			}
			skipped := len(lines) - contextLines*2
			output = append(output, " "+strings.Repeat(" ", width)+" ...")
			oldLineNumber += skipped
			newLineNumber += skipped
			for _, line := range lines[len(lines)-contextLines:] {
				output = append(output, fmt.Sprintf(" %*d %s", width, oldLineNumber, displayDiffLine(line)))
				oldLineNumber++
				newLineNumber++
			}
		case lastWasChange:
			shown := min(contextLines, len(lines))
			for _, line := range lines[:shown] {
				output = append(output, fmt.Sprintf(" %*d %s", width, oldLineNumber, displayDiffLine(line)))
				oldLineNumber++
				newLineNumber++
			}
			if skipped := len(lines) - shown; skipped > 0 {
				output = append(output, " "+strings.Repeat(" ", width)+" ...")
				oldLineNumber += skipped
				newLineNumber += skipped
			}
		case nextIsChange:
			skipped := max(0, len(lines)-contextLines)
			if skipped > 0 {
				output = append(output, " "+strings.Repeat(" ", width)+" ...")
				oldLineNumber += skipped
				newLineNumber += skipped
			}
			for _, line := range lines[skipped:] {
				output = append(output, fmt.Sprintf(" %*d %s", width, oldLineNumber, displayDiffLine(line)))
				oldLineNumber++
				newLineNumber++
			}
		default:
			oldLineNumber += len(lines)
			newLineNumber += len(lines)
		}
		lastWasChange = false
	}
	return strings.Join(output, "\n"), firstChangedLine
}

func generateUnifiedPatch(path string, oldContent string, newContent string) string {
	operations := computeLineDiff(oldContent, newContent)
	oldLines := splitDiffLines(oldContent)
	newLines := splitDiffLines(newContent)
	var patch strings.Builder
	patch.WriteString("===================================================================\n")
	fmt.Fprintf(&patch, "--- %s\n+++ %s\n", path, path)
	fmt.Fprintf(&patch, "@@ -1,%d +1,%d @@\n", len(oldLines), len(newLines))
	for _, operation := range operations {
		for _, line := range operation.lines {
			patch.WriteByte(operation.kind)
			patch.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				patch.WriteByte('\n')
				patch.WriteString("\\ No newline at end of file\n")
			}
		}
	}
	return patch.String()
}
