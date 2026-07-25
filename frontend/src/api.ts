import type {
  BusinessLine,
  CreateReleasePayload,
  Project,
  Release,
  ReleaseTarget,
  User,
} from "./types";

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/api";
const TOKEN_KEY = "delivery-platform-token";

export function getToken() {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
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
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.message ?? `HTTP ${response.status}`);
  }
  return payload as T;
}

export const api = {
  async login(username: string, password: string) {
    return request<{ token: string; user: User }>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
  },
  me() {
    return request<User>("/me");
  },
  projects() {
    return request<Project[]>("/projects");
  },
  updateProject(project: Project) {
    return request<Project[]>(`/projects/${project.code}`, {
      method: "PUT",
      body: JSON.stringify(project),
    });
  },
  businessLines() {
    return request<BusinessLine[]>("/business-lines");
  },
  updateBusinessLine(line: BusinessLine) {
    return request<BusinessLine[]>(`/business-lines/${line.code}`, {
      method: "PUT",
      body: JSON.stringify(line),
    });
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
  createRelease(payload: CreateReleasePayload) {
    return request<Release>("/releases", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  },
  createTags(id: number) {
    return request<Release>(`/releases/${id}/tag`, { method: "POST" });
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
  deployProject(releaseId: number, releaseProjectId: number) {
    return request<Release>(`/releases/${releaseId}/projects/${releaseProjectId}/deploy`, {
      method: "POST",
    });
  },
};
