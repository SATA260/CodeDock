import type {
  BoardEngine,
  BoardView,
  Card,
  Checkout,
  CheckoutKind,
  InboxDecideRequest,
  InboxItem,
  IssueSnap,
  Packet,
  Placement,
  PullSnap,
  PutLinkRequest,
  SessionLinks,
  StartWorkSessionRequest,
  Work,
  WorkInfo,
} from "./types.ts";

export class BoardClientError extends Error {
  readonly status: number;

  // 记录看板 HTTP 失败状态。
  constructor(status: number, message: string) {
    super(message);
    this.name = "BoardClientError";
    this.status = status;
  }
}

export type BoardClientOptions = {
  baseUrl: string;
  userId: string;
  tenantId?: string;
  fetch?: typeof fetch;
};

export class BoardClient {
  readonly baseUrl: string;
  readonly userId: string;
  readonly tenantId: string;
  private readonly fetchImpl: typeof fetch;

  // 组装看板 HTTP 客户端；tenant 空则 default。
  constructor(options: BoardClientOptions) {
    this.baseUrl = options.baseUrl.replace(/\/$/, "");
    this.userId = options.userId;
    this.tenantId = options.tenantId?.trim() || "default";
    this.fetchImpl = options.fetch ?? fetch.bind(globalThis);
  }

  // listWorks 列出当前用户的卡。
  async listWorks(): Promise<Work[]> {
    const body = (await this.request(`/works${this.userQuery()}`)) as { works?: Work[] };
    return body.works ?? [];
  }

  // createWork 建一张卡。
  async createWork(title: string): Promise<Work> {
    const body = (await this.request("/works", {
      method: "POST",
      json: { user_id: this.userId, tenant_id: this.tenantId, title },
    })) as { work: Work };
    return body.work;
  }

  // updateWork 改卡片标题。
  async updateWork(workId: string, title: string): Promise<Work> {
    const body = (await this.request(`/works/${workId}`, {
      method: "PATCH",
      json: { title },
    })) as { work: Work };
    return body.work;
  }

  // deleteWork 删卡并断开归属。
  async deleteWork(workId: string): Promise<void> {
    await this.request(`/works/${workId}`, { method: "DELETE" });
  }

  // attachCheckout 把已存在目录挂到卡上。
  async attachCheckout(workId: string, path: string, kind: CheckoutKind = "folder"): Promise<Checkout> {
    const body = (await this.request(`/works/${workId}/checkouts`, {
      method: "POST",
      json: { path, kind },
    })) as { checkout: Checkout };
    return body.checkout;
  }

  // listCheckouts 列出卡上目录。
  async listCheckouts(workId: string): Promise<Checkout[]> {
    const body = (await this.request(`/works/${workId}/checkouts`)) as { checkouts?: Checkout[] };
    return body.checkouts ?? [];
  }

  // detachCheckout 卸下目录。
  async detachCheckout(workId: string, path: string): Promise<void> {
    await this.request(`/works/${workId}/checkouts`, {
      method: "DELETE",
      json: { path },
    });
  }

  // getInfo 读 Work 级或目录级说明。
  async getInfo(workId: string, checkout = ""): Promise<WorkInfo> {
    const query = new URLSearchParams();
    if (checkout) {
      query.set("checkout", checkout);
    }
    const suffix = query.toString() ? `?${query}` : "";
    const body = (await this.request(`/works/${workId}/info${suffix}`)) as { info: WorkInfo };
    return body.info;
  }

  // putInfo 覆盖写入说明。
  async putInfo(workId: string, bodyText: string, checkout = ""): Promise<WorkInfo> {
    const body = (await this.request(`/works/${workId}/info`, {
      method: "PUT",
      json: { checkout, body: bodyText },
    })) as { info: WorkInfo };
    return body.info;
  }

  // startSession 从一张卡开问答或目录会话。
  async startSession(
    workId: string,
    req: StartWorkSessionRequest,
  ): Promise<{ placement: Placement; session_id: string; engine: BoardEngine }> {
    return (await this.request(`/works/${workId}/sessions`, {
      method: "POST",
      json: {
        engine: req.engine,
        kind: req.kind,
        checkout: req.checkout ?? "",
        user_id: this.userId,
        tenant_id: this.tenantId,
        agent_id: req.agent_id,
      },
    })) as { placement: Placement; session_id: string; engine: BoardEngine };
  }

  // attachPlacement 把未归组会话补挂到卡上。
  async attachPlacement(workId: string, engine: BoardEngine, sessionId: string): Promise<Placement> {
    const body = (await this.request(`/works/${workId}/placements`, {
      method: "POST",
      json: { engine, session_id: sessionId },
    })) as { placement: Placement };
    return publicPlacement(body.placement);
  }

  // listPlacements 列出用户所有归属。
  async listPlacements(): Promise<Placement[]> {
    const body = (await this.request(`/placements${this.userQuery()}`)) as { placements?: Placement[] };
    return (body.placements ?? []).map(publicPlacement);
  }

  // getPlacement 读一路会话的归属。
  async getPlacement(engine: BoardEngine, sessionId: string): Promise<Placement> {
    const body = (await this.request(`/placements/${engine}/${sessionId}`)) as { placement: Placement };
    return publicPlacement(body.placement);
  }

  // deletePlacement 断开归属。
  async deletePlacement(engine: BoardEngine, sessionId: string): Promise<void> {
    await this.request(`/placements/${engine}/${sessionId}`, { method: "DELETE" });
  }

  // bindDirectory 把已存在目录绑到会话上。
  async bindDirectory(engine: BoardEngine, sessionId: string, path: string): Promise<string> {
    const body = (await this.request(`/session-directories/${engine}/${sessionId}`, {
      method: "PUT",
      json: { path },
    })) as { path?: string };
    return body.path ?? "";
  }

  // clearDirectory 解绑会话上的目录。
  async clearDirectory(engine: BoardEngine, sessionId: string): Promise<void> {
    await this.request(`/session-directories/${engine}/${sessionId}`, { method: "DELETE" });
  }

  // getBoard 聚合横向看板。
  async getBoard(): Promise<BoardView> {
    const body = (await this.request(`/board${this.userQuery()}`)) as { board?: BoardView };
    return {
      cards: body.board?.cards ?? [],
      ungrouped: body.board?.ungrouped ?? [],
    };
  }

  // getCard 聚合一张卡。
  async getCard(workId: string): Promise<Card> {
    const body = (await this.request(`/board/cards/${workId}`)) as { card: Card };
    return body.card;
  }

  // listInbox 扫该卡下三引擎待批。
  async listInbox(workId: string): Promise<InboxItem[]> {
    const body = (await this.request(`/works/${workId}/inbox`)) as { items?: InboxItem[] };
    return body.items ?? [];
  }

  // decideInbox 按引擎把裁决转给已有审批。
  async decideInbox(req: InboxDecideRequest): Promise<void> {
    await this.request("/inbox/decision", {
      method: "POST",
      json: req,
    });
  }

  // getPacket 读开回合用的只读 Packet。
  async getPacket(engine: BoardEngine, sessionId: string): Promise<Packet> {
    const body = (await this.request(`/placements/${engine}/${sessionId}/packet`)) as { packet: Packet };
    return body.packet;
  }

  // getLinks 读会话头上的 Issue/PR。
  async getLinks(engine: BoardEngine, sessionId: string): Promise<SessionLinks> {
    const body = (await this.request(`/session-links/${engine}/${sessionId}`)) as { links?: SessionLinks };
    return { issue: body.links?.issue ?? null, pulls: body.links?.pulls ?? [] };
  }

  // replaceLinks 用一批链接收掉会话上的 Issue/PR，种类由链接路径决定。
  async replaceLinks(engine: BoardEngine, sessionId: string, links: string[]): Promise<SessionLinks> {
    const body = (await this.request(`/session-links/${engine}/${sessionId}`, {
      method: "PUT",
      json: { links },
    })) as { links?: SessionLinks };
    return { issue: body.links?.issue ?? null, pulls: body.links?.pulls ?? [] };
  }

  // putIssue 用 gh 拉 Issue 快照并挂到会话头。
  async putIssue(engine: BoardEngine, sessionId: string, req: PutLinkRequest): Promise<IssueSnap> {
    const body = (await this.request(`/session-links/${engine}/${sessionId}/issue`, {
      method: "PUT",
      json: req,
    })) as { issue: IssueSnap };
    return body.issue;
  }

  // deleteIssue 去掉会话上的 Issue。
  async deleteIssue(engine: BoardEngine, sessionId: string): Promise<void> {
    await this.request(`/session-links/${engine}/${sessionId}/issue`, { method: "DELETE" });
  }

  // putPull 用 gh 拉 PR 快照并挂到会话头。
  async putPull(engine: BoardEngine, sessionId: string, req: PutLinkRequest): Promise<PullSnap> {
    const body = (await this.request(`/session-links/${engine}/${sessionId}/pulls`, {
      method: "POST",
      json: req,
    })) as { pull: PullSnap };
    return body.pull;
  }

  // deletePull 去掉会话上的一条 PR。
  async deletePull(engine: BoardEngine, sessionId: string, number: number, repo = ""): Promise<void> {
    const query = repo ? `?repo=${encodeURIComponent(repo)}` : "";
    await this.request(`/session-links/${engine}/${sessionId}/pulls/${number}${query}`, { method: "DELETE" });
  }

  // userQuery 带上当前用户，给列表接口用。
  private userQuery(): string {
    const search = new URLSearchParams({ user_id: this.userId, tenant_id: this.tenantId });
    return `?${search.toString()}`;
  }

  // request 发 JSON 请求，失败时抛 BoardClientError。
  private async request(path: string, init: RequestInit & { json?: unknown } = {}): Promise<unknown> {
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
      throw new BoardClientError(res.status, message);
    }
    return parsed;
  }
}

// publicPlacement 把库内 native 收成前端 agent。
function publicPlacement(place: Placement): Placement {
  return {
    ...place,
    engine: place.engine === "native" ? "agent" : place.engine,
  };
}
