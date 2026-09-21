package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
)

func decodeEditInput(rawInput json.RawMessage) (EditInput, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawInput, &fields); err != nil {
		return EditInput{}, err
	}
	if fields == nil {
		return EditInput{}, errors.New("tool input must be a JSON object")
	}
	pathJSON, found := fields["path"]
	if !found {
		return EditInput{}, fmt.Errorf("missing required property: path")
	}
	if bytes.Equal(bytes.TrimSpace(pathJSON), []byte("null")) {
		return EditInput{}, errors.New("property path must not be null")
	}
	var path string
	if err := json.Unmarshal(pathJSON, &path); err != nil {
		return EditInput{}, err
	}

	input := EditInput{Path: path}
	if editsJSON, found := fields["edits"]; found {
		if bytes.Equal(bytes.TrimSpace(editsJSON), []byte("null")) {
			return EditInput{}, errors.New("property edits must not be null")
		}
		edits, err := decodeEditReplacements(editsJSON)
		if err != nil {
			return EditInput{}, err
		}
		input.Edits = edits
	}
	oldTextJSON, hasOldText := fields["oldText"]
	newTextJSON, hasNewText := fields["newText"]
	var oldText string
	var newText string
	oldTextValid := hasOldText && json.Unmarshal(oldTextJSON, &oldText) == nil &&
		!bytes.Equal(bytes.TrimSpace(oldTextJSON), []byte("null"))
	newTextValid := hasNewText && json.Unmarshal(newTextJSON, &newText) == nil &&
		!bytes.Equal(bytes.TrimSpace(newTextJSON), []byte("null"))
	if oldTextValid && newTextValid {
		input.Edits = append(input.Edits, EditReplacement{
			OldText: oldText,
			NewText: newText,
		})
	}
	return input, nil
}

func decodeEditReplacements(raw json.RawMessage) ([]EditReplacement, error) {
	var encoded string
	if err := json.Unmarshal(raw, &encoded); err == nil {
		return decodeEditReplacements(json.RawMessage(encoded))
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		items = []json.RawMessage{raw}
	}
	replacements := make([]EditReplacement, 0, len(items))
	for _, item := range items {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item, &fields); err != nil || fields == nil {
			if err == nil {
				err = errors.New("edit replacement must be a JSON object")
			}
			return nil, err
		}
		oldTextJSON, hasOldText := fields["oldText"]
		newTextJSON, hasNewText := fields["newText"]
		if !hasOldText || !hasNewText {
			return nil, errors.New("edit replacement requires oldText and newText")
		}
		oldText, err := decodeRequiredString(oldTextJSON, "oldText")
		if err != nil {
			return nil, err
		}
		newText, err := decodeRequiredString(newTextJSON, "newText")
		if err != nil {
			return nil, err
		}
		replacements = append(replacements, EditReplacement{OldText: oldText, NewText: newText})
	}
	return replacements, nil
}

func decodeRequiredString(raw json.RawMessage, field string) (string, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", fmt.Errorf("property %s must not be null", field)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

func (executor *Executor) Edit(ctx context.Context, cwd string, input EditInput) (ToolResult, error) {
	executor = executor.withDefaults()
	if len(input.Edits) == 0 {
		return ToolResult{}, fmt.Errorf("Edit tool input is invalid. edits must contain at least one replacement.")
	}
	absolutePath, err := executor.resolveToCWD(input.Path, cwd)
	if err != nil {
		return ToolResult{}, err
	}
	return executor.withFileMutation(absolutePath, func() (ToolResult, error) {
		if err := contextOperationError(ctx); err != nil {
			return ToolResult{}, err
		}
		if err := executor.FS.Access(absolutePath); err != nil {
			return ToolResult{}, fmt.Errorf("Could not edit file: %s. %s.", input.Path, formatFileError(err))
		}
		rawContent, err := executor.FS.ReadFile(absolutePath)
		if err != nil {
			return ToolResult{}, fmt.Errorf("Could not edit file: %s. %s.", input.Path, formatFileError(err))
		}
		if err := contextOperationError(ctx); err != nil {
			return ToolResult{}, err
		}

		bom := []byte(nil)
		if len(rawContent) >= 3 && rawContent[0] == 0xef && rawContent[1] == 0xbb && rawContent[2] == 0xbf {
			bom = []byte{0xef, 0xbb, 0xbf}
			rawContent = rawContent[3:]
		}
		content := stringsToValidUTF8(rawContent)
		lineEnding := detectLineEnding(content)
		baseContent, newContent, err := applyEditsToNormalizedContent(normalizeToLF(content), input.Edits, input.Path)
		if err != nil {
			return ToolResult{}, err
		}
		diff, firstChangedLine := generateDiffString(baseContent, newContent, 4)
		details := &ResultDetails{
			Diff:             diff,
			Patch:            generateUnifiedPatch(input.Path, baseContent, newContent),
			FirstChangedLine: firstChangedLine,
		}
		finalContent := append(bom, []byte(restoreLineEndings(newContent, lineEnding))...)
		if err := executor.FS.WriteFile(absolutePath, finalContent, 0o644); err != nil {
			return ToolResult{}, fmt.Errorf("Could not edit file: %s. %s.", input.Path, formatFileError(err))
		}
		if err := applyEditGuard(executor.FS, executor.Lint, absolutePath, content, string(finalContent), true); err != nil {
			return ToolResult{}, err
		}
		if err := contextOperationError(ctx); err != nil {
			return ToolResult{}, err
		}

		return textResult(
			fmt.Sprintf("Successfully replaced %d block(s) in %s.", len(input.Edits), input.Path),
			details,
		), nil
	})
}

func formatFileError(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "Error code: ENOENT"
	case errors.Is(err, fs.ErrPermission):
		return "Error code: EACCES"
	default:
		return fmt.Sprintf("Error: %s", err)
	}
}

func stringsToValidUTF8(data []byte) string {
	return string([]rune(string(data)))
}
