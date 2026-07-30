import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App.vue";

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

const releaseChangesPayload = [
  {
    id: 1,
    releaseId: 1,
    type: "db",
    title: "订单表结构调整",
    status: "pending",
    riskLevel: "high",
    contentJson: JSON.stringify({
      datasource: "prod-order",
      defaultDatabase: "order_db",
      sqlText: "CREATE TABLE order_db.order_log (id bigint);",
      normalizedSql: "USE order_db;\n\nCREATE TABLE order_db.order_log (id bigint);",
      rollbackSql: "DROP TABLE order_db.order_log;",
      warnings: ["检测到 order_db.table 写法，执行预览已自动补充 USE order_db;"],
    }),
    createdById: 1,
    createdBy: usersPayload[0],
    createdAt: "2026-07-29T09:00:00Z",
    updatedAt: "2026-07-29T09:00:00Z",
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
    changes: releaseChangesPayload,
    events: [],
    createdAt: "2026-07-29T09:00:00Z",
    updatedAt: "2026-07-29T09:00:00Z",
  },
];

describe("App", () => {
  let consoleError: ReturnType<typeof vi.spyOn> | undefined;

  beforeEach(() => {
    vi.stubGlobal("localStorage", createMemoryStorage());
    consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    vi.stubGlobal("fetch", vi.fn(mockFetch));
    vi.stubGlobal("confirm", vi.fn(() => true));
  });

  afterEach(() => {
    consoleError?.mockRestore();
    vi.unstubAllGlobals();
  });

  it("logs in and renders every view when dependencies are null", async () => {
    const wrapper = mount(App);

    await wrapper.find("form").trigger("submit");
    await flushPromises();
    await wrapper.vm.$nextTick();

    expect(wrapper.text()).toContain("统一认证中心");
    expect(wrapper.text()).toContain("上线单申请");
    expect(wrapper.text()).toContain("发布业务线");
    expect(wrapper.text()).toContain("从历史上线单复制");
    expect(wrapper.text()).toContain("上线单预览");
    expect(wrapper.text()).toContain("变更事项");
    expect(wrapper.text()).toContain("变更事项预览");
    expect(wrapper.text()).toContain("复制到当前申请");

    await wrapper.findAll("button").find((item) => item.text() === "新增 DB")!.trigger("click");
    await wrapper.vm.$nextTick();
    const sqlTextarea = wrapper.findAll("textarea").find((item) => item.attributes("placeholder")?.includes("CREATE TABLE"));
    expect(sqlTextarea, "missing DB SQL textarea").toBeTruthy();
    await sqlTextarea!.setValue("CREATE TABLE pay_db.pay_log (id bigint);");
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).toContain("USE pay_db;");
    expect(wrapper.text()).toContain("检测到 pay_db.table 写法");
    await wrapper.findAll("button").find((item) => item.text() === "删除")!.trigger("click");
    await wrapper.vm.$nextTick();

    const historySelect = wrapper.findAll("select").find((item) => item.text().includes("PRD-20260729-001"));
    expect(historySelect, "missing history release import selector").toBeTruthy();
    await historySelect!.setValue("1");
    await wrapper.findAll("button").find((item) => item.text() === "复制到当前申请")!.trigger("click");
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).toContain("已从 PRD-20260729-001 复制 1 个项目、1 个变更事项到申请草稿");
    expect(wrapper.text()).toContain("订单表结构调整");

    for (const tab of ["构建执行台", "项目配置", "Tag 与依赖", "发布历史", "上线单申请"]) {
      const button = wrapper.findAll("button").find((item) => item.text() === tab);
      expect(button, `missing sidebar tab ${tab}`).toBeTruthy();
      await button!.trigger("click");
      await wrapper.vm.$nextTick();
      expect(wrapper.text()).toContain(tab);
      if (tab === "构建执行台") {
        expect(wrapper.text()).not.toContain("已从 PRD-20260729-001 复制 1 个项目、1 个变更事项到申请草稿");
      }
    }

    await wrapper.findAll("button").find((item) => item.text() === "构建执行台")!.trigger("click");
    expect(wrapper.text()).toContain("PRD-20260729-001");
    expect(wrapper.text()).toContain("删除任务");
    expect(wrapper.text()).toContain("Pipeline 流程");
    expect(wrapper.text()).toContain("GitLab Tags API");
    expect(wrapper.text()).toContain("构建 Jobs");
    expect(wrapper.text()).toContain("全量打 Tag 构建");
    expect(wrapper.text()).toContain("后端打 Tag 构建");
    expect(wrapper.text()).toContain("前端打 Tag 构建");
    expect(wrapper.text()).toContain("全量重打新 Tag 构建");
    expect(wrapper.text()).toContain("build-image");
    expect(wrapper.text()).toContain("编辑来源");
    expect(wrapper.text()).toContain("重试原 Pipeline");
    expect(wrapper.text()).toContain("最新来源重打 Tag");
    expect(wrapper.text()).toContain("GitLab");
    await wrapper.findAll("button").find((item) => item.text() === "编辑来源")!.trigger("click");
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).toContain("保存来源");
    const sourceInput = wrapper.findAll("input").find((item) => item.element.value === "master");
    expect(sourceInput, "missing release project source input").toBeTruthy();
    await sourceInput!.setValue("hotfix/checkout");
    await wrapper.findAll("button").find((item) => item.text() === "保存来源")!.trigger("click");
    await flushPromises();
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/releases/1/projects/1/source"),
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ sourceType: "branch", sourceRef: "hotfix/checkout" }),
      }),
    );
    await wrapper.findAll("button").find((item) => item.text() === "后端打 Tag 构建")!.trigger("click");
    await flushPromises();
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/releases/1/tag?target=backend&mode=resume"),
      expect.objectContaining({ method: "POST" }),
    );
    await wrapper.findAll("button").find((item) => item.text() === "最新来源重打 Tag")!.trigger("click");
    await flushPromises();
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/releases/1/projects/1/tag"),
      expect.objectContaining({ method: "POST" }),
    );

    await wrapper.findAll("button").find((item) => item.text() === "项目配置")!.trigger("click");
    expect(wrapper.text()).toContain("新增项目");
    expect(wrapper.text()).toContain("编辑");
    expect(wrapper.text()).toContain("删除");
    await wrapper.findAll("button").find((item) => item.text() === "新增项目")!.trigger("click");
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).toContain("默认业务线");

    await wrapper.findAll("button").find((item) => item.text() === "Tag 与依赖")!.trigger("click");
    expect(wrapper.text()).toContain("新增业务线");
    expect(wrapper.text()).toContain("项目依赖顺序");
    expect(wrapper.text()).toContain("新增依赖关系");
    expect(wrapper.text()).toContain("上移");
    expect(wrapper.text()).toContain("下移");
    expect(wrapper.text()).toContain("删除整行");
    expect(wrapper.text()).toContain("编辑");
    expect(wrapper.text()).toContain("删除");
    expect(wrapper.text()).not.toContain("关联项目 / 迁移到");

    const firstMoveDown = wrapper.findAll("button").find((item) => item.text() === "下移");
    expect(firstMoveDown, "missing project move down button").toBeTruthy();
    await firstMoveDown!.trigger("click");
    await flushPromises();
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/projects/order"),
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ codes: ["order-core", "base-auth"] }),
      }),
    );
    expect(wrapper.text()).toContain("打包顺序已保存");

    const opsDeleteButton = wrapper
      .findAll("tr")
      .find((row) => row.text().includes("OPS 平台业务线"))
      ?.findAll("button")
      .find((button) => button.text() === "删除");
    expect(opsDeleteButton, "missing OPS delete button").toBeTruthy();
    await opsDeleteButton!.trigger("click");
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).toContain("迁移后删除");
    expect(wrapper.text()).toContain("1 个项目");
    expect(wrapper.text()).toContain("迁移到");

    await wrapper.findAll("button").find((item) => item.text() === "发布历史")!.trigger("click");
    expect(wrapper.text()).toContain("PRD-20260729-001");
    expect(wrapper.text()).toContain("删除任务");
    expect(wrapper.text()).toContain("SQL 执行预览");
    expect(wrapper.text()).toContain("USE order_db;");

    await wrapper.findAll("button").find((item) => item.text() === "用户权限")!.trigger("click");
    expect(wrapper.text()).toContain("新增用户");
    expect(wrapper.text()).toContain("发布经理");
    expect(wrapper.text()).toContain("管理员");

    expect(consoleError).not.toHaveBeenCalled();
  });
});

async function mockFetch(input: RequestInfo | URL, init?: RequestInit) {
  const url = input.toString();
  const method = init?.method ?? "GET";

  if (method === "POST" && url.endsWith("/api/auth/login")) {
    return jsonResponse({
      token: "test-token",
      user: {
        id: 1,
        username: "admin",
        displayName: "高远",
        role: "admin",
        status: "enabled",
      },
    });
  }
  if (method === "GET" && url.endsWith("/api/projects")) {
    return jsonResponse(projectsPayload);
  }
  if (method === "PUT" && url.endsWith("/api/projects/order")) {
    const body = JSON.parse(String(init?.body ?? "{}")) as { codes?: string[] };
    const projectByCode = new Map(projectsPayload.map((project) => [project.code, project]));
    return jsonResponse(
      (body.codes ?? []).map((code, index) => {
        const project = projectByCode.get(code);
        if (!project) {
          throw new Error(`unknown project ${code}`);
        }
        return {
          ...project,
          sortOrder: (index + 1) * 10,
        };
      }),
    );
  }
  if (method === "GET" && url.endsWith("/api/business-lines")) {
    return jsonResponse(businessLinesPayload);
  }
  if (method === "GET" && url.endsWith("/api/releases")) {
    return jsonResponse(releasesPayload);
  }
  if (method === "POST" && url.includes("/api/releases/1/tag")) {
    return jsonResponse(releasesPayload[0]);
  }
  if (method === "POST" && url.includes("/api/releases/1/projects/1/tag")) {
    return jsonResponse(releasesPayload[0]);
  }
  if (method === "PUT" && url.includes("/api/releases/1/projects/1/source")) {
    const body = JSON.parse(String(init?.body ?? "{}"));
    return jsonResponse({
      ...releasesPayload[0],
      projects: [
        {
          ...releasesPayload[0].projects[0],
          sourceType: body.sourceType,
          sourceRef: body.sourceRef,
          sourceDirty: true,
        },
      ],
    });
  }
  if (method === "GET" && url.endsWith("/api/users")) {
    return jsonResponse(usersPayload);
  }

  return jsonResponse({ message: `Unhandled request: ${method} ${url}` }, 404);
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      "Content-Type": "application/json",
    },
  });
}

function createMemoryStorage(): Storage {
  const store = new Map<string, string>();
  return {
    get length() {
      return store.size;
    },
    clear() {
      store.clear();
    },
    getItem(key: string) {
      return store.get(key) ?? null;
    },
    key(index: number) {
      return Array.from(store.keys())[index] ?? null;
    },
    removeItem(key: string) {
      store.delete(key);
    },
    setItem(key: string, value: string) {
      store.set(key, value);
    },
  };
}
