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
    businessLineCodes: ["ops", "aa"],
    businessLines: [
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
    ],
    gitlabUrl: "https://gitlab.corp/delivery/base-auth",
    gitlabProjectId: "delivery/base-auth",
    defaultBranch: "master",
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
    businessLineCodes: ["aa"],
    businessLines: [
      {
        id: 2,
        code: "aa",
        name: "AA 零售业务线",
        platform: "AAPRD",
        tagPrefix: "aaprd",
        tagTemplate: "{prefix}-{date}-{releaseNo}",
        approver: "交易发布经理",
      },
    ],
    gitlabUrl: "https://gitlab.corp/delivery/order-core",
    gitlabProjectId: "delivery/order-core",
    defaultBranch: "master",
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

const usersPayload = [
  {
    id: 1,
    username: "admin",
    displayName: "高远",
    role: "admin",
    status: "enabled",
  },
  {
    id: 2,
    username: "release",
    displayName: "发布经理",
    role: "release_manager",
    status: "enabled",
  },
];

const releasesPayload = [
  {
    id: 1,
    batchNo: "PRD-20260729-001",
    applicant: {
      id: 1,
      username: "admin",
      displayName: "高远",
      role: "admin",
      status: "enabled",
    },
    businessLineId: 1,
    businessLine: businessLinesPayload[0],
    status: "pending",
    releaseWindow: "2026-07-29T10:00:00Z",
    remark: "回归测试",
    projects: [
      {
        id: 1,
        releaseId: 1,
        projectId: 1,
        project: projectsPayload[0],
        businessLineId: 1,
        businessLine: projectsPayload[0].businessLine,
        sourceType: "branch",
        sourceRef: "master",
        sourceDirty: false,
        targetTag: "opsprd-20260729100000-001",
        pipelineId: "",
        buildJobId: "",
        deployJobId: "",
        jobs: [
          {
            id: 1,
            releaseProjectId: 1,
            gitlabJobId: "201",
            name: "build-image",
            stage: "build",
            status: "manual",
            webUrl: "https://gitlab.corp/delivery/base-auth/-/jobs/201",
            manual: true,
            allowFailure: false,
          },
          {
            id: 2,
            releaseProjectId: 1,
            gitlabJobId: "301",
            name: "deploy-prod",
            stage: "deploy",
            status: "manual",
            webUrl: "https://gitlab.corp/delivery/base-auth/-/jobs/301",
            manual: true,
            allowFailure: false,
          },
        ],
        status: "pending",
        errorMessage: "",
        sortOrder: 10,
      },
    ],
    events: [],
    createdAt: "2026-07-29T09:00:00Z",
    updatedAt: "2026-07-29T09:00:00Z",
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
    await route.fulfill({ json: releasesPayload });
  });
  await page.route("**/api/users", async (route) => {
    await route.fulfill({ json: usersPayload });
  });

  await page.goto("/");
  await page.getByRole("button", { name: "登录" }).click();

  await expect(page.getByRole("heading", { name: "上线单申请" })).toBeVisible();
  await expect(page.getByText("统一认证中心")).toBeVisible();

  for (const tab of ["构建执行台", "项目配置", "Tag 与依赖", "发布历史", "用户权限", "上线单申请"]) {
    await page.getByRole("button", { name: tab }).click();
    await expect(page.getByRole("heading", { level: 1, name: tab })).toBeVisible();
  }

  await page.getByRole("button", { name: "构建执行台" }).click();
  await expect(page.getByRole("button", { name: "删除任务" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Pipeline 流程" })).toBeVisible();
  await expect(page.getByText("GitLab Tags API")).toBeVisible();
  await expect(page.getByRole("button", { name: "全量打 Tag 构建" })).toBeVisible();
  await expect(page.getByRole("button", { name: "后端打 Tag 构建" })).toBeVisible();
  await expect(page.getByRole("button", { name: "前端打 Tag 构建" })).toBeVisible();
  await expect(page.getByRole("button", { name: "全量重打新 Tag 构建" })).toBeVisible();
  await expect(page.getByText("build-image")).toBeVisible();
  await expect(page.getByRole("button", { name: "编辑来源" })).toBeVisible();
  await expect(page.getByRole("button", { name: "重试原 Pipeline" })).toBeVisible();
  await expect(page.getByRole("button", { name: "最新来源重打 Tag" })).toBeVisible();
  await expect(page.getByText("GitLab").first()).toBeVisible();

  await page.getByRole("button", { name: "项目配置" }).click();
  await expect(page.getByRole("button", { name: "新增项目" })).toBeVisible();
  await expect(page.getByRole("button", { name: "编辑" }).first()).toBeVisible();
  await expect(page.getByRole("button", { name: "删除" }).first()).toBeVisible();
  await page.getByRole("button", { name: "新增项目" }).click();
  await expect(page.getByText("默认业务线")).toBeVisible();

  await page.getByRole("button", { name: "Tag 与依赖" }).click();
  await expect(page.getByRole("button", { name: "新增业务线" })).toBeVisible();
  await expect(page.getByText("关联项目 / 迁移到")).toHaveCount(0);
  await page.locator("tr").filter({ hasText: "OPS 平台业务线" }).getByRole("button", { name: "删除" }).click();
  await expect(page.getByText("迁移后删除")).toBeVisible();
  await expect(page.getByText("迁移到")).toBeVisible();
  await expect(page.getByRole("heading", { name: "项目依赖顺序" })).toBeVisible();
  await expect(page.getByRole("button", { name: "新增依赖关系" })).toBeVisible();
  await expect(page.getByRole("button", { name: "上移" }).first()).toBeVisible();
  await expect(page.getByRole("button", { name: "下移" }).first()).toBeVisible();
  await expect(page.getByRole("button", { name: "删除整行" })).toBeVisible();

  await page.getByRole("button", { name: "发布历史" }).click();
  await expect(page.getByRole("heading", { name: "PRD-20260729-001" })).toBeVisible();
  await expect(page.getByRole("button", { name: "删除任务" })).toBeVisible();

  await page.getByRole("button", { name: "用户权限" }).click();
  await expect(page.getByRole("button", { name: "新增用户" })).toBeVisible();
  await expect(page.getByText("发布经理").first()).toBeVisible();

  expect(runtimeErrors).toEqual([]);
});
