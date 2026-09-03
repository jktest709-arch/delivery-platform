export type Role = "developer" | "release_manager" | "admin";
export type ProjectKind = "backend" | "frontend";
export type SourceType = "branch" | "tag" | "commit";
export type ReleaseTarget = "all" | ProjectKind;
export type ReleaseChangeType = "db" | "nacos" | "xxl_job" | "admin_op";
export type RiskLevel = "low" | "medium" | "high";

export type User = {
  id: number;
  username: string;
  displayName: string;
  role: Role;
  status: string;
};

export type UserPayload = {
  username: string;
  displayName: string;
  role: Role;
  status: string;
  password?: string;
};

export type BusinessLine = {
  id: number;
  code: string;
  name: string;
  platform: string;
  tagPrefix: string;
  tagTemplate: string;
  approver: string;
};

export type BusinessLinePayload = Omit<BusinessLine, "id">;

export type Project = {
  id: number;
  code: string;
  name: string;
  kind: ProjectKind;
  owner: string;
  businessLineCode: string;
  businessLine: BusinessLine;
  businessLineCodes?: string[];
  businessLines?: BusinessLine[];
  gitlabUrl: string;
  gitlabProjectId: string;
  defaultBranch: string;
  sortOrder: number;
  enabled: boolean;
  dependencies: string[] | null;
};

export type ProjectPayload = Omit<Project, "id" | "businessLine" | "businessLines" | "dependencies">;

export type ReleaseProject = {
  id: number;
  releaseId: number;
  projectId: number;
  project: Project;
  businessLineId?: number;
  businessLine?: BusinessLine;
  sourceType: SourceType;
  sourceRef: string;
  sourceDirty: boolean;
  targetTag: string;
  pipelineId: string;
  buildJobId: string;
  deployJobId: string;
  jobs: PipelineJob[];
  status: string;
  errorMessage: string;
  sortOrder: number;
};

export type ReleaseChange = {
  id: number;
  releaseId: number;
  type: ReleaseChangeType;
  title: string;
  status: string;
  riskLevel: RiskLevel;
  contentJson: string;
  createdById: number;
  createdBy?: User;
  createdAt: string;
  updatedAt: string;
};

export type PipelineJob = {
  id: number;
  releaseProjectId: number;
  gitlabJobId: string;
  name: string;
  stage: string;
  status: string;
  webUrl: string;
  manual: boolean;
  allowFailure: boolean;
};

export type ReleaseEvent = {
  id: number;
  releaseId: number;
  operator?: User;
  action: string;
  message: string;
  createdAt: string;
};

export type Release = {
  id: number;
  batchNo: string;
  applicant: User;
  businessLineId?: number;
  businessLine?: BusinessLine;
  approverId?: number | null;
  approver?: User;
  approvedAt?: string | null;
  status: string;
  releaseWindow: string;
  remark: string;
  projects: ReleaseProject[];
  changes: ReleaseChange[];
  events: ReleaseEvent[];
  createdAt: string;
  updatedAt: string;
};

export type CreateReleasePayload = {
  businessLineCode: string;
  releaseWindow: string;
  remark: string;
  projects: Array<{
    projectCode: string;
    businessLineCode?: string;
    sourceType: SourceType;
    sourceRef: string;
  }>;
  changes: Array<{
    type: ReleaseChangeType;
    title: string;
    riskLevel: RiskLevel;
    content: Record<string, unknown>;
  }>;
};
