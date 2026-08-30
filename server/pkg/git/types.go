package git

// Repo 是主仓根，不是某一份 worktree。
type Repo struct {
	Path string `json:"path"` // 主仓根路径；worktree 的目录放在 Checkout / Worktree。
}

// Checkout 是当前操作的那份检出（主仓工作区或某个 worktree）。
type Checkout struct {
	Path          string `json:"path"`           // 这份检出的目录。
	CurrentBranch string `json:"current_branch"` // 当前分支短名；detached 或还没有首提交时为空。
	CurrentCommit string `json:"current_commit"` // 当前 HEAD 全文 hash；空仓库为空。
	Detached      bool   `json:"detached"`       // 是否游离 HEAD；空仓库不是 detached，不要和空分支名混用。
}

// Remote 是一条 git remote 地址，不是 origin/main 那种远程跟踪分支。
type Remote struct {
	Name     string `json:"name"`      // remote 名，通常是 origin。
	FetchURL string `json:"fetch_url"` // fetch 用的地址。
	PushURL  string `json:"push_url"`  // push 用的地址；和 fetch 不同时才需要分开展示。
}

// Worktree 是仓库的一份检出。
type Worktree struct {
	Path     string `json:"path"`     // 这份检出的目录。
	Branch   string `json:"branch"`   // 挂着的本地分支；detached 时为空。
	Head     string `json:"head"`     // 这份检出的 HEAD hash。
	Detached bool   `json:"detached"` // 是否游离 HEAD。
	Locked   bool   `json:"locked"`   // 是否被 git worktree lock；锁住时不要删。
}

// FileStatus 是一个路径相对 HEAD / 暂存区的状态。
type FileStatus struct {
	Path           string `json:"path"`            // 仓库内相对路径；重命名后指新路径。
	OrigPath       string `json:"orig_path"`       // 重命名或复制的旧路径；不是重命名则为空。
	StagedStatus   string `json:"staged_status"`   // porcelain XY 第一位：暂存区相对 HEAD；空格表示暂存区没改。
	WorktreeStatus string `json:"worktree_status"` // porcelain XY 第二位：工作区相对暂存区；? 表示未跟踪。
	Unmerged       bool   `json:"unmerged"`        // 未解决冲突；true 时进冲突会话，不要当普通改动。
}

// SiteState 是一份检出的整局：身份、跟踪、文件、remote。不是「只回文件列表」。
type SiteState struct {
	Path          string       `json:"path"`           // 这份检出的绝对路径。
	IsRepo        bool         `json:"is_repo"`        // 当前文件夹是不是 Git 仓库；不是则下面字段都无意义。
	Empty         bool         `json:"empty"`          // 还没有任何提交；不能 reset HEAD~1，也没有图。
	Branch        string       `json:"branch"`         // 当前分支名；detached 时为空。
	Head          string       `json:"head"`           // 当前 HEAD hash；空仓库为空。
	Detached      bool         `json:"detached"`       // 是否游离 HEAD。
	Upstream      string       `json:"upstream"`       // 跟踪的远程分支，如 origin/feat/git；没设置则为空。
	Ahead         int          `json:"ahead"`          // 比 upstream 超前的提交数；用来判断「能否安全撤上次提交」和要不要推。
	Behind        int          `json:"behind"`         // 比 upstream 落后的提交数；用来判断要不要拉。
	UpstreamGone  bool         `json:"upstream_gone"`  // 跟踪目标在远端已删除；推拉前要换跟踪或改 remote。
	Integrating   string       `json:"integrating"`    // 未完成的整合：merge | rebase | cherry_pick | revert；空表示没有。不是 pull。
	DefaultBranch string       `json:"default_branch"` // origin/HEAD 指向的默认分支短名；没有远端则为空。
	Files         []FileStatus `json:"files"`          // 暂存区和工作区的文件（含未跟踪，不含忽略）。
	Remotes       []Remote     `json:"remotes"`        // 已配置的 remote 地址；推送前用来判断有没有 URL。
}

// DiffFile 是已暂存或工作区的一份差异。staged / worktree 由调用时的 scope 区分。
type DiffFile struct {
	Path     string `json:"path"`      // 当前路径。
	OrigPath string `json:"orig_path"` // 重命名来源；不是重命名则为空。
	Kind     string `json:"kind"`      // added | modified | deleted | renamed | unmerged。
	Binary   bool   `json:"binary"`    // 二进制则不要当文本展示，也不要喂给提交说明生成。
	Patch    string `json:"patch"`     // unified diff 文本；二进制或无法生成时为空。
}

// Commit 是一次提交。函数叫 CreateCommit，避免和类型同名。
type Commit struct {
	ID      string   `json:"id"`      // 全文 hash；图的节点 ID、重置目标都用它。
	Parents []string `json:"parents"` // 父提交 hash；首次提交为空，合并提交多于一个，图靠它连边。
	Title   string   `json:"title"`   // 说明第一行。
	Body    string   `json:"body"`    // 第一行之后的正文。
	Author  string   `json:"author"`  // 作者名，给图和时间线展示。
	Date    string   `json:"date"`    // 作者时间，ISO-8601，给图画新旧。
}

// Ref 是落在某次提交上的名字，用来给图上的点上色。
type Ref struct {
	Name string `json:"name"` // 短名，如 feat/git、origin/main、v1.0。
	Kind string `json:"kind"` // local | remote | tag | head。
}

// GraphNode 是近期图上的一个点。
type GraphNode struct {
	Commit Commit `json:"commit"` // 这个点对应的提交。
	Refs   []Ref  `json:"refs"`   // 落在这个提交上的分支 / 标签 / HEAD，用来画分支尖。
}

// GraphEdge 是父子边。
type GraphEdge struct {
	Child  string `json:"child"`  // 子提交 hash，时间上更新的一方。
	Parent string `json:"parent"` // 父提交 hash。
}

// Graph 是近期分叉，不是全仓库考古。点只用于看。
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// Branch 是一条本地分支或远程跟踪分支（refs/remotes 缓存，不是 ls-remote 网上现场）。
type Branch struct {
	Name         string `json:"name"`          // 短名；远程分支带 remote 前缀，如 origin/main。
	Head         string `json:"head"`          // 尖端提交 hash，切过去或从图上认点用。
	IsCurrent    bool   `json:"is_current"`    // 是不是当前检出所在分支。
	IsRemote     bool   `json:"is_remote"`     // 是不是 refs/remotes；列表要和本地分开画。
	Upstream     string `json:"upstream"`      // 这条本地分支跟踪的远程短名；远程分支或无跟踪则为空。
	Ahead        int    `json:"ahead"`         // 比自己的 upstream 超前的提交数；无 upstream 为 0。
	Behind       int    `json:"behind"`        // 比自己的 upstream 落后的提交数。
	UpstreamGone bool   `json:"upstream_gone"` // 跟踪目标在远端已删除。
	WorktreePath string `json:"worktree_path"` // 占用这条分支的检出路径；空表示没被占用。已被占用则不要再 switch 到同一分支。
	Title        string `json:"title"`         // 尖端提交标题，给分支列表预览。
}

// ConflictItem 是未合并文件的种类和三方内容。
type ConflictItem struct {
	Path   string `json:"path"`   // 冲突路径。
	Kind   string `json:"kind"`   // both_modified | deleted_by_us | deleted_by_them | both_added | both_deleted | added_by_us | added_by_them；决定展示三方还是「一侧已删除」。
	Base   string `json:"base"`   // stage 1 共同祖先内容；某侧从一开始就没有该文件时为空。
	Ours   string `json:"ours"`   // stage 2 我方内容；我方删除时为空。
	Theirs string `json:"theirs"` // stage 3 对方内容；对方删除时为空。
	Result string `json:"result"` // 用户写入的决议全文；空表示尚未解决。
}
