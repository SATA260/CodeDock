package tools

import (
	"context"
	"encoding/json"
	"io/fs"
)

const (
	ToolRead       = "read"
	ToolBash       = "bash"
	ToolPowerShell = "powershell"
	ToolEdit       = "edit"
	ToolWrite      = "write"
	ToolGrep       = "grep"
	ToolFind       = "find"
	ToolLS         = "ls"
)

var ToolNames = []string{
	ToolRead,
	ToolBash,
	ToolPowerShell,
	ToolEdit,
	ToolWrite,
	ToolGrep,
	ToolFind,
	ToolLS,
}

type ContentBlock struct {
	Type     string `json:"type"`               // text 或 image
	Text     string `json:"text,omitempty"`     // type=text 时的正文
	Data     string `json:"data,omitempty"`     // type=image 时的 base64
	MIMEType string `json:"mimeType,omitempty"` // type=image 时的 MIME
}

func (block ContentBlock) MarshalJSON() ([]byte, error) {
	switch block.Type {
	case "text":
		return json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: block.Type, Text: block.Text})
	case "image":
		return json.Marshal(struct {
			Type     string `json:"type"`
			Data     string `json:"data"`
			MIMEType string `json:"mimeType"`
		}{Type: block.Type, Data: block.Data, MIMEType: block.MIMEType})
	default:
		type rawContentBlock ContentBlock
		return json.Marshal(rawContentBlock(block))
	}
}

type ToolResult struct {
	Content []ContentBlock `json:"content"`
	Details *ResultDetails `json:"details,omitempty"`
}

type ResultDetails struct {
	Truncation         *TruncationResult `json:"truncation,omitempty"`
	FullOutputPath     string            `json:"fullOutputPath,omitempty"`   // 超长输出落到的旁路文件
	Diff               string            `json:"diff,omitempty"`             // 给人看的行级 diff
	Patch              string            `json:"patch,omitempty"`            // unified patch
	FirstChangedLine   *int              `json:"firstChangedLine,omitempty"` // 第一处改动的新文件行号
	MatchLimitReached  *float64          `json:"matchLimitReached,omitempty"`
	LinesTruncated     *bool             `json:"linesTruncated,omitempty"`
	ResultLimitReached *float64          `json:"resultLimitReached,omitempty"`
	EntryLimitReached  *float64          `json:"entryLimitReached,omitempty"`
}

type ReadInput struct {
	Path   string   `json:"path"`             // 相对工作区或绝对路径
	Offset *float64 `json:"offset,omitempty"` // 起始行，从 1 计
	Limit  *float64 `json:"limit,omitempty"`  // 最多返回多少行
}

type ShellInput struct {
	Command string   `json:"command"`
	Timeout *float64 `json:"timeout,omitempty"` // 秒；空则用执行器默认
}

type EditReplacement struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

type EditInput struct {
	Path  string            `json:"path"`
	Edits []EditReplacement `json:"edits"`
}

type WriteInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type GrepInput struct {
	Pattern    string   `json:"pattern"`
	Path       *string  `json:"path,omitempty"` // 搜索起点；空则工作区根
	Glob       *string  `json:"glob,omitempty"`
	IgnoreCase *bool    `json:"ignoreCase,omitempty"`
	Literal    *bool    `json:"literal,omitempty"` // 按字面量而不是正则
	Context    *float64 `json:"context,omitempty"` // 匹配行上下各取几行
	Limit      *float64 `json:"limit,omitempty"`
}

type FindInput struct {
	Pattern string   `json:"pattern"`
	Path    *string  `json:"path,omitempty"` // 搜索起点；空则工作区根
	Limit   *float64 `json:"limit,omitempty"`
}

type LSInput struct {
	Path  *string  `json:"path,omitempty"` // 要列出的目录；空则工作区根
	Limit *float64 `json:"limit,omitempty"`
}

type CommandResult struct {
	ExitCode int
}

type CommandFunc func(
	ctx context.Context,
	name string,
	args []string,
	dir string,
	env []string,
	onStdout func(data []byte),
	onStderr func(data []byte),
) (CommandResult, error)

type FileSystem interface {
	Access(name string) error
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	MkdirAll(path string, perm fs.FileMode) error
	Stat(name string) (fs.FileInfo, error)
	ReadDir(name string) ([]fs.DirEntry, error)
	EvalSymlinks(path string) (string, error)
}

type Executor struct {
	FS         FileSystem
	RunCommand CommandFunc
	LookPath   func(file string) (string, error)
	TempDir    string
	GOOS       string
	HomeDir    string
	Env        []string
}

func textResult(text string, details *ResultDetails) ToolResult {
	return ToolResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
		Details: details,
	}
}

func boolPointer(value bool) *bool {
	return &value
}

func intPointer(value int) *int {
	return &value
}

func floatPointer(value float64) *float64 {
	return &value
}
