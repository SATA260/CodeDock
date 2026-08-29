import type {
  AgentMode,
  Approval,
  CreateSessionRequest,
  DecideApprovalRequest,
  Message,
  PageInfo,
  Session,
  StartRunRequest,
  StartRunResponse,
} from "./types.ts";

export class AgentClientError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "AgentClientError";
    this.status = status;
  }
}

export interface AgentClientOptions {
  baseUrl: string;
  fetch?: typeof fetch;
}

export class AgentClient {
  readonly baseUrl: string;
  private readonly fetchImpl: typeof fetch;

  constructor(options: AgentClientOptions) {
    this.baseUrl = options.baseUrl.replace(/\/$/, "");
    this.fetchImpl = options.fetch ?? fetch.bind(globalThis);
  }

  async createSession(req: CreateSessionRequest): Promise<Session> {
    const body = await this.request<{ session: Session }>("/sessions", {
      method: "POST",
      json: {
        tenant_id: req.tenant_id ?? "default",
        user_id: req.user_id,
        agent_id: req.agent_id ?? "default",
        workspace_id: req.workspace_id ?? "default",
      },
    });
    return body.session;
  }

  async listSessions(page = 1, pageSize = 50): Promise<{ sessions: Session[]; page: PageInfo }> {
    const query = new URLSearchParams({
      page: String(page),
      page_size: String(pageSize),
      sort_by: "updated_at",
      sort_order: "desc",
    });
    const body = await this.request<{ sessions: Session[] } & PageInfo>(`/sessions?${query}`);
    return {
      sessions: body.sessions ?? [],
      page: {
        page: body.page,
        page_size: body.page_size,
        sort_by: body.sort_by,
        sort_order: body.sort_order,
        total: body.total,
      },
    };
  }

  async getSession(sessionId: string): Promise<Session> {
    const body = await this.request<{ session: Session }>(`/sessions/${sessionId}`);
    return body.session;
  }

  async listMessages(sessionId: string): Promise<Message[]> {
    const messages: Message[] = [];
    let page = 1;
    let total = Number.POSITIVE_INFINITY;
    while (messages.length < total) {
      const query = new URLSearchParams({
        page: String(page),
        page_size: "100",
        sort_by: "event_seq",
        sort_order: "asc",
      });
      const body = await this.request<{ messages: Message[] } & PageInfo>(
        `/sessions/${sessionId}/messages?${query}`,
      );
      messages.push(...(body.messages ?? []));
      total = body.total ?? messages.length;
      if ((body.messages ?? []).length === 0) {
        break;
      }
      page += 1;
    }
    return messages;
  }

  async startRun(sessionId: string, req: StartRunRequest): Promise<StartRunResponse> {
    return this.request<StartRunResponse>(`/sessions/${sessionId}/runs`, {
      method: "POST",
      json: {
        content: req.content,
        input_mode: req.input_mode ?? "queue",
        mode: req.mode ?? ("ask_for_approval" satisfies AgentMode),
      },
    });
  }

  async cancelRun(runId: string): Promise<void> {
    await this.request<{ ok: boolean }>(`/runs/${runId}/cancel`, { method: "POST" });
  }

  async decideApproval(approvalId: string, req: DecideApprovalRequest): Promise<Approval> {
    const body = await this.request<{ approval: Approval }>(`/approvals/${approvalId}/decision`, {
      method: "POST",
      json: {
        decisions: req.decisions,
        scope: req.scope ?? "once",
        actor_id: req.actor_id ?? "",
        reason: req.reason ?? "",
      },
    });
    return body.approval;
  }

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
      throw new AgentClientError(res.status, message);
    }
    return parsed as T;
  }
}
