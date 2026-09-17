import type {
  ClaudeCommand,
  ClaudeDecision,
  ClaudeInput,
  ClaudeMode,
  ClaudeModel,
  ClaudeSession,
  ClaudeSettings,
  ClaudeSettingsPatch,
  ClaudeStartTurnRequest,
  ClaudeStatus,
  ClaudeTranscript,
} from "./types.ts";

export class ClaudeClientError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ClaudeClientError";
    this.status = status;
  }
}

export type ClaudeClientOptions = {
  baseUrl: string;
  userId: string;
  fetch?: typeof fetch;
};

export class ClaudeClient {
  readonly baseUrl: string;
  readonly userId: string;
  private readonly fetchImpl: typeof fetch;

  // 注入 API 根地址与默认用户，去掉末尾斜杠以免拼路径重复。
  constructor(options: ClaudeClientOptions) {
    this.baseUrl = options.baseUrl.replace(/\/$/, "");
    this.userId = options.userId;
    this.fetchImpl = options.fetch ?? fetch.bind(globalThis);
  }

  // 探测本机 Claude Code 是否可用、是否已授权。
  async probe(): Promise<ClaudeStatus> {
    return this.request<ClaudeStatus>("/claude/status");
  }

  // 列出本机 Claude 模型及各自推理强度。
  async listModels(): Promise<ClaudeModel[]> {
    const body = await this.request<{ models?: ClaudeModel[] }>("/claude/models");
    return body.models ?? [];
  }

  // 列出 Claude 官方权限档。
  async listModes(): Promise<ClaudeMode[]> {
    const body = await this.request<{ modes?: ClaudeMode[] }>("/claude/modes");
    return body.modes ?? [];
  }

  // 列出与 Claude 斜杠同名的命令。
  async listCommands(): Promise<ClaudeCommand[]> {
    const body = await this.request<{ commands?: ClaudeCommand[] }>("/claude/commands");
    return body.commands ?? [];
  }

  // 从本机 Claude 列对话。
  async listSessions(): Promise<ClaudeSession[]> {
    const body = await this.request<{ sessions?: ClaudeSession[] }>("/claude/sessions");
    return body.sessions ?? [];
  }

  // 开一条只走 Claude Code 的空对话。
  async createSession(userId = this.userId): Promise<ClaudeSession> {
    const body = await this.request<{ session: ClaudeSession }>("/claude/sessions", {
      method: "POST",
      json: { user_id: userId },
    });
    return body.session;
  }

  // 读一条对话的标题、绑定编号和进行中回合。
  async getSession(sessionId: string): Promise<ClaudeSession> {
    const body = await this.request<{ session: ClaudeSession }>(`/claude/sessions/${sessionId}`);
    return body.session;
  }

  // 改对话标题。
  async renameSession(sessionId: string, title: string): Promise<void> {
    await this.request<{ ok: boolean }>(`/claude/sessions/${sessionId}`, {
      method: "PATCH",
      json: { title },
    });
  }

  // 归档后不能再向 Claude 开回合。
  async archiveSession(sessionId: string): Promise<void> {
    await this.request<{ ok: boolean }>(`/claude/sessions/${sessionId}/archive`, { method: "POST" });
  }

  // 按官方 --fork-session 复制已落盘实录，换新对话。
  async forkSession(sessionId: string): Promise<ClaudeSession> {
    const body = await this.request<{ session: ClaudeSession }>(`/claude/sessions/${sessionId}/fork`, {
      method: "POST",
    });
    return body.session;
  }

  // 读生效配置（默认与覆盖合并）。
  async getSettings(sessionId: string): Promise<ClaudeSettings> {
    return this.request<ClaudeSettings>(`/claude/sessions/${sessionId}/settings`);
  }

  // 记下用户改过的模型、强度、权限档或目录，下次开回合交给 Claude。
  async applySettings(sessionId: string, patch: ClaudeSettingsPatch): Promise<ClaudeSettings> {
    return this.request<ClaudeSettings>(`/claude/sessions/${sessionId}/settings`, {
      method: "POST",
      json: {
        model: patch.model ?? "",
        effort: patch.effort ?? "",
        permission_mode: patch.permission_mode ?? "",
        cwd: patch.cwd ?? "",
        overridden: patch.overridden ?? [],
      },
    });
  }

  // 执行与斜杠同名的动作，或返回去终端改配置的提示。
  async invoke(sessionId: string, name: string, args = ""): Promise<string> {
    const body = await this.request<{ hint: string }>(`/claude/sessions/${sessionId}/commands`, {
      method: "POST",
      json: { name, args },
    });
    return body.hint ?? "";
  }

  // 把仓库内文件挂到下一条待发内容上。
  async mention(sessionId: string, path: string): Promise<void> {
    await this.request<{ ok: boolean }>(`/claude/sessions/${sessionId}/mentions`, {
      method: "POST",
      json: { path },
    });
  }

  // 把本地图片挂到下一条待发内容上。
  async attachImage(sessionId: string, path: string): Promise<void> {
    await this.request<{ ok: boolean }>(`/claude/sessions/${sessionId}/images`, {
      method: "POST",
      json: { path },
    });
  }

  // 空闲则开新一轮，进行中则排队。
  async startTurn(sessionId: string, req: ClaudeStartTurnRequest): Promise<string> {
    const input = emptyInput(req.input);
    const body = await this.request<{ turn_id: string }>(`/claude/sessions/${sessionId}/turns`, {
      method: "POST",
      json: {
        content: req.content,
        input,
        mode: req.mode ?? "start",
      },
    });
    return body.turn_id;
  }

  // 按本机 Claude 已落下的记录回放实录，并带上官方上下文用量。
  async hydrate(sessionId: string): Promise<ClaudeTranscript> {
    const body = await this.request<ClaudeTranscript>(`/claude/sessions/${sessionId}/transcript`);
    return { items: body.items ?? [], usage: body.usage };
  }

  // 打断当前一轮并传播取消。
  async cancelTurn(turnId: string): Promise<void> {
    await this.request<{ ok: boolean }>(`/claude/turns/${turnId}/cancel`, { method: "POST" });
  }

  // 审批后继续当前一轮。
  async continueTurn(turnId: string): Promise<void> {
    await this.request<{ ok: boolean }>(`/claude/turns/${turnId}/continue`, { method: "POST" });
  }

  // 对已知反问作答。
  async decide(approvalId: string, answer: ClaudeDecision): Promise<void> {
    await this.request<{ ok: boolean }>(`/claude/approvals/${approvalId}/decision`, {
      method: "POST",
      json: {
        approved: answer.approved,
        scope: answer.scope ?? "once",
        choice: answer.choice ?? "",
        values: answer.values ?? [],
      },
    });
  }

  // 回包不兼容的未知问票。
  async rejectUnknown(requestId: string, sessionId: string, turnId: string): Promise<void> {
    await this.request<{ ok: boolean }>(`/claude/asks/${requestId}/reject-unknown`, {
      method: "POST",
      json: { session_id: sessionId, turn_id: turnId },
    });
  }

  // 发 JSON 请求，非 2xx 抽出 error 字段抛 ClaudeClientError。
  private async request<T>(path: string, init: RequestInit & { json?: unknown } = {}): Promise<T> {
    const headers = new Headers(init.headers);
    if (init.json !== undefined) {
      headers.set("Content-Type", "application/json");
    }
    const res = await this.fetchImpl(`${this.baseUrl}${path}`, {
      ...init,
      headers,
      body: init.json !== undefined ? JSON.stringify(init.json) : init.body,
    });
    const text = await res.text();
    let parsed: unknown = undefined;
    if (text) {
      try {
        parsed = JSON.parse(text);
      } catch {
        parsed = { error: text };
      }
    }
    if (!res.ok) {
      const message =
        parsed && typeof parsed === "object" && "error" in parsed
          ? String((parsed as { error: unknown }).error)
          : res.statusText;
      throw new ClaudeClientError(res.status, message);
    }
    return parsed as T;
  }
}

// emptyInput 把缺省提及和图片收成空数组，避免后端收到 null。
function emptyInput(input?: Partial<ClaudeInput>): ClaudeInput {
  return {
    text: input?.text ?? "",
    mentions: input?.mentions ?? [],
    images: input?.images ?? [],
  };
}
