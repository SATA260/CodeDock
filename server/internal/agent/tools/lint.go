package tools

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/scanner"
	"go/token"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// LintSeverity 诊断级别。
type LintSeverity string

const (
	SeverityError   LintSeverity = "error"
	SeverityWarning LintSeverity = "warning"
)

// LintDiagnostic 一条语法或编译轻检诊断。
type LintDiagnostic struct {
	Line     int          `json:"line"`
	Column   int          `json:"column"`
	Message  string       `json:"message"`
	Severity LintSeverity `json:"severity"`
}

// EditInspection 写后增量语法检查结果。
type EditInspection struct {
	FilePath     string           `json:"file_path"`
	OldContent   string           `json:"old_content,omitempty"`
	NewContent   string           `json:"new_content,omitempty"`
	NewErrors    []LintDiagnostic `json:"new_errors,omitempty"`
	RollbackDone bool             `json:"rollback_done"`
	DiffSnippet  string           `json:"diff_snippet,omitempty"`
}

// EditInspector 写后检查接口，测试可替换。
type EditInspector interface {
	InspectEdit(filePath, oldContent, newContent string) (EditInspection, error)
}

// defaultInspector 是默认的本地语法检查。
type defaultInspector struct{}

// InspectEdit 对比新旧内容，只报告本次新引入的硬伤。
func InspectEdit(filePath, oldContent, newContent string) (EditInspection, error) {
	return defaultInspector{}.InspectEdit(filePath, oldContent, newContent)
}

// InspectEdit 对比新旧内容，只报告本次新引入的硬伤。
func (defaultInspector) InspectEdit(filePath, oldContent, newContent string) (EditInspection, error) {
	out := EditInspection{FilePath: filePath, OldContent: oldContent, NewContent: newContent}
	oldErrs := lintContent(filePath, oldContent)
	newErrs := lintContent(filePath, newContent)
	out.NewErrors = incrementalErrors(oldErrs, newErrs)
	if len(out.NewErrors) > 0 {
		out.DiffSnippet = lintDiffSnippet(oldContent, newContent, out.NewErrors[0].Line)
	}
	return out, nil
}

// lintContent 按扩展名做语法轻检；不认识的文件视为无错误。
func lintContent(filePath, content string) []LintDiagnostic {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return lintGo(filePath, content)
	case ".json":
		return lintJSON(content)
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
		return lintBalanced(content)
	default:
		return nil
	}
}

// lintGo 用 go/parser 检查单个文件。
func lintGo(filePath, content string) []LintDiagnostic {
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, filePath, content, parser.AllErrors)
	if err == nil {
		return nil
	}
	var out []LintDiagnostic
	if list, ok := err.(scanner.ErrorList); ok {
		for _, item := range list {
			out = append(out, LintDiagnostic{
				Line:     item.Pos.Line,
				Column:   item.Pos.Column,
				Message:  item.Msg,
				Severity: SeverityError,
			})
		}
		return out
	}
	return []LintDiagnostic{{Line: 1, Column: 1, Message: err.Error(), Severity: SeverityError}}
}

// lintJSON 检查 JSON 是否能解析。
func lintJSON(content string) []LintDiagnostic {
	if strings.TrimSpace(content) == "" {
		return []LintDiagnostic{{Line: 1, Column: 1, Message: "empty JSON", Severity: SeverityError}}
	}
	if err := jsonRawValid(content); err != nil {
		return []LintDiagnostic{{Line: 1, Column: 1, Message: err.Error(), Severity: SeverityError}}
	}
	return nil
}

// lintBalanced 检查括号是否配对，用于 JS/TS 轻检。
func lintBalanced(content string) []LintDiagnostic {
	type pair struct {
		open rune
		line int
		col  int
	}
	var stack []pair
	line, col := 1, 1
	inStr := rune(0)
	escape := false
	for _, r := range content {
		if inStr != 0 {
			if escape {
				escape = false
			} else if r == '\\' {
				escape = true
			} else if r == inStr {
				inStr = 0
			}
		} else if r == '"' || r == '\'' || r == '`' {
			inStr = r
		} else if r == '(' || r == '[' || r == '{' {
			stack = append(stack, pair{r, line, col})
		} else if r == ')' || r == ']' || r == '}' {
			want := map[rune]rune{')': '(', ']': '[', '}': '{'}[r]
			if len(stack) == 0 || stack[len(stack)-1].open != want {
				return []LintDiagnostic{{Line: line, Column: col, Message: fmt.Sprintf("unmatched %q", string(r)), Severity: SeverityError}}
			}
			stack = stack[:len(stack)-1]
		}
		if r == '\n' {
			line++
			col = 1
		} else {
			col += utf8.RuneLen(r)
			if col < 1 {
				col = 1
			}
		}
	}
	if len(stack) > 0 {
		last := stack[len(stack)-1]
		return []LintDiagnostic{{Line: last.line, Column: last.col, Message: fmt.Sprintf("unclosed %q", string(last.open)), Severity: SeverityError}}
	}
	return nil
}

// incrementalErrors 去掉修改前已有的同类报错。
func incrementalErrors(oldErrs, newErrs []LintDiagnostic) []LintDiagnostic {
	seen := map[string]struct{}{}
	for _, item := range oldErrs {
		seen[item.Message] = struct{}{}
	}
	var out []LintDiagnostic
	for _, item := range newErrs {
		if _, ok := seen[item.Message]; ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

// lintDiffSnippet 截出错误行附近的新旧对比。
func lintDiffSnippet(oldContent, newContent string, line int) string {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")
	start := line - 3
	if start < 1 {
		start = 1
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("around line %d\n", line))
	for i := start; i <= line+1 && i <= len(oldLines); i++ {
		b.WriteString(fmt.Sprintf("- %d: %s\n", i, oldLines[i-1]))
	}
	for i := start; i <= line+1 && i <= len(newLines); i++ {
		b.WriteString(fmt.Sprintf("+ %d: %s\n", i, newLines[i-1]))
	}
	return b.String()
}

// formatInspectionError 把检查失败格式化成工具错误，提醒模型不要原样重试。
func formatInspectionError(inspection EditInspection) string {
	var b strings.Builder
	b.WriteString("edit rejected: new syntax/compile errors were introduced and the file was rolled back.\n")
	for _, item := range inspection.NewErrors {
		b.WriteString(fmt.Sprintf("- line %d:%d %s\n", item.Line, item.Column, item.Message))
	}
	if inspection.DiffSnippet != "" {
		b.WriteString(inspection.DiffSnippet)
	}
	b.WriteString("Do not retry the same broken edit.")
	return b.String()
}

// applyEditGuard 写盘后做增量检查；有新硬伤则回滚并返回错误。
func applyEditGuard(fs FileSystem, inspector EditInspector, filePath, oldContent, newContent string, existed bool) error {
	if inspector == nil {
		inspector = defaultInspector{}
	}
	inspection, err := inspector.InspectEdit(filePath, oldContent, newContent)
	if err != nil {
		return err
	}
	if len(inspection.NewErrors) == 0 {
		return nil
	}
	if existed {
		if err := fs.WriteFile(filePath, []byte(oldContent), 0o644); err != nil {
			return fmt.Errorf("%s; rollback failed: %v", formatInspectionError(inspection), err)
		}
	} else if err := fs.Remove(filePath); err != nil {
		_ = fs.WriteFile(filePath, []byte(oldContent), 0o644)
		return fmt.Errorf("%s; rollback failed: %v", formatInspectionError(inspection), err)
	}
	return fmt.Errorf("%s", formatInspectionError(inspection))
}

// jsonRawValid 判断文本是不是合法 JSON。
func jsonRawValid(content string) error {
	var value any
	return json.Unmarshal([]byte(content), &value)
}
