import { expect, test } from "@playwright/test";

const projectsPayload = [
  {
    id: 1,
    code: "base-auth",
    name: "统一认证中心",
    kind: "backend",
    owner: "平台组",
    businessLineCode: "ops",
    businessLine: {
      id: 1,
      code: "ops",
      name: "OPS 平台业务线",
      platform: "OPSPRD",
      tagPrefix: "opsprd",
      tagTemplate: "{prefix}-{date}-{releaseNo}",
      approver: "平台 SRE",
    },
    gitlabUrl: "https://gitlab.corp/delivery/base-auth",
    gitlabProjectId: "delivery/base-auth",
    defaultBranch: "master",
    packageJob: "build-auth-prd",
    deployJob: "deploy-auth-prd",
    sortOrder: 10,
    enabled: true,
    dependencies: null,
  },
  {
    id: 2,
    code: "order-core",
    name: "订单核心服务",
    kind: "backend",
    owner: "交易组",
    businessLineCode: "aa",
    businessLine: {
      id: 2,
      code: "aa",
      name: "AA 零售业务线",
      platform: "AAPRD",
      tagPrefix: "aaprd",
      tagTemplate: "{prefix}-{date}-{releaseNo}",
      approver: "交易发布经理",
    },
    gitlabUrl: "https://gitlab.corp/delivery/order-core",
    gitlabProjectId: "delivery/order-core",
    defaultBranch: "master",
    packageJob: "build-order-prd",
    deployJob: "deploy-order-prd",
    sortOrder: 20,
    enabled: true,
    dependencies: ["base-auth"],
  },
];

const businessLinesPayload = [
  {
    id: 1,
    code: "ops",
    name: "OPS 平台业务线",
    platform: "OPSPRD",
    tagPrefix: "opsprd",
    tagTemplate: "{prefix}-{date}-{releaseNo}",
    approver: "平台 SRE",
  },
  {
    id: 2,
    code: "aa",
    name: "AA 零售业务线",
    platform: "AAPRD",
    tagPrefix: "aaprd",
    tagTemplate: "{prefix}-{date}-{releaseNo}",
    approver: "交易发布经理",
  },
];

test("admin can log in and visit every main page without runtime errors", async ({ page }) => {
  const runtimeErrors: string[] = [];
  page.on("pageerror", (error) => runtimeErrors.push(error.message));
  page.on("console", (message) => {
    if (message.type() === "error") {
      runtimeErrors.push(message.text());
    }
  });

  await page.route("**/api/auth/login", async (route) => {
    await route.fulfill({
      json: {
        token: "test-token",
        user: {
          id: 1,
          username: "admin",
          displayName: "高远",
          role: "admin",
          status: "enabled",
        },
      },
    });
  });
  await page.route("**/api/projects", async (route) => {
    await route.fulfill({ json: projectsPayload });
  });
  await page.route("**/api/business-lines", async (route) => {
    await route.fulfill({ json: businessLinesPayload });
  });
  await page.route("**/api/releases", async (route) => {
    await route.fulfill({ json: [] });
  });

  await page.goto("/");
  await page.getByRole("button", { name: "登录" }).click();

  await expect(page.getByRole("heading", { name: "上线单申请" })).toBeVisible();
  await expect(page.getByText("统一认证中心")).toBeVisible();

  for (const tab of ["构建执行台", "项目配置", "Tag 与依赖", "发布历史", "上线单申请"]) {
    await page.getByRole("button", { name: tab }).click();
    await expect(page.getByRole("heading", { level: 1, name: tab })).toBeVisible();
  }

  await page.getByRole("button", { name: "项目配置" }).click();
  await expect(page.getByRole("button", { name: "新增项目" })).toBeVisible();
  await expect(page.getByRole("button", { name: "编辑" }).first()).toBeVisible();
  await expect(page.getByRole("button", { name: "删除" }).first()).toBeVisible();

  await page.getByRole("button", { name: "Tag 与依赖" }).click();
  await expect(page.getByRole("button", { name: "新增业务线" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "项目依赖顺序" })).toBeVisible();

  expect(runtimeErrors).toEqual([]);
});
