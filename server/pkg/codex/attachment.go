package codex

// Input 是本条要带给 Codex 的正文、文件提及和图片。
type Input struct {
	Text     string   `json:"text,omitempty"`
	Mentions []string `json:"mentions,omitempty"` // 仓库内路径，对 Codex 的 mention。
	Images   []string `json:"images,omitempty"`   // 本地图片路径，对 Codex 的 localImage。
}

// Empty 表示还没有正文或附件。
func (in Input) Empty() bool {
	return in.Text == "" && len(in.Mentions) == 0 && len(in.Images) == 0
}

// UserInput 是发给 turn/start 的一条官方输入项。
type UserInput struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
	URL  string `json:"url,omitempty"`
}

// UserInputs 把本模块的 Input 编成 Codex 的 input 数组。
func UserInputs(in Input) []UserInput {
	items := make([]UserInput, 0, 1+len(in.Mentions)+len(in.Images))
	if in.Text != "" {
		items = append(items, UserInput{Type: "text", Text: in.Text})
	}
	for _, path := range in.Mentions {
		if path == "" {
			continue
		}
		items = append(items, UserInput{Type: "mention", Name: path, Path: path})
	}
	for _, path := range in.Images {
		if path == "" {
			continue
		}
		items = append(items, UserInput{Type: "localImage", Path: path})
	}
	if len(items) == 0 {
		return []UserInput{{Type: "text", Text: ""}}
	}
	return items
}
