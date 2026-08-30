import type { BranchView, Commit, CommitRequest, DiffFile, DiffScope, SiteState } from "./types.ts";

export class GitClientError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "GitClientError";
    this.status = status;
  }
}

export type GitClientOptions = {
  baseUrl: string;
  fetch?: typeof fetch;
};

export class GitClient {
  readonly baseUrl: string;
  private readonly fetchImpl: typeof fetch;

  constructor(options: GitClientOptions) {
    this.baseUrl = options.baseUrl.replace(/\/$/, "");
    this.fetchImpl = options.fetch ?? fetch.bind(globalThis);
  }

  async status(checkout?: string): Promise<SiteState> {
    return this.request<SiteState>(`/git/status${query({ checkout })}`);
  }

  async stage(paths: string[], checkout?: string): Promise<void> {
    await this.request<{ ok: boolean }>("/git/stage", {
      method: "POST",
      json: { paths, checkout: checkout ?? "" },
    });
  }

  async unstage(paths: string[], checkout?: string): Promise<void> {
    await this.request<{ ok: boolean }>("/git/unstage", {
      method: "POST",
      json: { paths, checkout: checkout ?? "" },
    });
  }

  async discard(paths: string[], checkout?: string): Promise<void> {
    await this.request<{ ok: boolean }>("/git/discard", {
      method: "POST",
      json: { paths, checkout: checkout ?? "" },
    });
  }

  async commit(req: CommitRequest): Promise<Commit> {
    const body = await this.request<{ commit: Commit }>("/git/commit", {
      method: "POST",
      json: {
        message: req.message,
        paths: req.paths ?? [],
        checkout: req.checkout ?? "",
      },
    });
    return body.commit;
  }

  async push(checkout?: string): Promise<void> {
    await this.request<{ ok: boolean }>("/git/push", {
      method: "POST",
      json: { checkout: checkout ?? "" },
    });
  }

  async listBranches(checkout?: string): Promise<BranchView> {
    return this.request<BranchView>(`/git/branches${query({ checkout })}`);
  }

  async createBranch(name: string, start?: string, checkout?: string): Promise<void> {
    await this.request<{ ok: boolean }>("/git/branches", {
      method: "POST",
      json: { name, start: start ?? "", checkout: checkout ?? "" },
    });
  }

  async switchBranch(name: string, checkout?: string): Promise<void> {
    await this.request<{ ok: boolean }>("/git/branches/switch", {
      method: "POST",
      json: { name, checkout: checkout ?? "" },
    });
  }

  async diff(scope: DiffScope = "staged", checkout?: string): Promise<DiffFile[]> {
    const body = await this.request<{ files?: DiffFile[] }>(`/git/diff${query({ scope, checkout })}`);
    return body.files ?? [];
  }

  async log(limit = 50, checkout?: string): Promise<Commit[]> {
    const body = await this.request<{ commits?: Commit[] }>(
      `/git/log${query({ limit: String(limit), checkout })}`,
    );
    return body.commits ?? [];
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
      throw new GitClientError(res.status, message);
    }
    return parsed as T;
  }
}

function query(params: Record<string, string | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value) {
      search.set(key, value);
    }
  }
  const encoded = search.toString();
  return encoded ? `?${encoded}` : "";
}
