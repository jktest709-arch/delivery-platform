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
    expect(wrapper.text()).toContain("订单核心服务");
    expect(wrapper.text()).toContain("上线单申请");

    for (const tab of ["构建执行台", "项目配置", "Tag 与依赖", "发布历史", "上线单申请"]) {
      const button = wrapper.findAll("button").find((item) => item.text() === tab);
      expect(button, `missing sidebar tab ${tab}`).toBeTruthy();
      await button!.trigger("click");
      await wrapper.vm.$nextTick();
      expect(wrapper.text()).toContain(tab);
    }

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
    return jsonResponse([]);
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
