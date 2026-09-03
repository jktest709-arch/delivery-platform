import type {
  BusinessLine,
  BusinessLinePayload,
  CreateReleasePayload,
  Project,
  ProjectPayload,
  Release,
  ReleaseTarget,
  SourceType,
  User,
  UserPayload,
} from "./types";

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? "/api";
let legacyBearerToken: string | null = null;
let sessionKnown = false;

export function getToken() {
  return legacyBearerToken;
}

export function setToken(token?: string) {
	// Authentication is maintained by the HttpOnly cookie. Keep this function
	// as a compatibility boundary for callers migrated from bearer storage.
	void token;
	legacyBearerToken = null;
	sessionKnown = true;
}

export function clearToken() {
	legacyBearerToken = null;
	sessionKnown = false;
}

export function hasSession() {
	return sessionKnown || document.cookie.split(";").some((cookie) => cookie.trim().startsWith("delivery-platform-session-present="));
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  headers.set("Content-Type", "application/json");
  const token = getToken();
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
    credentials: "include",
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.message ?? `HTTP ${response.status}`);
  }
  return payload as T;
}

export const api = {
  async login(username: string, password: string) {
    return request<{ token?: string; user: User }>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
  },
  logout() {
    return request<void>("/auth/logout", { method: "POST" });
  },
  me() {
    return request<User>("/me");
  },
  users() {
    return request<User[]>("/users");
  },
  createUser(user: UserPayload) {
    return request<User[]>("/users", {
      method: "POST",
      body: JSON.stringify(user),
    });
  },
  updateUser(id: number, user: UserPayload) {
    return request<User[]>(`/users/${id}`, {
      method: "PUT",
      body: JSON.stringify(user),
    });
  },
  deleteUser(id: number) {
    return request<User[]>(`/users/${id}`, { method: "DELETE" });
  },
  projects() {
    return request<Project[]>("/projects");
  },
  createProject(project: ProjectPayload) {
    return request<Project[]>("/projects", {
      method: "POST",
      body: JSON.stringify(project),
    });
  },
  updateProject(project: ProjectPayload) {
    return request<Project[]>(`/projects/${project.code}`, {
      method: "PUT",
      body: JSON.stringify(project),
    });
  },
  updateProjectOrder(codes: string[]) {
    return request<Project[]>("/projects/order", {
      method: "PUT",
      body: JSON.stringify({ codes }),
    });
  },
  deleteProject(code: string) {
    return request<Project[]>(`/projects/${code}`, { method: "DELETE" });
  },
  businessLines() {
    return request<BusinessLine[]>("/business-lines");
  },
  createBusinessLine(line: BusinessLinePayload) {
    return request<BusinessLine[]>("/business-lines", {
      method: "POST",
      body: JSON.stringify(line),
    });
  },
  updateBusinessLine(line: BusinessLinePayload) {
    return request<BusinessLine[]>(`/business-lines/${line.code}`, {
      method: "PUT",
      body: JSON.stringify(line),
    });
  },
  deleteBusinessLine(code: string, replacementCode = "") {
    const query = replacementCode ? `?replacementCode=${encodeURIComponent(replacementCode)}` : "";
    return request<BusinessLine[]>(`/business-lines/${code}${query}`, { method: "DELETE" });
  },
  updateDependencies(code: string, dependencies: string[]) {
    return request<Project[]>(`/dependencies/${code}`, {
      method: "PUT",
      body: JSON.stringify({ dependencies }),
    });
  },
  releases() {
    return request<Release[]>("/releases");
  },
  release(id: number) {
    return request<Release>(`/releases/${id}`);
  },
  jobTrace(releaseId: number, releaseProjectId: number, jobId: number) {
    return request<{ trace: string }>(`/releases/${releaseId}/projects/${releaseProjectId}/jobs/${jobId}/trace`);
  },
  createRelease(payload: CreateReleasePayload) {
    return request<Release>("/releases", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  },
  deleteRelease(id: number) {
    return request<Release[]>(`/releases/${id}`, { method: "DELETE" });
  },
  approveRelease(id: number) {
    return request<Release>(`/releases/${id}/approve`, { method: "POST" });
  },
  createTags(id: number, target: ReleaseTarget = "all", mode: "resume" | "restart" = "resume") {
    return request<Release>(`/releases/${id}/tag?target=${target}&mode=${mode}`, { method: "POST" });
  },
  packageRelease(id: number, target: ReleaseTarget) {
    return request<Release>(`/releases/${id}/package?target=${target}`, {
      method: "POST",
    });
  },
  deployRelease(id: number, target: ReleaseTarget) {
    return request<Release>(`/releases/${id}/deploy?target=${target}`, {
      method: "POST",
    });
  },
  packageProject(releaseId: number, releaseProjectId: number) {
    return request<Release>(`/releases/${releaseId}/projects/${releaseProjectId}/package`, {
      method: "POST",
    });
  },
  tagProject(releaseId: number, releaseProjectId: number) {
    return request<Release>(`/releases/${releaseId}/projects/${releaseProjectId}/tag`, {
      method: "POST",
    });
  },
  updateReleaseProjectSource(releaseId: number, releaseProjectId: number, payload: { sourceType: SourceType; sourceRef: string }) {
    return request<Release>(`/releases/${releaseId}/projects/${releaseProjectId}/source`, {
      method: "PUT",
      body: JSON.stringify(payload),
    });
  },
  deployProject(releaseId: number, releaseProjectId: number) {
    return request<Release>(`/releases/${releaseId}/projects/${releaseProjectId}/deploy`, {
      method: "POST",
    });
  },
};
