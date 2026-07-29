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

describe("App", () => {
  let consoleError: ReturnType<typeof vi.spyOn> | undefined;

  beforeEach(() => {
    vi.stubGlobal("localStorage", createMemoryStorage());
    consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    vi.stubGlobal("fetch", vi.fn(mockFetch));
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

    for (const tab of ["构建执行台", "项目配置", "Tag 与依赖", "发布历史", "上线单申请"]) {
      const button = wrapper.findAll("button").find((item) => item.text() === tab);
      expect(button, `missing sidebar tab ${tab}`).toBeTruthy();
      await button!.trigger("click");
      await wrapper.vm.$nextTick();
      expect(wrapper.text()).toContain(tab);
    }

    await wrapper.findAll("button").find((item) => item.text() === "构建执行台")!.trigger("click");
    expect(wrapper.text()).toContain("PRD-20260729-001");
    expect(wrapper.text()).toContain("删除任务");
    expect(wrapper.text()).toContain("Pipeline 流程");
    expect(wrapper.text()).toContain("GitLab Tags API");
    expect(wrapper.text()).toContain("构建 Jobs");
    expect(wrapper.text()).toContain("build-image");
    expect(wrapper.text()).toContain("重新构建");
    expect(wrapper.text()).toContain("GitLab");

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
    expect(wrapper.text()).toContain("删除整行");
    expect(wrapper.text()).toContain("编辑");
    expect(wrapper.text()).toContain("删除");
    expect(wrapper.text()).not.toContain("关联项目 / 迁移到");

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
  if (method === "GET" && url.endsWith("/api/business-lines")) {
    return jsonResponse(businessLinesPayload);
  }
  if (method === "GET" && url.endsWith("/api/releases")) {
    return jsonResponse(releasesPayload);
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
