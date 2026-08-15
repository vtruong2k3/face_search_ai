export type HealthResponse = {
  status: string;
  service: string;
};

import type { components } from "@/types/api.generated";

export type User = components["schemas"]["User"];
export type Credentials = components["schemas"]["Credentials"];
export type AuthResponse = components["schemas"]["AuthResponse"];

let accessToken: string | null = null;

const apiBaseUrl = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

async function authRequest(path: string, init: RequestInit = {}) {
  const headers = new Headers(init.headers);
  if (accessToken) headers.set("Authorization", `Bearer ${accessToken}`);
  if (init.body) headers.set("Content-Type", "application/json");
  return fetch(`${apiBaseUrl}/api/v1${path}`, { ...init, headers, credentials: "include", cache: "no-store" });
}

async function readAuth(response: Response): Promise<AuthResponse> {
  if (!response.ok) throw new Error("Authentication request rejected.");
  const result = (await response.json()) as AuthResponse;
  accessToken = result.accessToken;
  return result;
}

export function register(credentials: Credentials) { return authRequest("/auth/register", { method: "POST", body: JSON.stringify(credentials) }).then(readAuth); }
export function login(credentials: Credentials) { return authRequest("/auth/login", { method: "POST", body: JSON.stringify(credentials) }).then(readAuth); }
export function refresh() { return authRequest("/auth/refresh", { method: "POST" }).then(readAuth); }
export async function logout() { await authRequest("/auth/logout", { method: "POST" }); accessToken = null; }
export async function getCurrentUser(): Promise<User> { const response = await authRequest("/auth/me"); if (!response.ok) throw new Error("Authentication required."); return response.json() as Promise<User>; }


export async function getApiHealth(): Promise<HealthResponse> {
  const response = await fetch(`${apiBaseUrl}/health/ready`, {
    cache: "no-store",
  });
  if (!response.ok) {
    throw new Error(`API health check failed with status ${response.status}`);
  }
  return response.json() as Promise<HealthResponse>;
}
