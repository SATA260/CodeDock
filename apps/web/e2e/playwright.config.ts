import { defineConfig } from "@playwright/test";
import path from "node:path";

const root = path.resolve(__dirname, "../../..");
const api = process.env.E2E_API_BASE ?? "http://127.0.0.1:18080";
const web = process.env.E2E_WEB_BASE ?? "http://localhost:3100";

export default defineConfig({
  testDir: __dirname,
  timeout: 240_000,
  expect: { timeout: 30_000 },
  fullyParallel: false,
  workers: 1,
  reporter: "list",
  use: {
    baseURL: web,
    viewport: { width: 1400, height: 900 },
    trace: "retain-on-failure",
  },
  webServer: [
    {
      command: "sh scripts/e2e-api.sh",
      cwd: root,
      url: `${api}/health`,
      timeout: 120_000,
      reuseExistingServer: false,
    },
    {
      command: "pnpm --filter web exec next dev -p 3100",
      cwd: root,
      url: web,
      timeout: 120_000,
      reuseExistingServer: false,
      env: {
        ...process.env,
        NEXT_PUBLIC_API_BASE: api,
        NEXT_PUBLIC_USER_ID: "local",
      },
    },
  ],
});
