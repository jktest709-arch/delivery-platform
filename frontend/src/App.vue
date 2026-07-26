<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { api, clearToken, getToken, setToken } from "./api";
import type {
  BusinessLine,
  CreateReleasePayload,
  Project,
  Release,
  ReleaseProject,
  ReleaseTarget,
  SourceType,
  User,
} from "./types";

const tabs = [
  { key: "apply", label: "上线单申请" },
  { key: "console", label: "构建执行台" },
  { key: "projects", label: "项目配置" },
  { key: "rules", label: "Tag 与依赖" },
  { key: "history", label: "发布历史" },
];

const statusText: Record<string, string> = {
  pending: "待处理",
  tagged: "已打 Tag",
  building: "打包中",
  build_success: "打包完成",
  build_failed: "打包失败",
  deploying: "部署中",
  deploy_success: "部署成功",
  deploy_failed: "部署失败",
  partial_failed: "部分失败",
};

const sourceText: Record<SourceType, string> = {
  branch: "分支",
  tag: "Tag",
  commit: "Commit",
};

const kindText: Record<string, string> = {
  backend: "后端",
  frontend: "前端",
};

const user = ref<User | null>(null);
const projects = ref<Project[]>([]);
const lines = ref<BusinessLine[]>([]);
const releases = ref<Release[]>([]);
const selectedReleaseId = ref<number | null>(null);
const activeTab = ref("apply");
const loading = ref(false);
const message = ref("");
const error = ref("");

const loginForm = reactive({
  username: "admin",
  password: "admin123",
});

const releaseForm = reactive({
  releaseWindow: new Date(Date.now() + 2 * 60 * 60 * 1000).toISOString().slice(0, 16),
  remark: "",
});

type SourceForm = {
  sourceType: SourceType;
  sourceRef: string;
};

const selectedProjectCodes = ref<string[]>([]);
const sourceForms = reactive<Record<string, SourceForm>>({});
const dependencyText = reactive<Record<string, string>>({});

const canOperate = computed(() => {
  return user.value?.role === "release_manager" || user.value?.role === "admin";
});

const currentRelease = computed(() => {
  if (!releases.value.length) {
    return null;
  }
  return releases.value.find((item) => item.id === selectedReleaseId.value) ?? releases.value[0];
});

const orderedProjects = computed(() => {
  return [...projects.value].sort((a, b) => a.sortOrder - b.sortOrder);
});

const selectedProjects = computed(() => {
  const selected = new Set(selectedProjectCodes.value);
  return orderedProjects.value.filter((project) => selected.has(project.code));
});

const selectedBackendCount = computed(() => {
  return selectedProjects.value.filter((project) => project.kind === "backend").length;
});

const selectedFrontendCount = computed(() => {
  return selectedProjects.value.filter((project) => project.kind === "frontend").length;
});

onMounted(async () => {
  if (!getToken()) {
    return;
  }
  try {
    user.value = await api.me();
    await loadData();
  } catch {
    clearToken();
  }
});

async function login() {
  await run(async () => {
    const result = await api.login(loginForm.username, loginForm.password);
    setToken(result.token);
    user.value = result.user;
    await loadData();
    message.value = `已登录：${result.user.displayName}`;
  });
}

function logout() {
  clearToken();
  user.value = null;
  releases.value = [];
  projects.value = [];
  lines.value = [];
}

async function loadData() {
  const [projectResult, lineResult, releaseResult] = await Promise.all([
    api.projects(),
    api.businessLines(),
    api.releases(),
  ]);
  syncProjectState(projectResult);
  projects.value = projectResult;
  lines.value = lineResult;
  releases.value = releaseResult;
  selectedReleaseId.value = releaseResult[0]?.id ?? null;
}

function syncProjectState(projectResult: Project[]) {
  const activeCodes = new Set(projectResult.map((project) => project.code));
  for (const project of projectResult) {
    projectSourceForm(project);
    dependencyText[project.code] = projectDependencies(project).join(",");
  }
  for (const code of Object.keys(sourceForms)) {
    if (!activeCodes.has(code)) {
      delete sourceForms[code];
    }
  }
  for (const code of Object.keys(dependencyText)) {
    if (!activeCodes.has(code)) {
      delete dependencyText[code];
    }
  }

  selectedProjectCodes.value = selectedProjectCodes.value.filter((code) => activeCodes.has(code));
  if (selectedProjectCodes.value.length === 0) {
    selectedProjectCodes.value = projectResult.slice(0, 5).map((project) => project.code);
  }
}

function projectSourceForm(project: Project): SourceForm {
  if (!sourceForms[project.code]) {
    sourceForms[project.code] = {
      sourceType: "branch",
      sourceRef: project.defaultBranch || "main",
    };
  }
  return sourceForms[project.code];
}

function projectDependencies(project: Project) {
  return Array.isArray(project.dependencies) ? project.dependencies : [];
}

function toggleProject(code: string) {
  if (selectedProjectCodes.value.includes(code)) {
    selectedProjectCodes.value = selectedProjectCodes.value.filter((item) => item !== code);
    return;
  }
  selectedProjectCodes.value = [...selectedProjectCodes.value, code];
}

async function submitRelease() {
  const payload: CreateReleasePayload = {
    releaseWindow: new Date(releaseForm.releaseWindow).toISOString(),
    remark: releaseForm.remark,
    projects: selectedProjects.value.map((project) => {
      const form = projectSourceForm(project);
      return {
        projectCode: project.code,
        sourceType: form.sourceType,
        sourceRef: form.sourceRef,
      };
    }),
  };
  await run(async () => {
    const created = await api.createRelease(payload);
    upsertRelease(created);
    selectedReleaseId.value = created.id;
    activeTab.value = "console";
    message.value = `上线单 ${created.batchNo} 已提交`;
  });
}

async function releaseAction(action: "tag" | "package" | "deploy", target: ReleaseTarget = "all") {
  if (!currentRelease.value) {
    return;
  }
  await run(async () => {
    const updated =
      action === "tag"
        ? await api.createTags(currentRelease.value!.id)
        : action === "package"
          ? await api.packageRelease(currentRelease.value!.id, target)
          : await api.deployRelease(currentRelease.value!.id, target);
    upsertRelease(updated);
    message.value = `${updated.batchNo} 已更新`;
  });
}

async function projectAction(row: ReleaseProject, action: "package" | "deploy") {
  if (!currentRelease.value) {
    return;
  }
  await run(async () => {
    const updated =
      action === "package"
        ? await api.packageProject(currentRelease.value!.id, row.id)
        : await api.deployProject(currentRelease.value!.id, row.id);
    upsertRelease(updated);
    message.value = `${row.project.name} 已触发${action === "package" ? "打包" : "部署"}`;
  });
}

async function saveProject(project: Project) {
  await run(async () => {
    const nextProjects = await api.updateProject(project);
    syncProjectState(nextProjects);
    projects.value = nextProjects;
    message.value = `${project.name} 配置已保存`;
  });
}

async function saveLine(line: BusinessLine) {
  await run(async () => {
    lines.value = await api.updateBusinessLine(line);
    message.value = `${line.name} 配置已保存`;
  });
}

async function saveDependencies(project: Project) {
  const dependencies = (dependencyText[project.code] ?? "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
  await run(async () => {
    const nextProjects = await api.updateDependencies(project.code, dependencies);
    syncProjectState(nextProjects);
    projects.value = nextProjects;
    message.value = `${project.name} 依赖已保存`;
  });
}

function upsertRelease(updated: Release) {
  const index = releases.value.findIndex((item) => item.id === updated.id);
  if (index >= 0) {
    releases.value.splice(index, 1, updated);
  } else {
    releases.value.unshift(updated);
  }
  selectedReleaseId.value = updated.id;
}

async function run(task: () => Promise<void>) {
  loading.value = true;
  error.value = "";
  message.value = "";
  try {
    await task();
  } catch (err) {
    error.value = err instanceof Error ? err.message : "操作失败";
  } finally {
    loading.value = false;
  }
}

function formatDate(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString("zh-CN", { hour12: false });
}

function statusLabel(status: string) {
  return statusText[status] ?? status;
}
</script>

<template>
  <main v-if="!user" class="login-shell">
    <form class="login-panel" @submit.prevent="login">
      <div>
        <p class="eyebrow">GitLab CI</p>
        <h1>统一交付平台</h1>
      </div>
      <label>
        <span>用户名</span>
        <input v-model="loginForm.username" autocomplete="username" />
      </label>
      <label>
        <span>密码</span>
        <input v-model="loginForm.password" type="password" autocomplete="current-password" />
      </label>
      <button class="primary" :disabled="loading">登录</button>
      <p v-if="error" class="notice danger">{{ error }}</p>
    </form>
  </main>

  <main v-else class="app-shell">
    <aside class="sidebar">
      <div class="brand">
        <span>DP</span>
        <strong>统一交付平台</strong>
      </div>
      <button
        v-for="tab in tabs"
        :key="tab.key"
        :class="{ active: activeTab === tab.key }"
        @click="activeTab = tab.key"
      >
        {{ tab.label }}
      </button>
    </aside>

    <section class="workspace">
      <header class="topbar">
        <div>
          <p class="eyebrow">Production Delivery</p>
          <h1>{{ tabs.find((tab) => tab.key === activeTab)?.label }}</h1>
        </div>
        <div class="userbar">
          <span>{{ user.displayName }}</span>
          <small>{{ user.role }}</small>
          <button @click="logout">退出</button>
        </div>
      </header>

      <div v-if="message" class="notice success">{{ message }}</div>
      <div v-if="error" class="notice danger">{{ error }}</div>

      <section v-if="activeTab === 'apply'" class="panel">
        <div class="section-head">
          <h2>上线单</h2>
          <div class="counter">
            <span>{{ selectedProjects.length }} 个项目</span>
            <span>{{ selectedBackendCount }} 后端</span>
            <span>{{ selectedFrontendCount }} 前端</span>
          </div>
        </div>

        <div class="form-grid two">
          <label>
            <span>上线窗口</span>
            <input v-model="releaseForm.releaseWindow" type="datetime-local" />
          </label>
          <label>
            <span>备注</span>
            <input v-model="releaseForm.remark" placeholder="PRD 发布批次说明" />
          </label>
        </div>

        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>选择</th>
                <th>顺序</th>
                <th>项目</th>
                <th>类型</th>
                <th>业务线</th>
                <th>代码来源</th>
                <th>分支 / Tag / Commit</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="project in orderedProjects" :key="project.code">
                <td>
                  <input
                    type="checkbox"
                    :checked="selectedProjectCodes.includes(project.code)"
                    @change="toggleProject(project.code)"
                  />
                </td>
                <td>{{ project.sortOrder }}</td>
                <td>
                  <strong>{{ project.name }}</strong>
                  <small>{{ project.gitlabProjectId }}</small>
                </td>
                <td>{{ kindText[project.kind] }}</td>
                <td>{{ project.businessLine?.name }}</td>
                <td>
                  <select v-model="projectSourceForm(project).sourceType">
                    <option value="branch">{{ sourceText.branch }}</option>
                    <option value="tag">{{ sourceText.tag }}</option>
                    <option value="commit">{{ sourceText.commit }}</option>
                  </select>
                </td>
                <td>
                  <input v-model="projectSourceForm(project).sourceRef" />
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="actions">
          <button class="primary" :disabled="loading || selectedProjects.length === 0" @click="submitRelease">
            提交上线单
          </button>
        </div>
      </section>

      <section v-if="activeTab === 'console'" class="panel">
        <div class="section-head">
          <h2>执行台</h2>
          <select v-model.number="selectedReleaseId">
            <option v-for="item in releases" :key="item.id" :value="item.id">
              {{ item.batchNo }}
            </option>
          </select>
        </div>

        <template v-if="currentRelease">
          <div class="release-summary">
            <div>
              <small>批次</small>
              <strong>{{ currentRelease.batchNo }}</strong>
            </div>
            <div>
              <small>状态</small>
              <strong>{{ statusLabel(currentRelease.status) }}</strong>
            </div>
            <div>
              <small>上线窗口</small>
              <strong>{{ formatDate(currentRelease.releaseWindow) }}</strong>
            </div>
            <div>
              <small>申请人</small>
              <strong>{{ currentRelease.applicant?.displayName }}</strong>
            </div>
          </div>

          <div class="actions split">
            <button :disabled="!canOperate || loading" @click="releaseAction('tag')">统一打 Tag</button>
            <button :disabled="!canOperate || loading" @click="releaseAction('package', 'all')">全量打包</button>
            <button :disabled="!canOperate || loading" @click="releaseAction('package', 'backend')">后端打包</button>
            <button :disabled="!canOperate || loading" @click="releaseAction('package', 'frontend')">前端打包</button>
            <button class="primary" :disabled="!canOperate || loading" @click="releaseAction('deploy', 'all')">
              生产部署
            </button>
          </div>

          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>顺序</th>
                  <th>项目</th>
                  <th>来源</th>
                  <th>目标 Tag</th>
                  <th>Pipeline</th>
                  <th>状态</th>
                  <th>单项目操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in currentRelease.projects" :key="row.id">
                  <td>{{ row.sortOrder }}</td>
                  <td>
                    <strong>{{ row.project.name }}</strong>
                    <small>{{ kindText[row.project.kind] }} / {{ row.project.owner }}</small>
                  </td>
                  <td>{{ sourceText[row.sourceType] }}: {{ row.sourceRef }}</td>
                  <td><code>{{ row.targetTag }}</code></td>
                  <td>{{ row.pipelineId || "-" }}</td>
                  <td>
                    <span class="status" :class="row.status">{{ statusLabel(row.status) }}</span>
                  </td>
                  <td class="inline-actions">
                    <button :disabled="!canOperate || loading" @click="projectAction(row, 'package')">打包</button>
                    <button :disabled="!canOperate || loading" @click="projectAction(row, 'deploy')">部署</button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>
      </section>

      <section v-if="activeTab === 'projects'" class="panel">
        <div class="section-head">
          <h2>项目配置</h2>
        </div>
        <div class="project-editor">
          <article v-for="project in orderedProjects" :key="project.code" class="editor-row">
            <div class="row-title">
              <strong>{{ project.name }}</strong>
              <small>{{ project.code }}</small>
            </div>
            <label>
              <span>类型</span>
              <select v-model="project.kind">
                <option value="backend">后端</option>
                <option value="frontend">前端</option>
              </select>
            </label>
            <label>
              <span>业务线</span>
              <select v-model="project.businessLineCode">
                <option v-for="line in lines" :key="line.code" :value="line.code">{{ line.name }}</option>
              </select>
            </label>
            <label>
              <span>GitLab 地址</span>
              <input v-model="project.gitlabUrl" />
            </label>
            <label>
              <span>Project ID</span>
              <input v-model="project.gitlabProjectId" />
            </label>
            <label>
              <span>默认分支</span>
              <input v-model="project.defaultBranch" />
            </label>
            <label>
              <span>打包 Job</span>
              <input v-model="project.packageJob" />
            </label>
            <label>
              <span>部署 Job</span>
              <input v-model="project.deployJob" />
            </label>
            <label>
              <span>排序</span>
              <input v-model.number="project.sortOrder" type="number" />
            </label>
            <button :disabled="loading" @click="saveProject(project)">保存</button>
          </article>
        </div>
      </section>

      <section v-if="activeTab === 'rules'" class="panel stack">
        <div class="section-head">
          <h2>业务线 Tag</h2>
        </div>
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>业务线</th>
                <th>平台</th>
                <th>前缀</th>
                <th>模板</th>
                <th>审批人</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="line in lines" :key="line.code">
                <td><input v-model="line.name" /></td>
                <td><input v-model="line.platform" /></td>
                <td><input v-model="line.tagPrefix" /></td>
                <td><input v-model="line.tagTemplate" /></td>
                <td><input v-model="line.approver" /></td>
                <td><button :disabled="loading" @click="saveLine(line)">保存</button></td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="section-head">
          <h2>项目依赖顺序</h2>
        </div>
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>顺序</th>
                <th>项目</th>
                <th>依赖项目 Code</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="project in orderedProjects" :key="project.code">
                <td>{{ project.sortOrder }}</td>
                <td>{{ project.name }}</td>
                <td><input v-model="dependencyText[project.code]" /></td>
                <td><button :disabled="loading" @click="saveDependencies(project)">保存</button></td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section v-if="activeTab === 'history'" class="panel history-layout">
        <div class="history-list">
          <button
            v-for="item in releases"
            :key="item.id"
            :class="{ active: selectedReleaseId === item.id }"
            @click="selectedReleaseId = item.id"
          >
            <strong>{{ item.batchNo }}</strong>
            <span>{{ statusLabel(item.status) }}</span>
          </button>
        </div>

        <div v-if="currentRelease" class="history-detail">
          <div class="section-head">
            <h2>{{ currentRelease.batchNo }}</h2>
            <span class="status" :class="currentRelease.status">{{ statusLabel(currentRelease.status) }}</span>
          </div>
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>项目</th>
                  <th>来源</th>
                  <th>Tag</th>
                  <th>状态</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in currentRelease.projects" :key="row.id">
                  <td>{{ row.project.name }}</td>
                  <td>{{ sourceText[row.sourceType] }}: {{ row.sourceRef }}</td>
                  <td><code>{{ row.targetTag }}</code></td>
                  <td>{{ statusLabel(row.status) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="timeline">
            <article v-for="event in currentRelease.events" :key="event.id">
              <time>{{ formatDate(event.createdAt) }}</time>
              <span>{{ event.operator?.displayName || "系统" }}</span>
              <p>{{ event.message }}</p>
            </article>
          </div>
        </div>
      </section>
    </section>
  </main>
</template>
