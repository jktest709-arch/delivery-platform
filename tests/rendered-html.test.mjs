import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const developmentPreviewMeta =
  /<meta(?=[^>]*\bname=["']codex-preview["'])(?=[^>]*\bcontent=["']development["'])[^>]*>/i;

async function render() {
  const workerUrl = new URL("../dist/server/index.js", import.meta.url);
  workerUrl.searchParams.set("test", `${process.pid}-${Date.now()}`);
  const { default: worker } = await import(workerUrl.href);

  return worker.fetch(
    new Request("http://localhost/", {
      headers: { accept: "text/html" },
    }),
    {
      ASSETS: {
        fetch: async () => new Response("Not found", { status: 404 }),
      },
    },
    {
      waitUntil() {},
      passThroughOnException() {},
    },
  );
}

test("server-renders the delivery platform shell", async () => {
  const response = await render();
  assert.equal(response.status, 200);
  assert.match(response.headers.get("content-type") ?? "", /^text\/html\b/i);

  const html = await response.text();
  assert.doesNotMatch(html, developmentPreviewMeta);
  assert.match(html, /<title>统一交付平台<\/title>/i);
  assert.match(html, /GitLab CI 统一发布入口/);
  assert.match(html, /上线单申请/);
  assert.match(html, /提交上线申请/);
  assert.match(html, /打包部署执行台/);
  assert.match(html, /一键打包全部/);
  assert.match(html, /后端一键打包/);
  assert.match(html, /前端一键打包/);
  assert.match(html, /选择项目、分支或 Commit/);
  assert.match(html, /GitLab、Tag 与依赖/);
  assert.match(html, /GitLab 仓库配置/);
  assert.match(html, /https:\/\/gitlab\.corp\/delivery\/order-core/);
  assert.match(html, /GitLab Project ID/);
  assert.match(html, /项目打包依赖管理/);
  assert.match(html, /订单核心服务/);
  assert.match(html, /固定顺序:\s*(?:<!-- -->)?20/);
  assert.match(html, /aaprd-20260725-042/);
});

test("removes starter preview wiring", async () => {
  const [page, layout, packageJson, css] = await Promise.all([
    readFile(new URL("../app/page.tsx", import.meta.url), "utf8"),
    readFile(new URL("../app/layout.tsx", import.meta.url), "utf8"),
    readFile(new URL("../package.json", import.meta.url), "utf8"),
    readFile(new URL("../app/globals.css", import.meta.url), "utf8"),
  ]);

  assert.doesNotMatch(page, /_sites-preview|SkeletonPreview|codex-preview/);
  assert.doesNotMatch(layout, /Starter Project|next\/font\/google/);
  assert.doesNotMatch(packageJson, /react-loading-skeleton/);
  assert.match(css, /delivery-shell/);
  assert.match(css, /pipeline-strip/);
  assert.match(css, /package-lanes/);
  assert.match(css, /dependency-configs/);
  assert.match(css, /application-toolbar/);
  assert.match(css, /execution-note/);
  assert.match(css, /repo-configs/);
  assert.match(css, /repo-fields/);
});
