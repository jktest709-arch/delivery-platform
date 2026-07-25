"use client";

import { useMemo, useState } from "react";

type SourceMode = "branch" | "tag" | "commit";
type UserRole = "developer" | "releaseManager" | "admin";
type ProjectStatus =
  | "待构建"
  | "构建中"
  | "构建成功"
  | "构建失败"
  | "部署中"
  | "部署成功"
  | "重新排队";

type Project = {
  id: string;
  name: string;
  repo: string;
  line: string;
  owner: string;
  order: number;
  dependencies: string[];
  pipeline: string;
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

const RELEASE_DATE = "20260725";
const RELEASE_WINDOW = "2026-07-25 21:00";

const PROJECTS: Project[] = [
  {
    id: "base-auth",
    name: "统一认证中心",
    repo: "gitlab.corp/delivery/base-auth",
    line: "ops",
    owner: "平台组",
    order: 10,
    dependencies: [],
    pipeline: "build-auth-prd",
  },
  {
    id: "order-core",
    name: "订单核心服务",
    repo: "gitlab.corp/delivery/order-core",
    line: "aa",
    owner: "交易组",
    order: 20,
    dependencies: ["base-auth"],
    pipeline: "build-order-prd",
  },
  {
    id: "pay-gateway",
    name: "支付网关",
    repo: "gitlab.corp/delivery/pay-gateway",
    line: "aa",
    owner: "支付组",
    order: 30,
    dependencies: ["base-auth", "order-core"],
    pipeline: "build-pay-prd",
  },
  {
    id: "dispatch-engine",
    name: "履约调度引擎",
    repo: "gitlab.corp/delivery/dispatch-engine",
    line: "bb",
    owner: "履约组",
    order: 40,
    dependencies: ["order-core"],
    pipeline: "build-dispatch-prd",
  },
  {
    id: "merchant-portal",
    name: "商家工作台",
    repo: "gitlab.corp/delivery/merchant-portal",
    line: "bb",
    owner: "商家组",
    order: 50,
    dependencies: ["dispatch-engine"],
    pipeline: "build-portal-prd",
  },
  {
    id: "mobile-bff",
    name: "移动端 BFF",
    repo: "gitlab.corp/delivery/mobile-bff",
    line: "aa",
    owner: "无线组",
    order: 60,
    dependencies: ["order-core", "pay-gateway"],
    pipeline: "build-mobile-prd",
  },
  {
    id: "reporting",
    name: "运营报表中心",
    repo: "gitlab.corp/delivery/reporting",
    line: "ops",
    owner: "数据组",
    order: 70,
    dependencies: ["order-core", "dispatch-engine"],
    pipeline: "build-report-prd",
  },
];

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
    description: "可统一打 tag、触发构建和执行生产部署。",
    canTag: true,
    canDeploy: true,
    canManage: false,
    privileges: ["审核上线单", "统一打 tag", "触发构建", "生产部署"],
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
    acc[project.id] = project.id === "pay-gateway" ? "构建失败" : "待构建";
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
  待构建: "neutral",
  构建中: "info",
  构建成功: "success",
  构建失败: "danger",
  部署中: "info",
  部署成功: "success",
  重新排队: "warning",
};

function dependencyNames(project: Project) {
  return project.dependencies
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
  const [activity, setActivity] = useState([
    "支付网关上一次 pipeline 在 package 阶段失败，已保留单项目重发入口。",
    "系统按预设依赖顺序完成排序：认证 -> 订单 -> 支付 -> 履约 -> BFF。",
    "开发已提交 5 个项目的上线申请，等待发布经理统一打 tag。",
  ]);

  const permissions = ROLE_POLICIES[currentRole];
  const selectedProjects = useMemo(
    () =>
      PROJECTS.filter((project) => selectedProjectIds.includes(project.id)).sort(
        (a, b) => a.order - b.order,
      ),
    [selectedProjectIds],
  );

  const dependencyWarnings = selectedProjects.filter((project) =>
    project.dependencies.some((dependency) => !selectedProjectIds.includes(dependency)),
  );

  const completedCount = selectedProjects.filter(
    (project) => statuses[project.id] === "部署成功",
  ).length;
  const buildReadyCount = selectedProjects.filter((project) =>
    ["构建成功", "部署中", "部署成功"].includes(statuses[project.id]),
  ).length;

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

  function addActivity(message: string) {
    setActivity((current) => [message, ...current].slice(0, 5));
  }

  function submitRelease() {
    addActivity(
      `上线单已提交：${selectedProjects.length} 个项目，发布窗口 ${RELEASE_WINDOW}。`,
    );
  }

  function buildAll() {
    if (!permissions.canTag || selectedProjects.length === 0) return;
    const projectIds = selectedProjects.map((project) => project.id);
    setStatuses((current) => ({
      ...current,
      ...Object.fromEntries(projectIds.map((id) => [id, "构建中" as ProjectStatus])),
    }));
    addActivity(
      `已创建统一发布批次 PRD-${RELEASE_DATE}-${String(releaseNo).padStart(
        3,
        "0",
      )}，并按业务线前缀创建项目 tag。`,
    );
    window.setTimeout(() => {
      setStatuses((current) => ({
        ...current,
        ...Object.fromEntries(
          projectIds.map((id) => [id, "构建成功" as ProjectStatus]),
        ),
      }));
    }, 700);
  }

  function deployAll() {
    if (!permissions.canDeploy || selectedProjects.length === 0) return;
    const projectIds = selectedProjects.map((project) => project.id);
    setStatuses((current) => ({
      ...current,
      ...Object.fromEntries(projectIds.map((id) => [id, "部署中" as ProjectStatus])),
    }));
    addActivity("已触发生产部署，系统按依赖顺序串行推进 GitLab deploy jobs。");
    window.setTimeout(() => {
      setStatuses((current) => ({
        ...current,
        ...Object.fromEntries(
          projectIds.map((id) => [id, "部署成功" as ProjectStatus]),
        ),
      }));
    }, 700);
  }

  function runProjectAction(projectId: string, action: "build" | "deploy" | "retry") {
    const project = PROJECTS.find((item) => item.id === projectId);
    if (!project) return;

    if (action === "build") {
      setStatuses((current) => ({ ...current, [projectId]: "构建成功" }));
      addActivity(`${project.name} 已单独触发构建 job：${project.pipeline}。`);
      return;
    }

    if (action === "deploy") {
      setStatuses((current) => ({ ...current, [projectId]: "部署成功" }));
      addActivity(`${project.name} 已单独触发部署，用于补发或灰度后重放。`);
      return;
    }

    setStatuses((current) => ({ ...current, [projectId]: "重新排队" }));
    addActivity(`${project.name} 已按原始 ref 重新排队，保留同一个发布 tag。`);
    window.setTimeout(() => {
      setStatuses((current) => ({ ...current, [projectId]: "构建成功" }));
    }, 600);
  }

  return (
    <main className="delivery-shell">
      <section className="top-band" aria-labelledby="platform-title">
        <div>
          <p className="eyebrow">GitLab CI 统一发布入口</p>
          <h1 id="platform-title">统一交付平台</h1>
          <p className="summary">
            从上线申请、项目依赖排序、统一 tag、构建到部署，集中管理多项目生产发布。
          </p>
        </div>
        <div className="release-card" aria-label="当前发布批次">
          <span className="label">发布窗口</span>
          <strong>{RELEASE_WINDOW}</strong>
          <span>批次 PRD-{RELEASE_DATE}-{String(releaseNo).padStart(3, "0")}</span>
        </div>
      </section>

      <section className="pipeline-strip" aria-label="上线流程">
        {["上线申请", "依赖排序", "统一打 Tag", "GitLab 构建", "生产部署"].map(
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
              <p className="eyebrow">发布编排</p>
              <h2 id="release-title">上线单</h2>
            </div>
            <div className="release-metrics" aria-label="发布状态">
              <span>{selectedProjects.length} 个项目</span>
              <span>{buildReadyCount} 个已构建</span>
              <span>{completedCount} 个已部署</span>
            </div>
          </div>

          <div className="release-toolbar">
            <label>
              发布序号
              <input
                aria-label="发布序号"
                min="1"
                onChange={(event) =>
                  setReleaseNo(Math.max(1, Number(event.target.value) || 1))
                }
                type="number"
                value={releaseNo}
              />
            </label>
            <button onClick={submitRelease} type="button">
              提交上线流程
            </button>
            <button
              className="primary"
              disabled={!permissions.canTag || selectedProjects.length === 0}
              onClick={buildAll}
              type="button"
            >
              统一打 Tag 并构建
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
              : "依赖检查通过，系统已按预设顺序排序。"}
          </div>

          <div className="ordered-list" aria-label="已选项目发布顺序">
            {selectedProjects.map((project, index) => (
              <article className="ordered-item" key={project.id}>
                <div className="sequence">{String(index + 1).padStart(2, "0")}</div>
                <div className="project-main">
                  <div className="project-title-row">
                    <div>
                      <h3>{project.name}</h3>
                      <p>{project.repo}</p>
                    </div>
                    <span className={`status ${statusTone[statuses[project.id]]}`}>
                      {statuses[project.id]}
                    </span>
                  </div>
                  <div className="project-meta">
                    <span>{businessLines[project.line].platform}</span>
                    <span>{SOURCE_LABELS[sources[project.id].mode]}: {sources[project.id].ref}</span>
                    <span>Tag: {tagForProject(project)}</span>
                  </div>
                  <div className="dependency-line">
                    依赖: {dependencyNames(project) || "无前置项目"}
                  </div>
                </div>
                <div className="row-actions" aria-label={`${project.name} 单项目操作`}>
                  <button
                    disabled={!permissions.canTag}
                    onClick={() => runProjectAction(project.id, "build")}
                    type="button"
                  >
                    构建
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
        <section className="panel project-panel" aria-labelledby="project-title">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">项目入口</p>
              <h2 id="project-title">选择项目、分支或 Commit</h2>
            </div>
            <span className="count-pill">按 order 自动排序</span>
          </div>

          <div className="project-table" role="table" aria-label="项目选择表">
            <div className="table-head" role="row">
              <span>项目</span>
              <span>来源</span>
              <span>引用值</span>
              <span>业务线</span>
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
                      <small>{project.owner} · order {project.order}</small>
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
                  <span>{businessLines[project.line].name}</span>
                </div>
              );
            })}
          </div>
        </section>

        <section className="panel settings-panel" aria-labelledby="settings-title">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">业务线管理</p>
              <h2 id="settings-title">Tag 前缀配置</h2>
            </div>
            <span className="count-pill">{permissions.canManage ? "可编辑" : "展示模式"}</span>
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

          <div className="activity-feed" aria-label="操作记录">
            <h3>最近动作</h3>
            {activity.map((item) => (
              <p key={item}>{item}</p>
            ))}
          </div>
        </section>
      </section>
    </main>
  );
}
