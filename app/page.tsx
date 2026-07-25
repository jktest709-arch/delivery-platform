"use client";

import { useMemo, useState } from "react";

type SourceMode = "branch" | "tag" | "commit";
type UserRole = "developer" | "releaseManager" | "admin";
type ProjectKind = "backend" | "frontend";
type PackageTarget = "all" | ProjectKind;
type ReleaseHistoryStatus =
  | "待审批"
  | "打包中"
  | "打包完成"
  | "部署中"
  | "部署成功"
  | "部分失败";
type ProjectStatus =
  | "待打包"
  | "打包中"
  | "打包成功"
  | "打包失败"
  | "部署中"
  | "部署成功"
  | "重新排队";

type Project = {
  id: string;
  name: string;
  line: string;
  kind: ProjectKind;
  owner: string;
};

type SourceSelection = {
  mode: SourceMode;
  ref: string;
};

type BusinessLineConfig = {
  id: string;
  name: string;
  platform: string;
  prefix: string;
  template: string;
  approver: string;
};

type GitLabRepoConfig = {
  gitlabUrl: string;
  projectId: string;
  defaultBranch: string;
  packageJob: string;
};

type DependencyRule = {
  order: number;
  dependencies: string[];
};

type ReleaseHistoryProject = {
  name: string;
  kind: string;
  source: string;
  ref: string;
  repo: string;
  tag: string;
  state: string;
};

type ReleaseHistoryRecord = {
  id: string;
  batchNo: string;
  applicant: string;
  approver: string;
  status: ReleaseHistoryStatus;
  window: string;
  createdAt: string;
  updatedAt: string;
  projectCount: number;
  backendCount: number;
  frontendCount: number;
  tagSummary: string;
  summary: string;
  timeline: string[];
  projects: ReleaseHistoryProject[];
};

const RELEASE_DATE = "20260725";
const RELEASE_WINDOW = "2026-07-25 21:00";

const PROJECTS: Project[] = [
  {
    id: "base-auth",
    name: "统一认证中心",
    line: "ops",
    kind: "backend",
    owner: "平台组",
  },
  {
    id: "order-core",
    name: "订单核心服务",
    line: "aa",
    kind: "backend",
    owner: "交易组",
  },
  {
    id: "pay-gateway",
    name: "支付网关",
    line: "aa",
    kind: "backend",
    owner: "支付组",
  },
  {
    id: "dispatch-engine",
    name: "履约调度引擎",
    line: "bb",
    kind: "backend",
    owner: "履约组",
  },
  {
    id: "merchant-portal",
    name: "商家工作台",
    line: "bb",
    kind: "frontend",
    owner: "商家组",
  },
  {
    id: "mobile-bff",
    name: "移动端 BFF",
    line: "aa",
    kind: "backend",
    owner: "无线组",
  },
  {
    id: "reporting",
    name: "运营报表中心",
    line: "ops",
    kind: "frontend",
    owner: "数据组",
  },
];

const INITIAL_DEPENDENCY_RULES: Record<string, DependencyRule> = {
  "base-auth": {
    order: 10,
    dependencies: [],
  },
  "order-core": {
    order: 20,
    dependencies: ["base-auth"],
  },
  "pay-gateway": {
    order: 30,
    dependencies: ["base-auth", "order-core"],
  },
  "dispatch-engine": {
    order: 40,
    dependencies: ["order-core"],
  },
  "merchant-portal": {
    order: 50,
    dependencies: ["dispatch-engine"],
  },
  "mobile-bff": {
    order: 60,
    dependencies: ["order-core", "pay-gateway"],
  },
  reporting: {
    order: 70,
    dependencies: ["order-core", "dispatch-engine"],
  },
};

const INITIAL_REPO_CONFIGS: Record<string, GitLabRepoConfig> = {
  "base-auth": {
    gitlabUrl: "https://gitlab.corp/delivery/base-auth",
    projectId: "delivery/base-auth",
    defaultBranch: "master",
    packageJob: "build-auth-prd",
  },
  "order-core": {
    gitlabUrl: "https://gitlab.corp/delivery/order-core",
    projectId: "delivery/order-core",
    defaultBranch: "master",
    packageJob: "build-order-prd",
  },
  "pay-gateway": {
    gitlabUrl: "https://gitlab.corp/delivery/pay-gateway",
    projectId: "delivery/pay-gateway",
    defaultBranch: "master",
    packageJob: "build-pay-prd",
  },
  "dispatch-engine": {
    gitlabUrl: "https://gitlab.corp/delivery/dispatch-engine",
    projectId: "delivery/dispatch-engine",
    defaultBranch: "master",
    packageJob: "build-dispatch-prd",
  },
  "merchant-portal": {
    gitlabUrl: "https://gitlab.corp/delivery/merchant-portal",
    projectId: "delivery/merchant-portal",
    defaultBranch: "main",
    packageJob: "build-portal-prd",
  },
  "mobile-bff": {
    gitlabUrl: "https://gitlab.corp/delivery/mobile-bff",
    projectId: "delivery/mobile-bff",
    defaultBranch: "master",
    packageJob: "build-mobile-prd",
  },
  reporting: {
    gitlabUrl: "https://gitlab.corp/delivery/reporting",
    projectId: "delivery/reporting",
    defaultBranch: "main",
    packageJob: "build-report-prd",
  },
};

const INITIAL_LINES: Record<string, BusinessLineConfig> = {
  aa: {
    id: "aa",
    name: "AA 零售业务线",
    platform: "AAPRD",
    prefix: "aaprd",
    template: "{prefix}-{date}-{releaseNo}",
    approver: "交易发布经理",
  },
  bb: {
    id: "bb",
    name: "BB 履约业务线",
    platform: "BBPRD",
    prefix: "bbprd",
    template: "{prefix}-{date}-{releaseNo}",
    approver: "履约发布经理",
  },
  ops: {
    id: "ops",
    name: "OPS 平台业务线",
    platform: "OPSPRD",
    prefix: "opsprd",
    template: "{prefix}-{date}-{releaseNo}",
    approver: "平台 SRE",
  },
};

const ROLE_POLICIES: Record<
  UserRole,
  {
    label: string;
    description: string;
    canTag: boolean;
    canDeploy: boolean;
    canManage: boolean;
    privileges: string[];
  }
> = {
  developer: {
    label: "开发",
    description: "可发起上线单，选择项目、分支或 commit。",
    canTag: false,
    canDeploy: false,
    canManage: false,
    privileges: ["发起上线", "选择项目来源", "查看流水线"],
  },
  releaseManager: {
    label: "发布经理",
    description: "可统一打 tag、触发打包和执行生产部署。",
    canTag: true,
    canDeploy: true,
    canManage: false,
    privileges: ["审核上线单", "统一打 tag", "触发打包", "生产部署"],
  },
  admin: {
    label: "管理员",
    description: "可维护用户、业务线和项目依赖顺序。",
    canTag: true,
    canDeploy: true,
    canManage: true,
    privileges: ["用户管理", "业务线配置", "依赖编排", "所有发布操作"],
  },
};

const TEAM_MEMBERS = [
  { name: "林辰", role: "开发", scope: "AA 零售业务线", state: "可发起" },
  { name: "周岚", role: "发布经理", scope: "AA/BB 生产", state: "可审批" },
  { name: "高远", role: "管理员", scope: "全平台", state: "可配置" },
];

const INITIAL_SELECTED = [
  "base-auth",
  "order-core",
  "pay-gateway",
  "dispatch-engine",
  "merchant-portal",
  "mobile-bff",
];

const INITIAL_SOURCES: Record<string, SourceSelection> = PROJECTS.reduce(
  (acc, project) => {
    acc[project.id] = {
      mode: project.id === "pay-gateway" ? "commit" : "branch",
      ref: project.id === "pay-gateway" ? "8f34c91" : "release/2026.07",
    };
    return acc;
  },
  {} as Record<string, SourceSelection>,
);

const INITIAL_STATUS: Record<string, ProjectStatus> = PROJECTS.reduce(
  (acc, project) => {
    acc[project.id] = project.id === "pay-gateway" ? "打包失败" : "待打包";
    return acc;
  },
  {} as Record<string, ProjectStatus>,
);

const SOURCE_LABELS: Record<SourceMode, string> = {
  branch: "分支",
  tag: "Tag",
  commit: "Commit",
};

const statusTone: Record<ProjectStatus, string> = {
  待打包: "neutral",
  打包中: "info",
  打包成功: "success",
  打包失败: "danger",
  部署中: "info",
  部署成功: "success",
  重新排队: "warning",
};

const KIND_LABELS: Record<ProjectKind, string> = {
  backend: "后端",
  frontend: "前端",
};

const PACKAGE_TARGET_LABELS: Record<PackageTarget, string> = {
  all: "全量",
  backend: "后端",
  frontend: "前端",
};

const INITIAL_RELEASE_HISTORY: ReleaseHistoryRecord[] = [
  {
    id: "PRD-20260725-041",
    batchNo: "PRD-20260725-041",
    applicant: "林辰",
    approver: "周岚",
    status: "部署成功",
    window: "2026-07-24 21:00",
    createdAt: "2026-07-24 18:12",
    updatedAt: "2026-07-24 21:36",
    projectCount: 4,
    backendCount: 3,
    frontendCount: 1,
    tagSummary: "aaprd-20260724-041 / bbprd-20260724-041",
    summary: "订单核心服务、支付网关、履约调度引擎、商家工作台",
    timeline: [
      "2026-07-24 21:36 生产部署完成，发布批次关闭。",
      "2026-07-24 20:42 全量一键打包完成。",
      "2026-07-24 18:12 开发提交上线申请。",
    ],
    projects: [
      {
        name: "订单核心服务",
        kind: "后端",
        source: "分支",
        ref: "release/2026.07",
        repo: "https://gitlab.corp/delivery/order-core",
        tag: "aaprd-20260724-041",
        state: "部署成功",
      },
      {
        name: "支付网关",
        kind: "后端",
        source: "Commit",
        ref: "9c2d19f",
        repo: "https://gitlab.corp/delivery/pay-gateway",
        tag: "aaprd-20260724-041",
        state: "部署成功",
      },
      {
        name: "履约调度引擎",
        kind: "后端",
        source: "分支",
        ref: "release/2026.07",
        repo: "https://gitlab.corp/delivery/dispatch-engine",
        tag: "bbprd-20260724-041",
        state: "部署成功",
      },
      {
        name: "商家工作台",
        kind: "前端",
        source: "分支",
        ref: "release/2026.07",
        repo: "https://gitlab.corp/delivery/merchant-portal",
        tag: "bbprd-20260724-041",
        state: "部署成功",
      },
    ],
  },
  {
    id: "PRD-20260723-040",
    batchNo: "PRD-20260723-040",
    applicant: "赵悦",
    approver: "平台 SRE",
    status: "部分失败",
    window: "2026-07-23 22:00",
    createdAt: "2026-07-23 17:40",
    updatedAt: "2026-07-23 22:18",
    projectCount: 3,
    backendCount: 2,
    frontendCount: 1,
    tagSummary: "opsprd-20260723-040 / aaprd-20260723-040",
    summary: "统一认证中心、移动端 BFF、运营报表中心",
    timeline: [
      "2026-07-23 22:18 运营报表中心部署失败，等待补发。",
      "2026-07-23 21:20 后端打包完成。",
      "2026-07-23 17:40 开发提交上线申请。",
    ],
    projects: [
      {
        name: "统一认证中心",
        kind: "后端",
        source: "Tag",
        ref: "v2.18.0",
        repo: "https://gitlab.corp/delivery/base-auth",
        tag: "opsprd-20260723-040",
        state: "部署成功",
      },
      {
        name: "移动端 BFF",
        kind: "后端",
        source: "分支",
        ref: "release/2026.07",
        repo: "https://gitlab.corp/delivery/mobile-bff",
        tag: "aaprd-20260723-040",
        state: "部署成功",
      },
      {
        name: "运营报表中心",
        kind: "前端",
        source: "Commit",
        ref: "12ad7e0",
        repo: "https://gitlab.corp/delivery/reporting",
        tag: "opsprd-20260723-040",
        state: "部署失败",
      },
    ],
  },
];

function dependencyNames(projectId: string, rules: Record<string, DependencyRule>) {
  return rules[projectId].dependencies
    .map((id) => PROJECTS.find((item) => item.id === id)?.name)
    .filter(Boolean)
    .join("、");
}

export default function Home() {
  const [currentRole, setCurrentRole] = useState<UserRole>("releaseManager");
  const [releaseNo, setReleaseNo] = useState(42);
  const [selectedProjectIds, setSelectedProjectIds] =
    useState<string[]>(INITIAL_SELECTED);
  const [sources, setSources] = useState(INITIAL_SOURCES);
  const [statuses, setStatuses] = useState(INITIAL_STATUS);
  const [businessLines, setBusinessLines] = useState(INITIAL_LINES);
  const [repoConfigs, setRepoConfigs] = useState(INITIAL_REPO_CONFIGS);
  const [dependencyRules, setDependencyRules] = useState(INITIAL_DEPENDENCY_RULES);
  const [releaseHistory, setReleaseHistory] = useState(INITIAL_RELEASE_HISTORY);
  const [selectedHistoryId, setSelectedHistoryId] = useState(
    INITIAL_RELEASE_HISTORY[0].id,
  );
  const [activity, setActivity] = useState([
    "支付网关上一次 pipeline 在 package 阶段失败，已保留单项目重发入口。",
    "项目 GitLab 仓库地址由配置管理维护，上线单和执行台统一读取。",
    "打包依赖顺序由配置管理维护，发布申请只选择项目和代码来源。",
    "开发已提交 6 个项目的上线申请，等待发布经理一键打 tag 并打包。",
  ]);

  const permissions = ROLE_POLICIES[currentRole];
  const selectedProjects = useMemo(
    () =>
      PROJECTS.filter((project) => selectedProjectIds.includes(project.id)).sort(
        (a, b) => dependencyRules[a.id].order - dependencyRules[b.id].order,
      ),
    [dependencyRules, selectedProjectIds],
  );

  const selectedBackendProjects = selectedProjects.filter(
    (project) => project.kind === "backend",
  );
  const selectedFrontendProjects = selectedProjects.filter(
    (project) => project.kind === "frontend",
  );

  const dependencyWarnings = selectedProjects.filter((project) =>
    dependencyRules[project.id].dependencies.some(
      (dependency) => !selectedProjectIds.includes(dependency),
    ),
  );

  const completedCount = selectedProjects.filter(
    (project) => statuses[project.id] === "部署成功",
  ).length;
  const buildReadyCount = selectedProjects.filter((project) =>
    ["打包成功", "部署中", "部署成功"].includes(statuses[project.id]),
  ).length;
  const selectedHistory =
    releaseHistory.find((record) => record.id === selectedHistoryId) ??
    releaseHistory[0];

  function tagForProject(project: Project) {
    const config = businessLines[project.line];
    return `${config.prefix}-${RELEASE_DATE}-${String(releaseNo).padStart(3, "0")}`;
  }

  function toggleProject(projectId: string) {
    setSelectedProjectIds((current) =>
      current.includes(projectId)
        ? current.filter((item) => item !== projectId)
        : [...current, projectId],
    );
  }

  function updateSource(projectId: string, patch: Partial<SourceSelection>) {
    setSources((current) => ({
      ...current,
      [projectId]: { ...current[projectId], ...patch },
    }));
  }

  function updateBusinessLine(
    lineId: string,
    field: keyof Pick<BusinessLineConfig, "prefix" | "template" | "approver">,
    value: string,
  ) {
    setBusinessLines((current) => ({
      ...current,
      [lineId]: { ...current[lineId], [field]: value },
    }));
  }

  function updateRepoConfig(
    projectId: string,
    field: keyof GitLabRepoConfig,
    value: string,
  ) {
    setRepoConfigs((current) => ({
      ...current,
      [projectId]: {
        ...current[projectId],
        [field]: value,
      },
    }));
  }

  function updateDependencyOrder(projectId: string, order: number) {
    setDependencyRules((current) => ({
      ...current,
      [projectId]: {
        ...current[projectId],
        order,
      },
    }));
  }

  function updateDependencyList(projectId: string, value: string) {
    const dependencies = value
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean);
    setDependencyRules((current) => ({
      ...current,
      [projectId]: {
        ...current[projectId],
        dependencies,
      },
    }));
  }

  function addActivity(message: string) {
    setActivity((current) => [message, ...current].slice(0, 5));
  }

  function currentBatchNo() {
    return `PRD-${RELEASE_DATE}-${String(releaseNo).padStart(3, "0")}`;
  }

  function nowLabel() {
    return new Date().toLocaleString("zh-CN", {
      hour12: false,
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  }

  function historyProjects(
    stateForProject?: (project: Project) => string,
  ): ReleaseHistoryProject[] {
    return selectedProjects.map((project) => ({
      name: project.name,
      kind: KIND_LABELS[project.kind],
      source: SOURCE_LABELS[sources[project.id].mode],
      ref: sources[project.id].ref,
      repo: repoConfigs[project.id].gitlabUrl,
      tag: tagForProject(project),
      state: stateForProject?.(project) ?? statuses[project.id],
    }));
  }

  function updateReleaseHistory(
    status: ReleaseHistoryStatus,
    timelineEntry: string,
    stateForProject?: (project: Project) => string,
  ) {
    const batchNo = currentBatchNo();
    const timestamp = nowLabel();
    const projects = historyProjects(stateForProject);
    const tagSummary =
      Array.from(new Set(projects.map((project) => project.tag))).join(" / ") ||
      "未生成 tag";
    const nextRecord: ReleaseHistoryRecord = {
      id: batchNo,
      batchNo,
      applicant: "林辰",
      approver: "周岚",
      status,
      window: RELEASE_WINDOW,
      createdAt: timestamp,
      updatedAt: timestamp,
      projectCount: selectedProjects.length,
      backendCount: selectedBackendProjects.length,
      frontendCount: selectedFrontendProjects.length,
      tagSummary,
      summary: projects.map((project) => project.name).join("、"),
      timeline: [`${timestamp} ${timelineEntry}`],
      projects,
    };

    setReleaseHistory((current) => {
      const existing = current.find((record) => record.id === batchNo);
      if (!existing) {
        return [nextRecord, ...current].slice(0, 10);
      }

      return current.map((record) =>
        record.id === batchNo
          ? {
              ...record,
              ...nextRecord,
              createdAt: record.createdAt,
              timeline: [
                `${timestamp} ${timelineEntry}`,
                ...record.timeline,
              ].slice(0, 8),
            }
          : record,
      );
    });
    setSelectedHistoryId(batchNo);
  }

  function submitRelease() {
    addActivity(
      `上线单已提交：${selectedProjects.length} 个项目，发布窗口 ${RELEASE_WINDOW}，打包顺序读取配置管理。`,
    );
    updateReleaseHistory(
      "待审批",
      `上线申请已提交，包含 ${selectedProjects.length} 个项目。`,
    );
  }

  function packageProjects(target: PackageTarget) {
    const projects =
      target === "all"
        ? selectedProjects
        : selectedProjects.filter((project) => project.kind === target);

    if (!permissions.canTag || projects.length === 0) return;

    const projectIds = projects.map((project) => project.id);
    const queueNames = projects.map((project) => project.name).join(" -> ");

    setStatuses((current) => ({
      ...current,
      ...Object.fromEntries(
        projectIds.map((id, index) => [
          id,
          index === 0 ? "打包中" : ("重新排队" as ProjectStatus),
        ]),
      ),
    }));
    addActivity(
      `已创建统一 tag 批次 PRD-${RELEASE_DATE}-${String(releaseNo).padStart(
        3,
        "0",
      )}，${PACKAGE_TARGET_LABELS[target]}一键打包队列：${queueNames}，仓库地址读取 GitLab 配置。`,
    );
    updateReleaseHistory(
      "打包中",
      `${PACKAGE_TARGET_LABELS[target]}一键打包已触发：${queueNames}。`,
      (project) => (projectIds.includes(project.id) ? "打包中" : statuses[project.id]),
    );

    projectIds.forEach((projectId, index) => {
      window.setTimeout(() => {
        setStatuses((current) => ({
          ...current,
          ...(index > 0 ? { [projectIds[index - 1]]: "打包成功" as ProjectStatus } : {}),
          [projectId]: "打包中",
        }));
      }, index * 550);

      window.setTimeout(() => {
        setStatuses((current) => ({
          ...current,
          [projectId]: "打包成功",
        }));
      }, index * 550 + 420);
    });

    window.setTimeout(() => {
      updateReleaseHistory(
        "打包完成",
        `${PACKAGE_TARGET_LABELS[target]}一键打包完成。`,
        (project) =>
          projectIds.includes(project.id) ? "打包成功" : statuses[project.id],
      );
    }, projectIds.length * 550 + 520);
  }

  function deployAll() {
    if (!permissions.canDeploy || selectedProjects.length === 0) return;
    const projectIds = selectedProjects.map((project) => project.id);
    setStatuses((current) => ({
      ...current,
      ...Object.fromEntries(projectIds.map((id) => [id, "部署中" as ProjectStatus])),
    }));
    addActivity("已触发生产部署，系统按配置管理里的依赖顺序串行推进 GitLab deploy jobs。");
    updateReleaseHistory("部署中", "生产部署已触发。", () => "部署中");
    window.setTimeout(() => {
      setStatuses((current) => ({
        ...current,
        ...Object.fromEntries(
          projectIds.map((id) => [id, "部署成功" as ProjectStatus]),
        ),
      }));
      updateReleaseHistory("部署成功", "生产部署完成。", () => "部署成功");
    }, 700);
  }

  function runProjectAction(projectId: string, action: "build" | "deploy" | "retry") {
    const project = PROJECTS.find((item) => item.id === projectId);
    if (!project) return;

    if (action === "build") {
      setStatuses((current) => ({ ...current, [projectId]: "打包成功" }));
      addActivity(
        `${project.name} 已单独触发打包 job：${repoConfigs[project.id].packageJob}。`,
      );
      updateReleaseHistory(
        "打包完成",
        `${project.name} 单项目打包完成。`,
        (item) => (item.id === projectId ? "打包成功" : statuses[item.id]),
      );
      return;
    }

    if (action === "deploy") {
      setStatuses((current) => ({ ...current, [projectId]: "部署成功" }));
      addActivity(`${project.name} 已单独触发部署，用于补发或灰度后重放。`);
      updateReleaseHistory(
        "部署成功",
        `${project.name} 单项目部署完成。`,
        (item) => (item.id === projectId ? "部署成功" : statuses[item.id]),
      );
      return;
    }

    setStatuses((current) => ({ ...current, [projectId]: "重新排队" }));
    addActivity(`${project.name} 已按原始 ref 重新排队，保留同一个发布 tag。`);
    updateReleaseHistory(
      "打包中",
      `${project.name} 单项目重发已排队。`,
      (item) => (item.id === projectId ? "重新排队" : statuses[item.id]),
    );
    window.setTimeout(() => {
      setStatuses((current) => ({ ...current, [projectId]: "打包成功" }));
    }, 600);
  }

  return (
    <main className="delivery-shell">
      <section className="top-band" aria-labelledby="platform-title">
        <div>
          <p className="eyebrow">GitLab CI 统一发布入口</p>
          <h1 id="platform-title">统一交付平台</h1>
          <p className="summary">
            从上线申请、固定依赖配置、统一 tag、一键打包到部署，集中管理多项目生产发布。
          </p>
        </div>
        <div className="release-card" aria-label="当前发布批次">
          <span className="label">发布窗口</span>
          <strong>{RELEASE_WINDOW}</strong>
          <span>批次 PRD-{RELEASE_DATE}-{String(releaseNo).padStart(3, "0")}</span>
        </div>
      </section>

      <section className="pipeline-strip" aria-label="上线流程">
        {["上线申请", "依赖配置", "统一打 Tag", "一键打包", "生产部署"].map(
          (step, index) => (
            <div className="pipeline-step" key={step}>
              <span>{index + 1}</span>
              <strong>{step}</strong>
            </div>
          ),
        )}
      </section>

      <section className="control-grid">
        <aside className="panel role-panel" aria-labelledby="role-title">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">用户与权限</p>
              <h2 id="role-title">当前身份</h2>
            </div>
            <span className="count-pill">{permissions.label}</span>
          </div>

          <div className="role-switcher" aria-label="角色切换">
            {(Object.keys(ROLE_POLICIES) as UserRole[]).map((role) => (
              <button
                aria-pressed={currentRole === role}
                className={currentRole === role ? "is-active" : ""}
                key={role}
                onClick={() => setCurrentRole(role)}
                type="button"
              >
                {ROLE_POLICIES[role].label}
              </button>
            ))}
          </div>
          <p className="muted">{permissions.description}</p>
          <div className="privilege-list">
            {permissions.privileges.map((item) => (
              <span key={item}>{item}</span>
            ))}
          </div>

          <div className="member-list">
            {TEAM_MEMBERS.map((member) => (
              <div className="member-row" key={member.name}>
                <div>
                  <strong>{member.name}</strong>
                  <span>{member.scope}</span>
                </div>
                <span>{member.role}</span>
                <em>{member.state}</em>
              </div>
            ))}
          </div>
        </aside>

        <section className="panel release-panel" aria-labelledby="release-title">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">发布执行</p>
              <h2 id="release-title">打包部署执行台</h2>
            </div>
            <div className="release-metrics" aria-label="发布状态">
              <span>{selectedProjects.length} 个项目</span>
              <span>{buildReadyCount} 个已打包</span>
              <span>{completedCount} 个已部署</span>
            </div>
          </div>

          <div className="execution-note">
            当前执行批次 PRD-{RELEASE_DATE}-{String(releaseNo).padStart(3, "0")}
            ，来源于下方上线单申请；这里只处理统一 tag、打包和部署。
          </div>

          <div className="release-toolbar">
            <button
              className="primary"
              disabled={!permissions.canTag || selectedProjects.length === 0}
              onClick={() => packageProjects("all")}
              type="button"
            >
              一键打包全部
            </button>
            <button
              disabled={!permissions.canTag || selectedBackendProjects.length === 0}
              onClick={() => packageProjects("backend")}
              type="button"
            >
              后端一键打包
            </button>
            <button
              disabled={!permissions.canTag || selectedFrontendProjects.length === 0}
              onClick={() => packageProjects("frontend")}
              type="button"
            >
              前端一键打包
            </button>
            <button
              disabled={!permissions.canDeploy || selectedProjects.length === 0}
              onClick={deployAll}
              type="button"
            >
              按顺序部署
            </button>
          </div>

          <div className="dependency-alert" data-state={dependencyWarnings.length > 0 ? "warn" : "ok"}>
            {dependencyWarnings.length > 0
              ? `有 ${dependencyWarnings.length} 个项目缺少预设依赖，建议补选后再发布。`
              : "依赖检查通过，系统已读取配置管理中的固定打包顺序。"}
          </div>

          <div className="package-lanes" aria-label="前后端打包队列">
            <div>
              <strong>后端队列</strong>
              <span>
                {selectedBackendProjects.map((project) => project.name).join(" -> ") ||
                  "未选择后端项目"}
              </span>
            </div>
            <div>
              <strong>前端队列</strong>
              <span>
                {selectedFrontendProjects.map((project) => project.name).join(" -> ") ||
                  "未选择前端项目"}
              </span>
            </div>
          </div>

          <div className="ordered-list" aria-label="已选项目发布顺序">
            {selectedProjects.map((project, index) => (
              <article className="ordered-item" key={project.id}>
                <div className="sequence">{String(index + 1).padStart(2, "0")}</div>
                <div className="project-main">
                  <div className="project-title-row">
                    <div>
                      <h3>{project.name}</h3>
                      <p>{repoConfigs[project.id].gitlabUrl}</p>
                    </div>
                    <span className={`status ${statusTone[statuses[project.id]]}`}>
                      {statuses[project.id]}
                    </span>
                  </div>
                  <div className="project-meta">
                    <span>{businessLines[project.line].platform}</span>
                    <span>{KIND_LABELS[project.kind]}</span>
                    <span>GitLab ID: {repoConfigs[project.id].projectId}</span>
                    <span>固定顺序: {dependencyRules[project.id].order}</span>
                    <span>{SOURCE_LABELS[sources[project.id].mode]}: {sources[project.id].ref}</span>
                    <span>Tag: {tagForProject(project)}</span>
                  </div>
                  <div className="dependency-line">
                    依赖: {dependencyNames(project.id, dependencyRules) || "无前置项目"}
                  </div>
                </div>
                <div className="row-actions" aria-label={`${project.name} 单项目操作`}>
                  <button
                    disabled={!permissions.canTag}
                    onClick={() => runProjectAction(project.id, "build")}
                    type="button"
                  >
                    打包
                  </button>
                  <button
                    disabled={!permissions.canDeploy}
                    onClick={() => runProjectAction(project.id, "deploy")}
                    type="button"
                  >
                    部署
                  </button>
                  <button
                    disabled={!permissions.canTag}
                    onClick={() => runProjectAction(project.id, "retry")}
                    type="button"
                  >
                    重发
                  </button>
                </div>
              </article>
            ))}
          </div>
        </section>
      </section>

      <section className="workspace-grid">
        <section className="panel project-panel application-panel" aria-labelledby="project-title">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">上线单申请</p>
              <h2 id="project-title">选择项目、分支或 Commit</h2>
            </div>
            <span className="count-pill">申请提交入口</span>
          </div>

          <div className="application-toolbar">
            <label>
              发布序号
              <input
                aria-label="上线单发布序号"
                min="1"
                onChange={(event) =>
                  setReleaseNo(Math.max(1, Number(event.target.value) || 1))
                }
                type="number"
                value={releaseNo}
              />
            </label>
            <div className="application-summary">
              <strong>上线窗口</strong>
              <span>{RELEASE_WINDOW}</span>
            </div>
            <div className="application-summary">
              <strong>申请范围</strong>
              <span>
                {selectedProjects.length} 个项目，按配置顺序自动排序
              </span>
            </div>
            <button
              className="primary"
              disabled={selectedProjects.length === 0}
              onClick={submitRelease}
              type="button"
            >
              提交上线申请
            </button>
          </div>

          <div className="project-table" role="table" aria-label="项目选择表">
            <div className="table-head" role="row">
              <span>项目</span>
              <span>来源</span>
              <span>引用值</span>
              <span>类型 / 业务线</span>
            </div>
            {PROJECTS.map((project) => {
              const checked = selectedProjectIds.includes(project.id);
              return (
                <div className="table-row" data-selected={checked} key={project.id} role="row">
                  <label className="check-cell">
                    <input
                      checked={checked}
                      onChange={() => toggleProject(project.id)}
                      type="checkbox"
                    />
                    <span>
                      <strong>{project.name}</strong>
                      <small>
                        {project.owner} · 固定顺序 {dependencyRules[project.id].order}
                      </small>
                      <small>{repoConfigs[project.id].gitlabUrl}</small>
                    </span>
                  </label>
                  <select
                    aria-label={`${project.name} 来源类型`}
                    onChange={(event) =>
                      updateSource(project.id, {
                        mode: event.target.value as SourceMode,
                      })
                    }
                    value={sources[project.id].mode}
                  >
                    <option value="branch">分支</option>
                    <option value="tag">Tag</option>
                    <option value="commit">Commit</option>
                  </select>
                  <input
                    aria-label={`${project.name} 引用值`}
                    onChange={(event) =>
                      updateSource(project.id, { ref: event.target.value })
                    }
                    value={sources[project.id].ref}
                  />
                  <span>
                    {KIND_LABELS[project.kind]} · {businessLines[project.line].name}
                  </span>
                </div>
              );
            })}
          </div>
        </section>

        <section className="panel settings-panel" aria-labelledby="settings-title">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">配置管理</p>
              <h2 id="settings-title">GitLab、Tag 与依赖</h2>
            </div>
            <span className="count-pill">{permissions.canManage ? "可编辑" : "展示模式"}</span>
          </div>

          <div className="repo-configs" aria-label="GitLab 仓库配置">
            <div className="config-title-row">
              <div>
                <h3>GitLab 仓库配置</h3>
                <p>维护项目对应的仓库地址、项目 ID、默认分支和打包 Job。</p>
              </div>
              <span>{PROJECTS.length} 个仓库</span>
            </div>
            {PROJECTS.map((project) => (
              <div className="repo-config" key={project.id}>
                <div>
                  <strong>{project.name}</strong>
                  <span>{KIND_LABELS[project.kind]} · {project.id}</span>
                </div>
                <div className="repo-fields">
                  <label>
                    GitLab 仓库地址
                    <input
                      disabled={!permissions.canManage}
                      onChange={(event) =>
                        updateRepoConfig(project.id, "gitlabUrl", event.target.value)
                      }
                      value={repoConfigs[project.id].gitlabUrl}
                    />
                  </label>
                  <label>
                    GitLab Project ID
                    <input
                      disabled={!permissions.canManage}
                      onChange={(event) =>
                        updateRepoConfig(project.id, "projectId", event.target.value)
                      }
                      value={repoConfigs[project.id].projectId}
                    />
                  </label>
                  <label>
                    默认分支
                    <input
                      disabled={!permissions.canManage}
                      onChange={(event) =>
                        updateRepoConfig(project.id, "defaultBranch", event.target.value)
                      }
                      value={repoConfigs[project.id].defaultBranch}
                    />
                  </label>
                  <label>
                    打包 Job
                    <input
                      disabled={!permissions.canManage}
                      onChange={(event) =>
                        updateRepoConfig(project.id, "packageJob", event.target.value)
                      }
                      value={repoConfigs[project.id].packageJob}
                    />
                  </label>
                </div>
              </div>
            ))}
          </div>

          <div className="line-configs">
            {Object.values(businessLines).map((line) => (
              <div className="line-config" key={line.id}>
                <div>
                  <strong>{line.name}</strong>
                  <span>{line.platform}</span>
                </div>
                <label>
                  前缀
                  <input
                    disabled={!permissions.canManage}
                    onChange={(event) =>
                      updateBusinessLine(line.id, "prefix", event.target.value)
                    }
                    value={line.prefix}
                  />
                </label>
                <label>
                  模板
                  <input
                    disabled={!permissions.canManage}
                    onChange={(event) =>
                      updateBusinessLine(line.id, "template", event.target.value)
                    }
                    value={line.template}
                  />
                </label>
                <label>
                  审批人
                  <input
                    disabled={!permissions.canManage}
                    onChange={(event) =>
                      updateBusinessLine(line.id, "approver", event.target.value)
                    }
                    value={line.approver}
                  />
                </label>
              </div>
            ))}
          </div>

          <div className="dependency-configs" aria-label="项目打包依赖顺序配置">
            <div className="config-title-row">
              <div>
                <h3>项目打包依赖管理</h3>
                <p>管理员预先维护，发布申请只读取这里的顺序和依赖。</p>
              </div>
              <span>{PROJECTS.length} 个项目</span>
            </div>
            {[...PROJECTS]
              .sort((a, b) => dependencyRules[a.id].order - dependencyRules[b.id].order)
              .map((project) => (
                <div className="dependency-config" key={project.id}>
                  <div>
                    <strong>{project.name}</strong>
                    <span>{KIND_LABELS[project.kind]} · {project.id}</span>
                  </div>
                  <label>
                    顺序
                    <input
                      disabled={!permissions.canManage}
                      min="1"
                      onChange={(event) =>
                        updateDependencyOrder(
                          project.id,
                          Math.max(1, Number(event.target.value) || 1),
                        )
                      }
                      type="number"
                      value={dependencyRules[project.id].order}
                    />
                  </label>
                  <label>
                    前置依赖 ID
                    <input
                      disabled={!permissions.canManage}
                      onChange={(event) =>
                        updateDependencyList(project.id, event.target.value)
                      }
                      placeholder="base-auth, order-core"
                      value={dependencyRules[project.id].dependencies.join(", ")}
                    />
                  </label>
                </div>
              ))}
          </div>

          <div className="activity-feed" aria-label="操作记录">
            <h3>最近动作</h3>
            {activity.map((item) => (
              <p key={item}>{item}</p>
            ))}
          </div>
        </section>
      </section>

      <section className="panel history-panel" aria-labelledby="history-title">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">发布历史</p>
            <h2 id="history-title">上线批次记录</h2>
          </div>
          <div className="release-metrics" aria-label="历史统计">
            <span>{releaseHistory.length} 个批次</span>
            <span>{releaseHistory[0]?.status ?? "暂无记录"}</span>
          </div>
        </div>

        <div className="history-layout">
          <div className="history-list" aria-label="发布历史列表">
            <div className="history-head">
              <span>批次</span>
              <span>状态</span>
              <span>项目</span>
              <span>更新时间</span>
            </div>
            {releaseHistory.map((record) => (
              <button
                className={record.id === selectedHistory.id ? "history-row is-active" : "history-row"}
                key={record.id}
                onClick={() => setSelectedHistoryId(record.id)}
                type="button"
              >
                <span>
                  <strong>{record.batchNo}</strong>
                  <small>{record.window}</small>
                </span>
                <em data-status={record.status}>{record.status}</em>
                <span>{record.projectCount} 个</span>
                <span>{record.updatedAt}</span>
              </button>
            ))}
          </div>

          <aside className="history-detail" aria-label="发布历史详情">
            <div className="history-detail-top">
              <div>
                <h3>{selectedHistory.batchNo}</h3>
                <p>{selectedHistory.summary}</p>
              </div>
              <em data-status={selectedHistory.status}>{selectedHistory.status}</em>
            </div>

            <div className="history-facts">
              <div>
                <span>发起人</span>
                <strong>{selectedHistory.applicant}</strong>
              </div>
              <div>
                <span>审批人</span>
                <strong>{selectedHistory.approver}</strong>
              </div>
              <div>
                <span>项目分布</span>
                <strong>
                  后端 {selectedHistory.backendCount} / 前端 {selectedHistory.frontendCount}
                </strong>
              </div>
              <div>
                <span>Tag</span>
                <strong>{selectedHistory.tagSummary}</strong>
              </div>
            </div>

            <div className="history-project-table">
              <div className="history-project-head">
                <span>项目</span>
                <span>来源</span>
                <span>Tag</span>
                <span>状态</span>
              </div>
              {selectedHistory.projects.map((project) => (
                <div className="history-project-row" key={`${selectedHistory.id}-${project.name}`}>
                  <span>
                    <strong>{project.name}</strong>
                    <small>{project.kind} · {project.repo}</small>
                  </span>
                  <span>{project.source}: {project.ref}</span>
                  <span>{project.tag}</span>
                  <em>{project.state}</em>
                </div>
              ))}
            </div>

            <div className="history-timeline">
              <h3>时间线</h3>
              {selectedHistory.timeline.map((item) => (
                <p key={item}>{item}</p>
              ))}
            </div>
          </aside>
        </div>
      </section>
    </main>
  );
}
