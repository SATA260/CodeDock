package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxDotEnvParents = 3

// LoadDotEnv 从工作目录向上查找 .env，只填充尚未设置的环境变量。
// 找不到文件视为成功；文件损坏返回错误。
func LoadDotEnv() error {
	path, err := findDotEnv()
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	pairs, err := parseDotEnvFile(path)
	if err != nil {
		return err
	}
	for key, value := range pairs {
		if _, set := os.LookupEnv(key); set {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("dotenv: set %s: %w", key, err)
		}
	}
	return nil
}

// findDotEnv 从 cwd 向上最多 maxDotEnvParents 层查找 .env。
func findDotEnv() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("dotenv: getwd: %w", err)
	}
	for i := 0; i <= maxDotEnvParents; i++ {
		candidate := filepath.Join(dir, ".env")
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("dotenv: stat %s: %w", candidate, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", nil
}

// parseDotEnvFile 读取 KEY=VALUE，支持 # 注释、引号和空行。
func parseDotEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("dotenv: open %s: %w", path, err)
	}
	defer file.Close()

	out := make(map[string]string)
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, err := parseDotEnvLine(line)
		if err != nil {
			return nil, fmt.Errorf("dotenv: %s:%d: %w", path, lineNo, err)
		}
		out[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("dotenv: read %s: %w", path, err)
	}
	return out, nil
}

func parseDotEnvLine(line string) (string, string, error) {
	key, raw, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", fmt.Errorf("missing '='")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", fmt.Errorf("empty key")
	}
	value := strings.TrimSpace(raw)
	if value == "" {
		return key, "", nil
	}
	if unquoted, ok := unquote(value); ok {
		return key, unquoted, nil
	}
	if i := strings.Index(value, " #"); i >= 0 {
		value = strings.TrimSpace(value[:i])
	}
	return key, value, nil
}

func unquote(value string) (string, bool) {
	if len(value) < 2 {
		return "", false
	}
	quote := value[0]
	if quote != '"' && quote != '\'' {
		return "", false
	}
	end := strings.LastIndexByte(value, quote)
	if end <= 0 {
		return "", false
	}
	return value[1:end], true
}
