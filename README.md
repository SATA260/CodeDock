# CodeDock

面向 Agent 的执行与协作平台骨架：提供会话、执行请求、过程事件和结果回报所需的服务端与 Web 端边界。不绑定 Issue、Task、Review、Workspace 等具体业务域。

改代码前请阅读 [`docs/architecture.md`](docs/architecture.md) 和 [`AGENTS.md`](AGENTS.md)。

## 本地运行

需要 [Go](https://go.dev/dl/) 1.26.5、[Node.js](https://nodejs.org/) 22.13+、[pnpm](https://pnpm.io/) 11.17.0。默认 `LLM_PROVIDER=fake`，不必配置模型 Key。

```bash
cp .env.example .env
cp apps/web/.env.example apps/web/.env.local
pnpm install
```

一次起 API + Web。若已有 `tmp/git-sandbox`，API 默认指到沙箱，避免在本仓上试撤回：

```bash
pnpm dev
```

也可以分开起。API（默认 `http://localhost:8080`）：

```bash
pnpm dev:api
```

从 `server/` 直接 `go run` 时，未设 `GIT_REPO` 会用进程 cwd（`server/` 不是仓根）。服务会从当前目录向上查找 `.env`。

Web（默认 `http://localhost:3000`）：

```bash
pnpm dev:web
```

浏览器打开 [http://localhost:3000](http://localhost:3000)。开发态顶栏可在对话和仓库之间切换。完整环境变量见 [`.env.example`](.env.example) 和 [`apps/web/.env.example`](apps/web/.env.example)，不要提交 `.env` 或密钥。

## 测试

```bash
cd server && go test ./... && go vet ./...
pnpm test:client
pnpm lint:web
```

Go 源文件用 `gofmt` 格式化（`gofmt -w .`）。CI 会检查未格式化的文件。

## 参与

从 `main` 开分支，按架构文档放置代码。本地测试通过后，向 `main` 开 Pull Request，说明改了什么、怎么验证。一种改动一个 PR；较大的行为或边界变化请先开 Issue。

## License

[MIT](LICENSE)
