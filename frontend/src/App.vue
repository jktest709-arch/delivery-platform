<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { api, clearToken, getToken, setToken } from "./api";
import type {
  BusinessLine,
  BusinessLinePayload,
  CreateReleasePayload,
  Project,
  ProjectKind,
  ProjectPayload,
  Release,
  ReleaseProject,
  ReleaseTarget,
  Role,
  SourceType,
  User,
  UserPayload,
} from "./types";

const tabs = [
  { key: "apply", label: "上线单申请" },
  { key: "console", label: "构建执行台" },
  { key: "projects", label: "项目配置" },
  { key: "rules", label: "Tag 与依赖" },
  { key: "history", label: "发布历史" },
  { key: "users", label: "用户权限" },
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

const roleText: Record<Role, string> = {
  developer: "开发",
  release_manager: "发布经理",
  admin: "管理员",
};

const user = ref<User | null>(null);
const users = ref<User[]>([]);
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

type UserForm = UserPayload & {
  password: string;
};
type ProjectForm = ProjectPayload;
type BusinessLineForm = BusinessLinePayload;
type DependencyForm = {
  projectCode: string;
  dependencyCode: string;
};
type DependencyRow = {
  key: string;
  project: Project;
  dependencyCode: string;
  isEmpty: boolean;
  isEditing: boolean;
};

const selectedProjectCodes = ref<string[]>([]);
const sourceForms = reactive<Record<string, SourceForm>>({});
const dependencyText = reactive<Record<string, string>>({});
const editingUserId = ref<number | null>(null);
const userDrafts = reactive<Record<number, UserForm>>({});
const showNewUserForm = ref(false);
const newUserForm = reactive<UserForm>(emptyUserForm());
const editingProjectCode = ref<string | null>(null);
const projectDrafts = reactive<Record<string, ProjectForm>>({});
const showNewProjectForm = ref(false);
const newProjectForm = reactive<ProjectForm>(emptyProjectForm());
const editingLineCode = ref<string | null>(null);
const lineDrafts = reactive<Record<string, BusinessLineForm>>({});
const lineReplacementCodes = reactive<Record<string, string>>({});
const deletingLineCode = ref<string | null>(null);
const showNewLineForm = ref(false);
const newLineForm = reactive<BusinessLineForm>(emptyBusinessLineForm());
const editingDependencyCode = ref<string | null>(null);
const dependencyDrafts = reactive<Record<string, string>>({});
const showNewDependencyForm = ref(false);
const newDependencyForm = reactive<DependencyForm>({
  projectCode: "",
  dependencyCode: "",
});

const canOperate = computed(() => {
  return user.value?.role === "release_manager" || user.value?.role === "admin";
});

const canManageUsers = computed(() => user.value?.role === "admin");

const visibleTabs = computed(() => {
  return tabs.filter((tab) => tab.key !== "users" || canManageUsers.value);
});

const currentTabLabel = computed(() => {
  return visibleTabs.value.find((tab) => tab.key === activeTab.value)?.label ?? "上线单申请";
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

const dependencyRows = computed<DependencyRow[]>(() => {
  const rows: DependencyRow[] = [];
  for (const project of orderedProjects.value) {
    if (editingDependencyCode.value === project.code) {
      rows.push({
        key: `${project.code}:editing`,
        project,
        dependencyCode: "",
        isEmpty: false,
        isEditing: true,
      });
      continue;
    }

    const dependencies = projectDependencies(project);
    if (dependencies.length === 0) {
      rows.push({
        key: `${project.code}:empty`,
        project,
        dependencyCode: "",
        isEmpty: true,
        isEditing: false,
      });
      continue;
    }
    for (const dependencyCode of dependencies) {
      rows.push({
        key: `${project.code}:${dependencyCode}`,
        project,
        dependencyCode,
        isEmpty: false,
        isEditing: false,
      });
    }
  }
  return rows;
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
  users.value = [];
  releases.value = [];
  projects.value = [];
  lines.value = [];
  activeTab.value = "request";
}

async function loadData() {
  const [projectResult, lineResult, releaseResult, userResult] = await Promise.all([
    api.projects(),
    api.businessLines(),
    api.releases(),
    canManageUsers.value ? api.users() : Promise.resolve([]),
  ]);
  projects.value = projectResult;
  lines.value = lineResult;
  users.value = userResult;
  syncProjectState(projectResult);
  syncLineState(lineResult);
  syncUserState(userResult);
  releases.value = releaseResult;
  selectedReleaseId.value = releaseResult[0]?.id ?? null;
  if (!canManageUsers.value && activeTab.value === "users") {
    activeTab.value = "request";
  }
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
  for (const code of Object.keys(projectDrafts)) {
    if (!activeCodes.has(code)) {
      delete projectDrafts[code];
    }
  }
  for (const code of Object.keys(dependencyDrafts)) {
    if (!activeCodes.has(code)) {
      delete dependencyDrafts[code];
    }
  }

  selectedProjectCodes.value = selectedProjectCodes.value.filter((code) => activeCodes.has(code));
  if (editingProjectCode.value && !activeCodes.has(editingProjectCode.value)) {
    editingProjectCode.value = null;
  }
  if (editingDependencyCode.value && !activeCodes.has(editingDependencyCode.value)) {
    editingDependencyCode.value = null;
  }
  if (showNewDependencyForm.value && !activeCodes.has(newDependencyForm.projectCode)) {
    Object.assign(newDependencyForm, emptyDependencyForm());
  }
  if (selectedProjectCodes.value.length === 0) {
    selectedProjectCodes.value = projectResult.slice(0, 5).map((project) => project.code);
  }
}

function syncLineState(lineResult: BusinessLine[]) {
  const activeCodes = new Set(lineResult.map((line) => line.code));
  for (const code of Object.keys(lineDrafts)) {
    if (!activeCodes.has(code)) {
      delete lineDrafts[code];
    }
  }
  for (const code of Object.keys(lineReplacementCodes)) {
    if (!activeCodes.has(code)) {
      delete lineReplacementCodes[code];
    }
  }
  if (editingLineCode.value && !activeCodes.has(editingLineCode.value)) {
    editingLineCode.value = null;
  }
  if (deletingLineCode.value && !activeCodes.has(deletingLineCode.value)) {
    deletingLineCode.value = null;
  }
  for (const line of lineResult) {
    const options = replacementLineOptions(line.code);
    if (!lineReplacementCodes[line.code] || !options.some((item) => item.code === lineReplacementCodes[line.code])) {
      lineReplacementCodes[line.code] = options[0]?.code ?? "";
    }
  }
}

function syncUserState(userResult: User[]) {
  const activeIds = new Set(userResult.map((item) => item.id));
  for (const id of Object.keys(userDrafts)) {
    if (!activeIds.has(Number(id))) {
      delete userDrafts[Number(id)];
    }
  }
  if (editingUserId.value && !activeIds.has(editingUserId.value)) {
    editingUserId.value = null;
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

async function deleteRelease(item: Release) {
  if (!canOperate.value) {
    return;
  }
  if (!window.confirm(`确认删除发布任务 ${item.batchNo}？相关项目任务和操作记录也会一起删除。`)) {
    return;
  }
  await run(async () => {
    const nextReleases = await api.deleteRelease(item.id);
    releases.value = nextReleases;
    selectedReleaseId.value = nextReleases[0]?.id ?? null;
    message.value = `${item.batchNo} 已删除`;
  });
}

function emptyUserForm(): UserForm {
  return {
    username: "",
    displayName: "",
    role: "developer",
    status: "enabled",
    password: "",
  };
}

function userToForm(item: User): UserForm {
  return {
    username: item.username,
    displayName: item.displayName,
    role: item.role,
    status: item.status,
    password: "",
  };
}

function userDraft(item: User): UserForm {
  if (!userDrafts[item.id]) {
    userDrafts[item.id] = userToForm(item);
  }
  return userDrafts[item.id];
}

function normalizeUserForm(form: UserForm): UserPayload {
  return {
    username: form.username.trim(),
    displayName: form.displayName.trim(),
    role: form.role,
    status: form.status,
    password: form.password.trim() || undefined,
  };
}

function openNewUserForm() {
  Object.assign(newUserForm, emptyUserForm());
  showNewUserForm.value = true;
  editingUserId.value = null;
}

function cancelNewUserForm() {
  showNewUserForm.value = false;
}

function startUserEdit(item: User) {
  userDrafts[item.id] = userToForm(item);
  editingUserId.value = item.id;
  showNewUserForm.value = false;
}

function cancelUserEdit(id: number) {
  delete userDrafts[id];
  if (editingUserId.value === id) {
    editingUserId.value = null;
  }
}

async function createUserFromDraft() {
  const payload = normalizeUserForm(newUserForm);
  await run(async () => {
    users.value = await api.createUser(payload);
    syncUserState(users.value);
    showNewUserForm.value = false;
    message.value = `${payload.displayName} 已新增`;
  });
}

async function saveUserDraft(item: User) {
  const payload = normalizeUserForm(userDraft(item));
  await run(async () => {
    users.value = await api.updateUser(item.id, payload);
    syncUserState(users.value);
    cancelUserEdit(item.id);
    message.value = `${payload.displayName} 已保存`;
  });
}

async function deleteUser(item: User) {
  if (user.value?.id === item.id) {
    error.value = "不能删除当前登录用户";
    return;
  }
  if (!window.confirm(`确认删除用户 ${item.displayName}？`)) {
    return;
  }
  await run(async () => {
    users.value = await api.deleteUser(item.id);
    syncUserState(users.value);
    message.value = `${item.displayName} 已删除`;
  });
}

function emptyProjectForm(): ProjectForm {
  const nextOrder =
    projects.value.length > 0 ? Math.max(...projects.value.map((project) => project.sortOrder)) + 10 : 10;
  return {
    code: "",
    name: "",
    kind: "backend",
    owner: "",
    businessLineCode: lines.value[0]?.code ?? "",
    gitlabUrl: "",
    gitlabProjectId: "",
    defaultBranch: "master",
    packageJob: "build-prd",
    deployJob: "deploy-prd",
    sortOrder: nextOrder,
    enabled: true,
  };
}

function projectToForm(project: Project): ProjectForm {
  return {
    code: project.code,
    name: project.name,
    kind: project.kind,
    owner: project.owner,
    businessLineCode: project.businessLineCode || project.businessLine?.code || "",
    gitlabUrl: project.gitlabUrl,
    gitlabProjectId: project.gitlabProjectId,
    defaultBranch: project.defaultBranch,
    packageJob: project.packageJob,
    deployJob: project.deployJob,
    sortOrder: project.sortOrder,
    enabled: project.enabled,
  };
}

function projectDraft(project: Project): ProjectForm {
  if (!projectDrafts[project.code]) {
    projectDrafts[project.code] = projectToForm(project);
  }
  return projectDrafts[project.code];
}

function normalizeProjectForm(form: ProjectForm): ProjectForm {
  return {
    ...form,
    code: form.code.trim(),
    name: form.name.trim(),
    kind: form.kind as ProjectKind,
    owner: form.owner.trim(),
    businessLineCode: form.businessLineCode.trim(),
    gitlabUrl: form.gitlabUrl.trim(),
    gitlabProjectId: form.gitlabProjectId.trim(),
    defaultBranch: form.defaultBranch.trim(),
    packageJob: form.packageJob.trim(),
    deployJob: form.deployJob.trim(),
    sortOrder: Number(form.sortOrder) || 0,
    enabled: Boolean(form.enabled),
  };
}

function openNewProjectForm() {
  Object.assign(newProjectForm, emptyProjectForm());
  showNewProjectForm.value = true;
  editingProjectCode.value = null;
}

function cancelNewProjectForm() {
  showNewProjectForm.value = false;
}

function startProjectEdit(project: Project) {
  projectDrafts[project.code] = projectToForm(project);
  editingProjectCode.value = project.code;
  showNewProjectForm.value = false;
}

function cancelProjectEdit(code: string) {
  delete projectDrafts[code];
  if (editingProjectCode.value === code) {
    editingProjectCode.value = null;
  }
}

async function createProjectFromDraft() {
  const payload = normalizeProjectForm(newProjectForm);
  await run(async () => {
    const nextProjects = await api.createProject(payload);
    syncProjectState(nextProjects);
    projects.value = nextProjects;
    showNewProjectForm.value = false;
    message.value = `${payload.name} 已新增`;
  });
}

async function saveProjectDraft(project: Project) {
  const payload = normalizeProjectForm(projectDraft(project));
  await run(async () => {
    const nextProjects = await api.updateProject(payload);
    syncProjectState(nextProjects);
    projects.value = nextProjects;
    cancelProjectEdit(project.code);
    message.value = `${payload.name} 配置已保存`;
  });
}

async function deleteProject(project: Project) {
  if (!window.confirm(`确认删除项目 ${project.name}？`)) {
    return;
  }
  await run(async () => {
    const nextProjects = await api.deleteProject(project.code);
    syncProjectState(nextProjects);
    projects.value = nextProjects;
    cancelProjectEdit(project.code);
    message.value = `${project.name} 已删除`;
  });
}

function emptyBusinessLineForm(): BusinessLineForm {
  return {
    code: "",
    name: "",
    platform: "",
    tagPrefix: "",
    tagTemplate: "{prefix}-{timestamp}-{releaseNo}",
    approver: "",
  };
}

function lineToForm(line: BusinessLine): BusinessLineForm {
  return {
    code: line.code,
    name: line.name,
    platform: line.platform,
    tagPrefix: line.tagPrefix,
    tagTemplate: line.tagTemplate,
    approver: line.approver,
  };
}

function lineDraft(line: BusinessLine): BusinessLineForm {
  if (!lineDrafts[line.code]) {
    lineDrafts[line.code] = lineToForm(line);
  }
  return lineDrafts[line.code];
}

function normalizeLineForm(form: BusinessLineForm): BusinessLineForm {
  return {
    code: form.code.trim(),
    name: form.name.trim(),
    platform: form.platform.trim(),
    tagPrefix: form.tagPrefix.trim(),
    tagTemplate: form.tagTemplate.trim() || "{prefix}-{timestamp}-{releaseNo}",
    approver: form.approver.trim(),
  };
}

function openNewLineForm() {
  Object.assign(newLineForm, emptyBusinessLineForm());
  showNewLineForm.value = true;
  editingLineCode.value = null;
  deletingLineCode.value = null;
}

function cancelNewLineForm() {
  showNewLineForm.value = false;
}

function startLineEdit(line: BusinessLine) {
  lineDrafts[line.code] = lineToForm(line);
  editingLineCode.value = line.code;
  deletingLineCode.value = null;
  showNewLineForm.value = false;
}

function cancelLineEdit(code: string) {
  delete lineDrafts[code];
  if (editingLineCode.value === code) {
    editingLineCode.value = null;
  }
}

async function createLineFromDraft() {
  const payload = normalizeLineForm(newLineForm);
  await run(async () => {
    lines.value = await api.createBusinessLine(payload);
    syncLineState(lines.value);
    showNewLineForm.value = false;
    message.value = `${payload.name} 已新增`;
  });
}

async function saveLineDraft(line: BusinessLine) {
  const payload = normalizeLineForm(lineDraft(line));
  await run(async () => {
    lines.value = await api.updateBusinessLine(payload);
    syncLineState(lines.value);
    cancelLineEdit(line.code);
    message.value = `${payload.name} 配置已保存`;
  });
}

function lineUsageCount(code: string) {
  return projects.value.filter((project) => (project.businessLineCode || project.businessLine?.code) === code).length;
}

function businessLineName(code: string) {
  return lines.value.find((line) => line.code === code)?.name ?? code;
}

function replacementLineOptions(code: string) {
  return lines.value.filter((line) => line.code !== code);
}

function prepareLineDelete(line: BusinessLine) {
  const usageCount = lineUsageCount(line.code);
  if (usageCount === 0) {
    void deleteLine(line);
    return;
  }
  const options = replacementLineOptions(line.code);
  if (options.length === 0) {
    error.value = "该业务线已被项目使用，请先新增替代业务线";
    return;
  }
  if (!lineReplacementCodes[line.code] || !options.some((item) => item.code === lineReplacementCodes[line.code])) {
    lineReplacementCodes[line.code] = options[0].code;
  }
  deletingLineCode.value = line.code;
  editingLineCode.value = null;
}

function cancelLineDelete(code: string) {
  if (deletingLineCode.value === code) {
    deletingLineCode.value = null;
  }
}

async function deleteLine(line: BusinessLine) {
  const usageCount = lineUsageCount(line.code);
  const replacementCode = usageCount > 0 ? lineReplacementCodes[line.code] : "";
  if (usageCount > 0 && !replacementCode) {
    error.value = "该业务线已被项目使用，请先新增或选择替代业务线";
    return;
  }
  const suffix = usageCount > 0 ? `，${usageCount} 个关联项目会迁移到 ${businessLineName(replacementCode)}` : "";
  if (!window.confirm(`确认删除业务线 ${line.name}${suffix}？`)) {
    return;
  }
  await run(async () => {
    lines.value = await api.deleteBusinessLine(line.code, replacementCode);
    const projectResult = await api.projects();
    projects.value = projectResult;
    syncProjectState(projectResult);
    syncLineState(lines.value);
    cancelLineDelete(line.code);
    cancelLineEdit(line.code);
    message.value = `${line.name} 已删除`;
  });
}

function dependencyDraft(project: Project) {
  if (dependencyDrafts[project.code] === undefined) {
    dependencyDrafts[project.code] = projectDependencies(project).join(",");
  }
  return dependencyDrafts[project.code];
}

function startDependencyEdit(project: Project) {
  dependencyDrafts[project.code] = projectDependencies(project).join(",");
  editingDependencyCode.value = project.code;
}

function cancelDependencyEdit(code: string) {
  delete dependencyDrafts[code];
  if (editingDependencyCode.value === code) {
    editingDependencyCode.value = null;
  }
}

async function saveDependencyDraft(project: Project) {
  const dependencies = dependencyDraft(project)
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
  await run(async () => {
    const nextProjects = await api.updateDependencies(project.code, dependencies);
    syncProjectState(nextProjects);
    projects.value = nextProjects;
    cancelDependencyEdit(project.code);
    message.value = `${project.name} 依赖已保存`;
  });
}

async function clearDependencyDraft(project: Project) {
  dependencyDrafts[project.code] = "";
  await saveDependencyDraft(project);
}

function emptyDependencyForm(): DependencyForm {
  const project = orderedProjects.value.find((item) => availableDependenciesForProject(item.code).length > 0);
  const dependency = project ? availableDependenciesForProject(project.code)[0] : null;
  return {
    projectCode: project?.code ?? orderedProjects.value[0]?.code ?? "",
    dependencyCode: dependency?.code ?? "",
  };
}

function dependencyProjectName(code: string) {
  return projects.value.find((project) => project.code === code)?.name ?? code;
}

function availableDependenciesForProject(projectCode: string) {
  const project = projects.value.find((item) => item.code === projectCode);
  if (!project) {
    return [];
  }
  const existing = new Set(projectDependencies(project));
  return orderedProjects.value.filter((candidate) => candidate.code !== projectCode && !existing.has(candidate.code));
}

function openNewDependencyForm() {
  Object.assign(newDependencyForm, emptyDependencyForm());
  showNewDependencyForm.value = true;
  editingDependencyCode.value = null;
}

function cancelNewDependencyForm() {
  showNewDependencyForm.value = false;
}

function syncNewDependencyCandidate() {
  newDependencyForm.dependencyCode = availableDependenciesForProject(newDependencyForm.projectCode)[0]?.code ?? "";
}

async function createDependencyFromDraft() {
  const project = projects.value.find((item) => item.code === newDependencyForm.projectCode);
  if (!project || !newDependencyForm.dependencyCode) {
    error.value = "请选择项目和依赖项目";
    return;
  }
  const dependencies = Array.from(new Set([...projectDependencies(project), newDependencyForm.dependencyCode]));
  await run(async () => {
    const nextProjects = await api.updateDependencies(project.code, dependencies);
    syncProjectState(nextProjects);
    projects.value = nextProjects;
    showNewDependencyForm.value = false;
    message.value = `${project.name} 已新增依赖 ${dependencyProjectName(newDependencyForm.dependencyCode)}`;
  });
}

async function deleteDependency(project: Project, dependencyCode: string) {
  if (!window.confirm(`确认删除整行依赖关系：${project.name} -> ${dependencyProjectName(dependencyCode)}？`)) {
    return;
  }
  const dependencies = projectDependencies(project).filter((code) => code !== dependencyCode);
  await run(async () => {
    const nextProjects = await api.updateDependencies(project.code, dependencies);
    syncProjectState(nextProjects);
    projects.value = nextProjects;
    message.value = `${project.name} 已删除整行依赖关系`;
  });
}

function tagStepState(row: ReleaseProject) {
  if (row.targetTag && row.status !== "pending") {
    return "done";
  }
  return row.targetTag ? "ready" : "pending";
}

function packageStepState(row: ReleaseProject) {
  if (row.status === "build_failed") {
    return "failed";
  }
  if (row.status === "building") {
    return "running";
  }
  if (["build_success", "deploying", "deploy_success", "deploy_failed"].includes(row.status)) {
    return "done";
  }
  return "pending";
}

function deployStepState(row: ReleaseProject) {
  if (row.status === "deploy_failed") {
    return "failed";
  }
  if (row.status === "deploying") {
    return "running";
  }
  if (row.status === "deploy_success") {
    return "done";
  }
  return "pending";
}

function pipelineStepLabel(state: string) {
  const labels: Record<string, string> = {
    pending: "待触发",
    ready: "已生成",
    running: "执行中",
    done: "完成",
    failed: "失败",
  };
  return labels[state] ?? state;
}

function pipelineUrl(row: ReleaseProject, id: string) {
  if (!id || !row.project.gitlabUrl) {
    return "";
  }
  return `${row.project.gitlabUrl.replace(/\/$/, "")}/-/pipelines/${encodeURIComponent(id)}`;
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
        v-for="tab in visibleTabs"
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
          <h1>{{ currentTabLabel }}</h1>
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
          <div class="head-actions">
            <select v-model.number="selectedReleaseId">
              <option v-for="item in releases" :key="item.id" :value="item.id">
                {{ item.batchNo }}
              </option>
            </select>
            <button
              class="danger-button"
              :disabled="!canOperate || loading || !currentRelease"
              @click="currentRelease && deleteRelease(currentRelease)"
            >
              删除任务
            </button>
          </div>
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

          <div class="pipeline-flow">
            <div class="section-head compact-head">
              <h2>Pipeline 流程</h2>
              <small>GitLab Tags API -> Pipeline API(package) -> Pipeline API(deploy)</small>
            </div>
            <div class="pipeline-list">
              <article v-for="row in currentRelease.projects" :key="row.id" class="pipeline-row">
                <div>
                  <strong>{{ row.project.name }}</strong>
                  <small>{{ row.project.gitlabProjectId }}</small>
                </div>
                <div class="pipeline-step" :class="tagStepState(row)">
                  <span>{{ pipelineStepLabel(tagStepState(row)) }}</span>
                  <strong>创建 Tag</strong>
                  <code>{{ row.targetTag || "-" }}</code>
                </div>
                <div class="pipeline-step" :class="packageStepState(row)">
                  <span>{{ pipelineStepLabel(packageStepState(row)) }}</span>
                  <strong>打包 Pipeline</strong>
                  <a v-if="row.buildJobId" :href="pipelineUrl(row, row.buildJobId)" target="_blank">
                    #{{ row.buildJobId }}
                  </a>
                  <small v-else>{{ row.project.packageJob }}</small>
                </div>
                <div class="pipeline-step" :class="deployStepState(row)">
                  <span>{{ pipelineStepLabel(deployStepState(row)) }}</span>
                  <strong>部署 Pipeline</strong>
                  <a v-if="row.deployJobId" :href="pipelineUrl(row, row.deployJobId)" target="_blank">
                    #{{ row.deployJobId }}
                  </a>
                  <small v-else>{{ row.project.deployJob }}</small>
                </div>
              </article>
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
          <button class="primary" :disabled="loading" @click="openNewProjectForm">新增项目</button>
        </div>

        <form v-if="showNewProjectForm" class="config-form" @submit.prevent="createProjectFromDraft">
          <div class="config-grid">
            <label>
              <span>项目 Code</span>
              <input v-model="newProjectForm.code" required />
            </label>
            <label>
              <span>项目名称</span>
              <input v-model="newProjectForm.name" required />
            </label>
            <label>
              <span>类型</span>
              <select v-model="newProjectForm.kind">
                <option value="backend">后端</option>
                <option value="frontend">前端</option>
              </select>
            </label>
            <label>
              <span>业务线</span>
              <select v-model="newProjectForm.businessLineCode" required>
                <option v-for="line in lines" :key="line.code" :value="line.code">{{ line.name }}</option>
              </select>
            </label>
            <label class="wide">
              <span>GitLab 地址</span>
              <input v-model="newProjectForm.gitlabUrl" required />
            </label>
            <label>
              <span>Project ID</span>
              <input v-model="newProjectForm.gitlabProjectId" required />
            </label>
            <label>
              <span>默认分支</span>
              <input v-model="newProjectForm.defaultBranch" required />
            </label>
            <label>
              <span>打包 Job</span>
              <input v-model="newProjectForm.packageJob" required />
            </label>
            <label>
              <span>部署 Job</span>
              <input v-model="newProjectForm.deployJob" required />
            </label>
            <label>
              <span>负责人</span>
              <input v-model="newProjectForm.owner" required />
            </label>
            <label>
              <span>排序</span>
              <input v-model.number="newProjectForm.sortOrder" type="number" />
            </label>
          </div>
          <div class="actions compact">
            <label class="checkbox-field">
              <input v-model="newProjectForm.enabled" type="checkbox" />
              <span>启用</span>
            </label>
            <button type="button" :disabled="loading" @click="cancelNewProjectForm">取消</button>
            <button class="primary" :disabled="loading" type="submit">保存新增</button>
          </div>
        </form>

        <div class="table-wrap config-table project-config-table">
          <table>
            <thead>
              <tr>
                <th>顺序</th>
                <th>项目</th>
                <th>类型 / 业务线</th>
                <th>GitLab 仓库</th>
                <th>默认分支 / Job</th>
                <th>负责人</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="project in orderedProjects" :key="project.code">
                <td>
                  <input
                    v-if="editingProjectCode === project.code"
                    v-model.number="projectDraft(project).sortOrder"
                    class="small-input"
                    type="number"
                  />
                  <span v-else>{{ project.sortOrder }}</span>
                </td>
                <td>
                  <template v-if="editingProjectCode === project.code">
                    <input v-model="projectDraft(project).name" />
                    <small>{{ project.code }}</small>
                  </template>
                  <template v-else>
                    <strong>{{ project.name }}</strong>
                    <small>{{ project.code }}</small>
                  </template>
                </td>
                <td class="stack-cell">
                  <template v-if="editingProjectCode === project.code">
                    <select v-model="projectDraft(project).kind">
                      <option value="backend">后端</option>
                      <option value="frontend">前端</option>
                    </select>
                    <select v-model="projectDraft(project).businessLineCode">
                      <option v-for="line in lines" :key="line.code" :value="line.code">{{ line.name }}</option>
                    </select>
                  </template>
                  <template v-else>
                    <span>{{ kindText[project.kind] }}</span>
                    <small>{{ project.businessLine?.name }}</small>
                  </template>
                </td>
                <td class="stack-cell">
                  <template v-if="editingProjectCode === project.code">
                    <input v-model="projectDraft(project).gitlabUrl" />
                    <input v-model="projectDraft(project).gitlabProjectId" />
                  </template>
                  <template v-else>
                    <span class="mono-line">{{ project.gitlabUrl }}</span>
                    <small>ID: {{ project.gitlabProjectId }}</small>
                  </template>
                </td>
                <td class="stack-cell">
                  <template v-if="editingProjectCode === project.code">
                    <input v-model="projectDraft(project).defaultBranch" />
                    <input v-model="projectDraft(project).packageJob" />
                    <input v-model="projectDraft(project).deployJob" />
                  </template>
                  <template v-else>
                    <span>{{ project.defaultBranch }}</span>
                    <small>{{ project.packageJob }} / {{ project.deployJob }}</small>
                  </template>
                </td>
                <td>
                  <input v-if="editingProjectCode === project.code" v-model="projectDraft(project).owner" />
                  <span v-else>{{ project.owner }}</span>
                </td>
                <td>
                  <label v-if="editingProjectCode === project.code" class="checkbox-field compact-check">
                    <input v-model="projectDraft(project).enabled" type="checkbox" />
                    <span>启用</span>
                  </label>
                  <span v-else class="status" :class="project.enabled ? 'deploy_success' : 'pending'">
                    {{ project.enabled ? "启用" : "停用" }}
                  </span>
                </td>
                <td class="inline-actions">
                  <template v-if="editingProjectCode === project.code">
                    <button class="primary" :disabled="loading" @click="saveProjectDraft(project)">保存</button>
                    <button :disabled="loading" @click="cancelProjectEdit(project.code)">取消</button>
                  </template>
                  <template v-else>
                    <button :disabled="loading" @click="startProjectEdit(project)">编辑</button>
                    <button class="danger-button" :disabled="loading" @click="deleteProject(project)">删除</button>
                  </template>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section v-if="activeTab === 'rules'" class="panel stack">
        <div class="section-head">
          <h2>业务线 Tag</h2>
          <button class="primary" :disabled="loading" @click="openNewLineForm">新增业务线</button>
        </div>

        <form v-if="showNewLineForm" class="config-form" @submit.prevent="createLineFromDraft">
          <div class="config-grid">
            <label>
              <span>业务线 Code</span>
              <input v-model="newLineForm.code" required />
            </label>
            <label>
              <span>业务线名称</span>
              <input v-model="newLineForm.name" required />
            </label>
            <label>
              <span>平台</span>
              <input v-model="newLineForm.platform" required />
            </label>
            <label>
              <span>Tag 前缀</span>
              <input v-model="newLineForm.tagPrefix" required />
            </label>
            <label class="wide">
              <span>Tag 模板</span>
              <input v-model="newLineForm.tagTemplate" required />
            </label>
            <label>
              <span>审批人</span>
              <input v-model="newLineForm.approver" required />
            </label>
          </div>
          <div class="actions compact">
            <button type="button" :disabled="loading" @click="cancelNewLineForm">取消</button>
            <button class="primary" :disabled="loading" type="submit">保存新增</button>
          </div>
        </form>

        <div class="table-wrap config-table">
          <table>
            <thead>
              <tr>
                <th>业务线</th>
                <th>平台</th>
                <th>Tag 前缀</th>
                <th>Tag 模板</th>
                <th>审批人</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <template v-for="line in lines" :key="line.code">
                <tr>
                  <td>
                    <template v-if="editingLineCode === line.code">
                      <input v-model="lineDraft(line).name" />
                      <small>{{ line.code }}</small>
                    </template>
                    <template v-else>
                      <strong>{{ line.name }}</strong>
                      <small>{{ line.code }}</small>
                    </template>
                  </td>
                  <td>
                    <input v-if="editingLineCode === line.code" v-model="lineDraft(line).platform" />
                    <span v-else>{{ line.platform }}</span>
                  </td>
                  <td>
                    <input v-if="editingLineCode === line.code" v-model="lineDraft(line).tagPrefix" />
                    <code v-else>{{ line.tagPrefix }}</code>
                  </td>
                  <td>
                    <input v-if="editingLineCode === line.code" v-model="lineDraft(line).tagTemplate" />
                    <code v-else>{{ line.tagTemplate }}</code>
                  </td>
                  <td>
                    <input v-if="editingLineCode === line.code" v-model="lineDraft(line).approver" />
                    <span v-else>{{ line.approver }}</span>
                  </td>
                  <td class="inline-actions">
                    <template v-if="editingLineCode === line.code">
                      <button class="primary" :disabled="loading" @click="saveLineDraft(line)">保存</button>
                      <button :disabled="loading" @click="cancelLineEdit(line.code)">取消</button>
                    </template>
                    <template v-else>
                      <button :disabled="loading" @click="startLineEdit(line)">编辑</button>
                      <button
                        class="danger-button"
                        :disabled="
                          loading || (lineUsageCount(line.code) > 0 && replacementLineOptions(line.code).length === 0)
                        "
                        @click="prepareLineDelete(line)"
                      >
                        删除
                      </button>
                    </template>
                  </td>
                </tr>
                <tr v-if="deletingLineCode === line.code" class="inline-confirm-row">
                  <td colspan="6">
                    <div class="inline-confirm">
                      <div>
                        <strong>迁移后删除</strong>
                        <small>{{ lineUsageCount(line.code) }} 个项目正在使用 {{ line.name }}</small>
                      </div>
                      <label>
                        <span>迁移到</span>
                        <select v-model="lineReplacementCodes[line.code]">
                          <option
                            v-for="option in replacementLineOptions(line.code)"
                            :key="option.code"
                            :value="option.code"
                          >
                            {{ option.name }}
                          </option>
                        </select>
                      </label>
                      <button class="danger-button" :disabled="loading" @click="deleteLine(line)">确认删除</button>
                      <button :disabled="loading" @click="cancelLineDelete(line.code)">取消</button>
                    </div>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>

        <div class="section-head">
          <h2>项目依赖顺序</h2>
          <button class="primary" :disabled="loading" @click="openNewDependencyForm">新增依赖关系</button>
        </div>

        <form v-if="showNewDependencyForm" class="config-form" @submit.prevent="createDependencyFromDraft">
          <div class="config-grid dependency-form-grid">
            <label>
              <span>项目</span>
              <select v-model="newDependencyForm.projectCode" required @change="syncNewDependencyCandidate">
                <option v-for="project in orderedProjects" :key="project.code" :value="project.code">
                  {{ project.name }}
                </option>
              </select>
            </label>
            <label>
              <span>依赖项目</span>
              <select
                v-model="newDependencyForm.dependencyCode"
                :disabled="availableDependenciesForProject(newDependencyForm.projectCode).length === 0"
                required
              >
                <option
                  v-for="candidate in availableDependenciesForProject(newDependencyForm.projectCode)"
                  :key="candidate.code"
                  :value="candidate.code"
                >
                  {{ candidate.name }}
                </option>
              </select>
            </label>
          </div>
          <div class="actions compact">
            <button type="button" :disabled="loading" @click="cancelNewDependencyForm">取消</button>
            <button class="primary" :disabled="loading || !newDependencyForm.dependencyCode" type="submit">
              保存新增
            </button>
          </div>
        </form>

        <div class="table-wrap config-table">
          <table>
            <thead>
              <tr>
                <th>顺序</th>
                <th>项目</th>
                <th>类型</th>
                <th>依赖项目 Code</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in dependencyRows" :key="row.key">
                <td>{{ row.project.sortOrder }}</td>
                <td>
                  <strong>{{ row.project.name }}</strong>
                  <small>{{ row.project.code }}</small>
                </td>
                <td>{{ kindText[row.project.kind] }}</td>
                <td>
                  <input v-if="row.isEditing" v-model="dependencyDrafts[row.project.code]" />
                  <span v-else-if="row.isEmpty">-</span>
                  <span v-else class="dependency-relation">
                    <code>{{ row.dependencyCode }}</code>
                    <small>{{ dependencyProjectName(row.dependencyCode) }}</small>
                  </span>
                </td>
                <td class="inline-actions">
                  <template v-if="row.isEditing">
                    <button class="primary" :disabled="loading" @click="saveDependencyDraft(row.project)">保存</button>
                    <button :disabled="loading" @click="cancelDependencyEdit(row.project.code)">取消</button>
                    <button class="danger-button" :disabled="loading" @click="clearDependencyDraft(row.project)">
                      清空
                    </button>
                  </template>
                  <template v-else>
                    <button :disabled="loading" @click="startDependencyEdit(row.project)">编辑</button>
                    <button
                      v-if="!row.isEmpty"
                      class="danger-button"
                      :disabled="loading"
                      @click="deleteDependency(row.project, row.dependencyCode)"
                    >
                      删除整行
                    </button>
                  </template>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section v-if="activeTab === 'users'" class="panel">
        <div class="section-head">
          <h2>用户权限</h2>
          <button class="primary" :disabled="loading" @click="openNewUserForm">新增用户</button>
        </div>

        <form v-if="showNewUserForm" class="config-form" @submit.prevent="createUserFromDraft">
          <div class="config-grid">
            <label>
              <span>用户名</span>
              <input v-model="newUserForm.username" required />
            </label>
            <label>
              <span>姓名</span>
              <input v-model="newUserForm.displayName" required />
            </label>
            <label>
              <span>角色</span>
              <select v-model="newUserForm.role">
                <option value="developer">开发</option>
                <option value="release_manager">发布经理</option>
                <option value="admin">管理员</option>
              </select>
            </label>
            <label>
              <span>状态</span>
              <select v-model="newUserForm.status">
                <option value="enabled">启用</option>
                <option value="disabled">禁用</option>
              </select>
            </label>
            <label>
              <span>初始密码</span>
              <input v-model="newUserForm.password" type="password" required />
            </label>
          </div>
          <div class="actions compact">
            <button type="button" :disabled="loading" @click="cancelNewUserForm">取消</button>
            <button class="primary" :disabled="loading" type="submit">保存新增</button>
          </div>
        </form>

        <div class="table-wrap config-table">
          <table>
            <thead>
              <tr>
                <th>用户名</th>
                <th>姓名</th>
                <th>角色</th>
                <th>状态</th>
                <th>重置密码</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in users" :key="item.id">
                <td>
                  <strong>{{ item.username }}</strong>
                  <small>ID: {{ item.id }}</small>
                </td>
                <td>
                  <input v-if="editingUserId === item.id" v-model="userDraft(item).displayName" />
                  <span v-else>{{ item.displayName }}</span>
                </td>
                <td>
                  <select v-if="editingUserId === item.id" v-model="userDraft(item).role">
                    <option value="developer">开发</option>
                    <option value="release_manager">发布经理</option>
                    <option value="admin">管理员</option>
                  </select>
                  <span v-else>{{ roleText[item.role] }}</span>
                </td>
                <td>
                  <select v-if="editingUserId === item.id" v-model="userDraft(item).status">
                    <option value="enabled">启用</option>
                    <option value="disabled">禁用</option>
                  </select>
                  <span v-else class="status" :class="item.status === 'enabled' ? 'deploy_success' : 'pending'">
                    {{ item.status === "enabled" ? "启用" : "禁用" }}
                  </span>
                </td>
                <td>
                  <input
                    v-if="editingUserId === item.id"
                    v-model="userDraft(item).password"
                    placeholder="留空不修改"
                    type="password"
                  />
                  <span v-else>-</span>
                </td>
                <td class="inline-actions">
                  <template v-if="editingUserId === item.id">
                    <button class="primary" :disabled="loading" @click="saveUserDraft(item)">保存</button>
                    <button :disabled="loading" @click="cancelUserEdit(item.id)">取消</button>
                  </template>
                  <template v-else>
                    <button :disabled="loading" @click="startUserEdit(item)">编辑</button>
                    <button class="danger-button" :disabled="loading || item.id === user.id" @click="deleteUser(item)">
                      删除
                    </button>
                  </template>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section v-if="activeTab === 'history'" class="panel history-layout">
        <div class="history-list">
          <article v-for="item in releases" :key="item.id" class="history-item">
            <button
              class="history-select"
              :class="{ active: selectedReleaseId === item.id }"
              @click="selectedReleaseId = item.id"
            >
              <strong>{{ item.batchNo }}</strong>
              <span>{{ statusLabel(item.status) }}</span>
            </button>
            <button class="danger-button" :disabled="!canOperate || loading" @click="deleteRelease(item)">删除</button>
          </article>
        </div>

        <div v-if="currentRelease" class="history-detail">
          <div class="section-head">
            <h2>{{ currentRelease.batchNo }}</h2>
            <div class="head-actions">
              <span class="status" :class="currentRelease.status">{{ statusLabel(currentRelease.status) }}</span>
              <button class="danger-button" :disabled="!canOperate || loading" @click="deleteRelease(currentRelease)">
                删除任务
              </button>
            </div>
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
