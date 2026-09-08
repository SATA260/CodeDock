package claude

// Mention 把仓库内文件挂到待发给 Claude Code 的内容上。
func Mention(sessionID, path string) error {
	if sessionID == "" {
		return wrapErr(errInvalid, "session_id is required")
	}
	if path == "" {
		return wrapErr(errInvalid, "path is required")
	}
	if _, err := Get(sessionID); err != nil {
		return err
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	sess := internLocked(sessionID)
	sess.Draft.Mentions = append(sess.Draft.Mentions, path)
	return nil
}

// AttachImage 把本地图片挂到待发给 Claude Code 的内容上。
func AttachImage(sessionID, path string) error {
	if sessionID == "" {
		return wrapErr(errInvalid, "session_id is required")
	}
	if path == "" {
		return wrapErr(errInvalid, "path is required")
	}
	if _, err := Get(sessionID); err != nil {
		return err
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	sess := internLocked(sessionID)
	sess.Draft.Images = append(sess.Draft.Images, path)
	return nil
}

// TakeDraft 取出本条附件草稿并清空。
func TakeDraft(sessionID string) (Input, error) {
	if sessionID == "" {
		return Input{Mentions: emptyStrings(), Images: emptyStrings()}, wrapErr(errInvalid, "session_id is required")
	}
	if _, err := Get(sessionID); err != nil {
		return Input{Mentions: emptyStrings(), Images: emptyStrings()}, err
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	sess := internLocked(sessionID)
	draft := sess.Draft
	sess.Draft = Input{Mentions: emptyStrings(), Images: emptyStrings()}
	if draft.Mentions == nil {
		draft.Mentions = emptyStrings()
	}
	if draft.Images == nil {
		draft.Images = emptyStrings()
	}
	return draft, nil
}

func mergeInput(content string, input, draft Input) Input {
	out := input
	if out.Text == "" {
		out.Text = content
	}
	out.Mentions = append(append([]string{}, draft.Mentions...), input.Mentions...)
	out.Images = append(append([]string{}, draft.Images...), input.Images...)
	if out.Mentions == nil {
		out.Mentions = emptyStrings()
	}
	if out.Images == nil {
		out.Images = emptyStrings()
	}
	return out
}
