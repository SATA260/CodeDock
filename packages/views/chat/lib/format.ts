export function shortId(id: string): string {
  return id.replace(/-/g, "").slice(0, 8);
}

export function relativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) {
    return "";
  }
  const delta = Date.now() - then;
  const minutes = Math.floor(delta / 60_000);
  if (minutes < 1) {
    return "刚刚";
  }
  if (minutes < 60) {
    return `${minutes} 分钟前`;
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 24) {
    return `${hours} 小时前`;
  }
  const days = Math.floor(hours / 24);
  if (days < 7) {
    return `${days} 天前`;
  }
  return new Date(iso).toLocaleDateString();
}

// sessionTitle 侧栏标题：正文中间可省略，末尾 (n) 必须留下。
export function sessionTitle(id: string, preview?: string): string {
  const { stem, suffix } = sessionTitleParts(id, preview);
  return `${stem}${suffix}`;
}

// sessionTitleParts 拆开可裁的正文和必须露出来的序号，给侧栏两段排。
export function sessionTitleParts(id: string, preview?: string): { stem: string; suffix: string } {
  const raw = preview?.trim() ? clipTitleKeepIndex(preview, 36) : `会话 ${shortId(id)}`;
  return splitForkSuffix(raw);
}

// clipTitleKeepIndex 取首行；超长时省略中间，保留末尾 (n)。
function clipTitleKeepIndex(text: string, max: number): string {
  const line = text.split("\n").find((part) => part.trim()) ?? text;
  const trimmed = line.trim();
  const { stem, suffix } = splitForkSuffix(trimmed);
  if ([...trimmed].length <= max) {
    return trimmed;
  }
  const budget = Math.max(8, max - [...suffix].length);
  return `${clipMiddle(stem, budget)}${suffix}`;
}

// splitForkSuffix 认出官方 fork 序号，如「 (1)」。
function splitForkSuffix(title: string): { stem: string; suffix: string } {
  const match = title.match(/ \((\d+)\)$/);
  if (!match || match.index == null) {
    return { stem: title, suffix: "" };
  }
  return { stem: title.slice(0, match.index), suffix: title.slice(match.index) };
}

// clipMiddle 超长正文留头尾，中间用省略号。
function clipMiddle(text: string, max: number): string {
  const runes = [...text];
  if (runes.length <= max) {
    return text.trim();
  }
  const keep = Math.max(1, max - 1);
  const head = Math.max(1, Math.ceil(keep * 0.55));
  const tail = Math.max(0, keep - head);
  const start = runes.slice(0, head).join("").trimEnd();
  const end = tail > 0 ? runes.slice(-tail).join("").trimStart() : "";
  return end ? `${start}…${end}` : `${start}…`;
}

export function shortWorkspace(path: string, max = 42): string {
  const trimmed = path.trim();
  if (!trimmed || trimmed.length <= max) {
    return trimmed;
  }
  const parts = trimmed.split(/[/\\]/).filter(Boolean);
  if (parts.length >= 2) {
    return `…/${parts.slice(-2).join("/")}`;
  }
  return `…${trimmed.slice(-(max - 1))}`;
}
