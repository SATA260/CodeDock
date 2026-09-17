import type {
  ApprovalAsk,
  AskAnswer,
  CommandResult,
  CommandSpec,
  EngineStatus,
  ModeInfo,
  ModelInfo,
  Session,
  SessionDetail,
  SessionPage,
  Settings,
  StartTurnRequest,
  Turn,
} from "./types.ts";

export class CodexClientError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "CodexClientError";
    this.status = status;
  }
}

export type CodexClientOptions = {
  baseUrl: string;
  fetch?: typeof fetch;
};

export class CodexClient {
  readonly baseUrl: string;
  private readonly fetchImpl: typeof fetch;

  constructor(options: CodexClientOptions) {
    this.baseUrl = options.baseUrl.replace(/\/$/, "");
    this.fetchImpl = options.fetch ?? fetch.bind(globalThis);
  }

  async status(): Promise<EngineStatus> {
    return this.request<EngineStatus>("/codex/status");
  }

  async listModels(): Promise<ModelInfo[]> {
    const body = await this.request<{ models?: ModelInfo[] }>("/codex/models");
    return body.models ?? [];
  }

  async listModes(): Promise<ModeInfo[]> {
    const body = await this.request<{ modes?: ModeInfo[] }>("/codex/modes");
    return body.modes ?? [];
  }

  async listCommands(): Promise<CommandSpec[]> {
    const body = await this.request<{ commands?: CommandSpec[] }>("/codex/commands");
    return body.commands ?? [];
  }

  async listSessions(opts?: { archived?: boolean; cursor?: string }): Promise<SessionPage> {
    const query = new URLSearchParams();
    if (opts?.archived) {
      query.set("archived", "true");
    }
    if (opts?.cursor) {
      query.set("cursor", opts.cursor);
    }
    const suffix = query.toString() ? `?${query}` : "";
    const body = await this.request<SessionPage>(`/codex/sessions${suffix}`);
    return { sessions: body.sessions ?? [], next_cursor: body.next_cursor };
  }

  async createSession(settings: Settings = {}): Promise<Session> {
    const body = await this.request<{ session: Session }>("/codex/sessions", {
      method: "POST",
      json: { settings },
    });
    return body.session;
  }

  async getSession(sessionId: string, signal?: AbortSignal): Promise<SessionDetail> {
    const body = await this.request<{
      session: Session;
      progress?: SessionDetail["progress"];
      asks?: ApprovalAsk[];
    }>(`/codex/sessions/${sessionId}`, { signal });
    return {
      session: body.session,
      progress: body.progress ?? [],
      asks: body.asks ?? [],
    };
  }

  async renameSession(sessionId: string, title: string): Promise<void> {
    await this.request<{ ok: string }>(`/codex/sessions/${sessionId}`, {
      method: "PATCH",
      json: { title },
    });
  }

  async forkSession(sessionId: string): Promise<Session> {
    const body = await this.request<{ session: Session }>(`/codex/sessions/${sessionId}/fork`, {
      method: "POST",
    });
    return body.session;
  }

  async archiveSession(sessionId: string): Promise<void> {
    await this.request<{ ok: string }>(`/codex/sessions/${sessionId}/archive`, { method: "POST" });
  }

  async compactSession(sessionId: string): Promise<void> {
    await this.request<{ ok: string }>(`/codex/sessions/${sessionId}/compact`, { method: "POST" });
  }

  async reviewSession(sessionId: string): Promise<void> {
    await this.request<{ ok: string }>(`/codex/sessions/${sessionId}/review`, { method: "POST" });
  }

  async getSettings(sessionId: string): Promise<Settings> {
    const body = await this.request<{ settings: Settings }>(`/codex/sessions/${sessionId}/settings`);
    return body.settings ?? {};
  }

  async applySettings(sessionId: string, patch: Settings): Promise<Settings> {
    const body = await this.request<{ settings: Settings }>(`/codex/sessions/${sessionId}/settings`, {
      method: "POST",
      json: patch,
    });
    return body.settings ?? {};
  }

  async invokeCommand(sessionId: string, name: string, args = ""): Promise<CommandResult> {
    return this.request<CommandResult>(`/codex/sessions/${sessionId}/commands`, {
      method: "POST",
      json: { name, args },
    });
  }

  async startTurn(sessionId: string, req: StartTurnRequest = {}): Promise<Turn> {
    const body = await this.request<{ turn: Turn }>(`/codex/sessions/${sessionId}/turns`, {
      method: "POST",
      json: {
        content: req.content ?? "",
        input: req.input ?? {},
        mode: req.mode ?? "start",
      },
    });
    return body.turn;
  }

  async mention(sessionId: string, path: string): Promise<void> {
    await this.request<{ ok: string }>(`/codex/sessions/${sessionId}/attachments/mention`, {
      method: "POST",
      json: { path },
    });
  }

  async attachImage(sessionId: string, path: string): Promise<void> {
    await this.request<{ ok: string }>(`/codex/sessions/${sessionId}/attachments/image`, {
      method: "POST",
      json: { path },
    });
  }

  async listAsks(sessionId: string): Promise<ApprovalAsk[]> {
    const body = await this.request<{ asks?: ApprovalAsk[] }>(`/codex/sessions/${sessionId}/asks`);
    return body.asks ?? [];
  }

  async interruptTurn(turnId: string, sessionId: string): Promise<void> {
    await this.request<{ ok: string }>(`/codex/turns/${turnId}/interrupt`, {
      method: "POST",
      json: { session_id: sessionId },
    });
  }

  async decideAsk(requestId: string, answer: AskAnswer): Promise<void> {
    await this.request<{ ok: string }>(`/codex/asks/${requestId}/decision`, {
      method: "POST",
      json: answer,
    });
  }

  async expireAsk(requestId: string): Promise<void> {
    await this.request<{ ok: string }>(`/codex/asks/${requestId}/expire`, { method: "POST" });
  }

  eventsUrl(sessionId: string, after = 0): string {
    return `${this.baseUrl}/codex/sessions/${sessionId}/events?after=${after}`;
  }

  private async request<T>(path: string, init: RequestInit & { json?: unknown } = {}): Promise<T> {
    const headers = new Headers(init.headers);
    if (init.json !== undefined) {
      headers.set("Content-Type", "application/json");
    }
    const { json, ...rest } = init;
    const res = await this.fetchImpl(`${this.baseUrl}${path}`, {
      ...rest,
      headers,
      body: json !== undefined ? JSON.stringify(json) : rest.body,
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
      throw new CodexClientError(res.status, message);
    }
    return parsed as T;
  }
}
