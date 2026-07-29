export type Role = "developer" | "release_manager" | "admin";
export type ProjectKind = "backend" | "frontend";
export type SourceType = "branch" | "tag" | "commit";
export type ReleaseTarget = "all" | ProjectKind;

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
  packageJob: string;
  deployJob: string;
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
  targetTag: string;
  pipelineId: string;
  buildJobId: string;
  deployJobId: string;
  status: string;
  errorMessage: string;
  sortOrder: number;
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
  approver?: User;
  status: string;
  releaseWindow: string;
  remark: string;
  projects: ReleaseProject[];
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
};
