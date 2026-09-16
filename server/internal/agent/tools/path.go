package tools

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

var unicodeSpaces = regexp.MustCompile(`[\x{00A0}\x{2000}-\x{200A}\x{202F}\x{205F}\x{3000}]`)

func normalizePathInput(input string, homeDir string, goos string, stripAtPrefix bool) (string, error) {
	normalized := unicodeSpaces.ReplaceAllString(input, " ")
	if stripAtPrefix && strings.HasPrefix(normalized, "@") {
		normalized = normalized[1:]
	}
	if goos == "windows" {
		normalized = normalizeWindowsShellPath(normalized)
	}

	if normalized == "~" {
		return homeDir, nil
	}
	if strings.HasPrefix(normalized, "~/") || (goos == "windows" && strings.HasPrefix(normalized, `~\`)) {
		return filepath.Join(homeDir, normalized[2:]), nil
	}
	if strings.HasPrefix(normalized, "file://") {
		parsed, err := url.Parse(normalized)
		if err != nil {
			return "", err
		}
		urlPath, err := url.PathUnescape(parsed.EscapedPath())
		if err != nil {
			return "", err
		}
		if goos == "windows" {
			if parsed.Host != "" && parsed.Host != "localhost" {
				return `\\` + parsed.Host + filepath.FromSlash(urlPath), nil
			}
			if len(urlPath) >= 3 && urlPath[0] == '/' && urlPath[2] == ':' {
				urlPath = urlPath[1:]
			}
			return filepath.FromSlash(urlPath), nil
		}
		if parsed.Host != "" && parsed.Host != "localhost" {
			return "", fmt.Errorf("file URL host must be empty or localhost on %s", goos)
		}
		return filepath.FromSlash(urlPath), nil
	}
	return normalized, nil
}

func normalizeWindowsShellPath(path string) string {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, `\`) {
		return path
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 0 {
		return path
	}
	if parts[0] == "mnt" || parts[0] == "cygdrive" {
		parts = parts[1:]
	}
	if len(parts) == 0 || len(parts[0]) != 1 {
		return path
	}
	drive := strings.ToUpper(parts[0])
	if drive[0] < 'A' || drive[0] > 'Z' {
		return path
	}
	return drive + `:\` + strings.Join(parts[1:], `\`)
}

func (executor *Executor) resolveToCWD(path string, cwd string) (string, error) {
	path, err := normalizePathInput(path, executor.HomeDir, executor.GOOS, true)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return filepath.Clean(filepath.Join(cwd, path)), nil
}

func (executor *Executor) resolveReadPath(path string, cwd string) (string, error) {
	resolved, err := executor.resolveToCWD(path, cwd)
	if err != nil {
		return "", err
	}
	candidates := []string{
		resolved,
		tryMacOSScreenshotPath(resolved),
		norm.NFD.String(resolved),
		strings.ReplaceAll(resolved, "'", "\u2019"),
		strings.ReplaceAll(norm.NFD.String(resolved), "'", "\u2019"),
	}
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, found := seen[candidate]; found {
			continue
		}
		seen[candidate] = struct{}{}
		if _, err := executor.FS.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return resolved, nil
}

func tryMacOSScreenshotPath(path string) string {
	replacer := regexp.MustCompile(`(?i) (AM|PM)\.`)
	return replacer.ReplaceAllString(path, "\u202f$1.")
}
