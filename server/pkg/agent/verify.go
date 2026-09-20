package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// VerifyStatus 工作区验证的判定状态。
type VerifyStatus string

const (
	VerifyStatusPassed    VerifyStatus = "passed"     // 全部验证命令退出码为 0
	VerifyStatusFailed    VerifyStatus = "failed"     // 测试或构建失败，打回模型
	VerifyStatusCannotRun VerifyStatus = "cannot_run" // 外部环境异常，转人工
)

// VerifyRule 单个范围的验证规则。
type VerifyRule struct {
	Commands   []string `json:"commands" yaml:"commands"`       // 按顺序执行的命令
	TimeoutSec int      `json:"timeout_sec" yaml:"timeout_sec"` // 单条命令超时秒数，默认 120
	MaxRounds  int      `json:"max_rounds" yaml:"max_rounds"`   // 本规则允许的打回次数，0 表示用全局
	ScopeGlob  string   `json:"scope_glob" yaml:"scope_glob"`   // 命中改动文件才跑；空表示总是跑
}

// VerifyFile 是 .cursor/verify.yaml 的可解析形态。
type VerifyFile struct {
	Rules                 []VerifyRule `json:"rules" yaml:"rules"`
	CheckNewTestsOnBase   bool         `json:"check_new_tests_on_base" yaml:"check_new_tests_on_base"` // 可选：新测试必须在旧代码上失败
}

// VerifyResult 验证流水线输出。
type VerifyResult struct {
	Status        VerifyStatus `json:"status"`
	FailedCommand string       `json:"failed_command,omitempty"`
	ExitCode      int          `json:"exit_code,omitempty"`
	Output        string       `json:"output,omitempty"`
	Fingerprint   string       `json:"fingerprint,omitempty"`
	Round         int          `json:"round"`
	DurationMs    int64        `json:"duration_ms"`
	Skipped       bool         `json:"skipped,omitempty"`
	CoverageNote  string       `json:"coverage_note,omitempty"`
}

// CommandRunner 在工作区执行一条 shell 命令。
type CommandRunner func(ctx context.Context, dir, command string, timeout time.Duration) (exitCode int, output string, err error)

// CheckWorkspace 按改动文件挑选规则并顺序执行验证命令。
func CheckWorkspace(ctx context.Context, workspaceRoot string, diffFiles []string, round int, lastFingerprint string, runner CommandRunner) (VerifyResult, error) {
	started := time.Now()
	rules, err := loadVerifyRules(workspaceRoot)
	if err != nil {
		return VerifyResult{}, err
	}
	if len(rules) == 0 {
		return VerifyResult{Status: VerifyStatusPassed, Skipped: true, Round: round, DurationMs: time.Since(started).Milliseconds()}, nil
	}
	selected := selectVerifyRules(rules, diffFiles)
	if len(selected) == 0 {
		return VerifyResult{Status: VerifyStatusPassed, Skipped: true, Round: round, DurationMs: time.Since(started).Milliseconds()}, nil
	}
	if runner == nil {
		runner = RunShellCommand
	}
	for _, rule := range selected {
		timeout := time.Duration(rule.TimeoutSec) * time.Second
		if timeout <= 0 {
			timeout = 120 * time.Second
		}
		for _, command := range rule.Commands {
			if strings.TrimSpace(command) == "" {
				continue
			}
			if err := ctx.Err(); err != nil {
				return VerifyResult{Status: VerifyStatusCannotRun, Output: err.Error(), Round: round, DurationMs: time.Since(started).Milliseconds()}, nil
			}
			code, output, runErr := runner(ctx, workspaceRoot, command, timeout)
			if runErr != nil && code == 0 {
				return VerifyResult{
					Status:        VerifyStatusCannotRun,
					FailedCommand: command,
					Output:        clipVerifyOutput(runErr.Error() + "\n" + output),
					Round:         round,
					DurationMs:    time.Since(started).Milliseconds(),
				}, nil
			}
			if code != 0 {
				fp := VerifyFingerprint(command, output)
				return VerifyResult{
					Status:        VerifyStatusFailed,
					FailedCommand: command,
					ExitCode:      code,
					Output:        clipVerifyOutput(output),
					Fingerprint:   fp,
					Round:         round,
					DurationMs:    time.Since(started).Milliseconds(),
				}, nil
			}
		}
	}
	_ = lastFingerprint
	return VerifyResult{Status: VerifyStatusPassed, Round: round, DurationMs: time.Since(started).Milliseconds()}, nil
}

// IsLooping 判断验证失败是否已原地空转或超出次数。
func IsLooping(currentFingerprint, lastFingerprint string, round, maxRounds int) bool {
	if maxRounds > 0 && round >= maxRounds {
		return true
	}
	if currentFingerprint != "" && currentFingerprint == lastFingerprint {
		return true
	}
	return false
}

// VerifyFingerprint 用失败命令与报错文本生成稳定指纹。
func VerifyFingerprint(command, output string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(command) + "\n" + normalizeFingerprintText(output)))
	return hex.EncodeToString(sum[:16])
}

// RunShellCommand 用 bash -lc 在指定目录执行命令。
func RunShellCommand(ctx context.Context, dir, command string, timeout time.Duration) (int, string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out), nil
	}
	if ctx.Err() != nil {
		return -1, string(out), ctx.Err()
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode(), string(out), nil
	}
	return -1, string(out), err
}

// loadVerifyRules 读取工作区 .cursor/verify.yaml；文件不存在视为无规则。
func loadVerifyRules(workspaceRoot string) ([]VerifyRule, error) {
	path := filepath.Join(workspaceRoot, ".cursor", "verify.yaml")
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	file, err := parseVerifyFile(string(body))
	if err != nil {
		return nil, err
	}
	return file.Rules, nil
}

// parseVerifyFile 解析 JSON 或极简 YAML 形态的验证规则。
func parseVerifyFile(raw string) (VerifyFile, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return VerifyFile{}, nil
	}
	var file VerifyFile
	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &file); err != nil {
			var rules []VerifyRule
			if err2 := json.Unmarshal([]byte(raw), &rules); err2 != nil {
				return VerifyFile{}, err
			}
			return VerifyFile{Rules: rules}, nil
		}
		return file, nil
	}
	return parseVerifyYAML(raw)
}

// parseVerifyYAML 解析本项目使用的扁平 YAML 规则文件。
func parseVerifyYAML(raw string) (VerifyFile, error) {
	var file VerifyFile
	var current *VerifyRule
	inCommands := false
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if trimmed == "rules:" {
			continue
		}
		if key, value, ok := strings.Cut(trimmed, ":"); ok && current == nil && !strings.HasPrefix(trimmed, "-") {
			if strings.TrimSpace(key) == "check_new_tests_on_base" {
				file.CheckNewTestsOnBase = strings.EqualFold(strings.Trim(strings.TrimSpace(value), `"'`), "true")
				continue
			}
		}
		if inCommands && current != nil && strings.HasPrefix(trimmed, "-") && indent >= 4 {
			cmd := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			cmd = strings.Trim(cmd, `"'`)
			if cmd != "" {
				current.Commands = append(current.Commands, cmd)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			file.Rules = append(file.Rules, VerifyRule{})
			current = &file.Rules[len(file.Rules)-1]
			inCommands = false
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			if rest == "commands:" {
				inCommands = true
			} else if rest != "" {
				applyVerifyYAMLField(current, rest)
				inCommands = strings.HasPrefix(rest, "commands:")
			}
			continue
		}
		if current == nil {
			file.Rules = append(file.Rules, VerifyRule{})
			current = &file.Rules[len(file.Rules)-1]
		}
		if trimmed == "commands:" {
			inCommands = true
			continue
		}
		if strings.HasPrefix(trimmed, "commands:") {
			inCommands = true
			applyVerifyYAMLField(current, trimmed)
			continue
		}
		if indent <= 4 {
			inCommands = false
		}
		applyVerifyYAMLField(current, trimmed)
	}
	return file, nil
}

// applyVerifyYAMLField 把一行 YAML 字段写进当前规则。
func applyVerifyYAMLField(rule *VerifyRule, line string) {
	if rule == nil {
		return
	}
	key, value, ok := strings.Cut(line, ":")
	if !ok {
		return
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	switch key {
	case "timeout_sec":
		fmt.Sscanf(value, "%d", &rule.TimeoutSec)
	case "max_rounds":
		fmt.Sscanf(value, "%d", &rule.MaxRounds)
	case "scope_glob":
		rule.ScopeGlob = value
	case "commands":
		if strings.HasPrefix(value, "[") {
			var items []string
			if json.Unmarshal([]byte(value), &items) == nil {
				rule.Commands = append(rule.Commands, items...)
			}
		}
	}
}

// NewTestFiles 从改动路径里挑出测试文件。
func NewTestFiles(changed []string) []string {
	var out []string
	for _, path := range changed {
		if isVerifyTestPath(path) {
			out = append(out, path)
		}
	}
	return out
}

// isVerifyTestPath 判断路径是否像测试文件。
func isVerifyTestPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(base, "_test.go") ||
		strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.")
}

// CheckNewTestsFailOnBase 在基线目录跑新增测试；若通过则说明没覆盖新行为。
func CheckNewTestsFailOnBase(ctx context.Context, baseDir string, newTests []string, runner CommandRunner) error {
	if len(newTests) == 0 || runner == nil {
		return nil
	}
	pkgs := map[string]struct{}{}
	for _, path := range newTests {
		dir := filepath.ToSlash(filepath.Dir(path))
		if dir == "" {
			dir = "."
		}
		pkgs[dir] = struct{}{}
	}
	for pkg := range pkgs {
		cmd := "go test ./" + pkg
		if pkg == "." {
			cmd = "go test ."
		}
		code, output, err := runner(ctx, baseDir, cmd, 2*time.Minute)
		if err != nil {
			return err
		}
		if code == 0 {
			return fmt.Errorf("新测试在改动前的代码上也通过，它没有覆盖本次改动：%s\n%s", cmd, clipVerifyOutput(output))
		}
	}
	return nil
}

// CoverageNote 把增量覆盖率观察写成交给复审的事实，不单独判定失败。
func CoverageNote(percent float64, threshold float64) string {
	if percent < 0 {
		return ""
	}
	if threshold <= 0 {
		threshold = 70
	}
	if percent < threshold {
		return fmt.Sprintf("增量覆盖率 %.1f%%，低于建议阈值 %.0f%%", percent, threshold)
	}
	return fmt.Sprintf("增量覆盖率 %.1f%%", percent)
}

// selectVerifyRules 只保留与改动文件匹配的规则；没有任何改动文件时跑全部。
func selectVerifyRules(rules []VerifyRule, diffFiles []string) []VerifyRule {
	if len(diffFiles) == 0 {
		return rules
	}
	out := make([]VerifyRule, 0, len(rules))
	for _, rule := range rules {
		if strings.TrimSpace(rule.ScopeGlob) == "" || matchScope(rule.ScopeGlob, diffFiles) {
			out = append(out, rule)
		}
	}
	return out
}

// matchScope 判断任一改动文件是否命中 glob。
func matchScope(pattern string, files []string) bool {
	for _, file := range files {
		ok, err := filepath.Match(pattern, file)
		if err == nil && ok {
			return true
		}
		ok, err = filepath.Match(pattern, filepath.ToSlash(file))
		if err == nil && ok {
			return true
		}
		if strings.Contains(pattern, "**") {
			prefix := strings.TrimSuffix(strings.ReplaceAll(pattern, "**", ""), "/")
			prefix = strings.TrimSuffix(prefix, "*")
			if prefix != "" && (strings.HasPrefix(filepath.ToSlash(file), strings.TrimSuffix(prefix, "/")) || strings.Contains(filepath.ToSlash(file), strings.Trim(prefix, "/*"))) {
				return true
			}
		}
	}
	return false
}

// clipVerifyOutput 保留报错尾部，避免把整份测试日志塞回模型。
func clipVerifyOutput(output string) string {
	const max = 8000
	output = strings.TrimSpace(output)
	if len(output) <= max {
		return output
	}
	return output[len(output)-max:]
}

// normalizeFingerprintText 去掉行号抖动，只留相对稳定的报错特征。
func normalizeFingerprintText(output string) string {
	lines := strings.Split(output, "\n")
	keep := make([]string, 0, 8)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "FAIL") || strings.Contains(line, "error:") || strings.Contains(line, "Error") || strings.Contains(line, "panic:") {
			keep = append(keep, line)
		}
	}
	if len(keep) == 0 && len(lines) > 0 {
		start := 0
		if len(lines) > 12 {
			start = len(lines) - 12
		}
		keep = lines[start:]
	}
	return strings.Join(keep, "\n")
}
