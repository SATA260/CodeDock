package agent

// CountTokens 按 UTF-8 字节估算 token 数（约 4 字节 1 token）。
func CountTokens(text string) int64 {
	if text == "" {
		return 0
	}
	n := int64(len(text) / 4)
	if n == 0 {
		return 1
	}
	return n
}
