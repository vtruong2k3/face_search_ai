export type HealthResponse = {
  status: string;
  service: string;
};

import type { components } from "@/types/api.generated";

export type User = components["schemas"]["User"];
export type Credentials = components["schemas"]["Credentials"];
export type AuthResponse = components["schemas"]["AuthResponse"];
export type OrganizationMembership = components["schemas"]["OrganizationMembership"];
export type Event = components["schemas"]["Event"];
export type CreateEvent = components["schemas"]["CreateEvent"];
export type UpdateEvent = components["schemas"]["UpdateEvent"];
export type EventProcessingStatus = components["schemas"]["EventProcessingStatus"];
export type PublicEvent = components["schemas"]["PublicEvent"];

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
export async function listOrganizations(): Promise<OrganizationMembership[]> { const response = await authRequest("/organizations"); if (!response.ok) throw new Error("Organizations could not be loaded."); return response.json() as Promise<OrganizationMembership[]>; }

async function eventRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await authRequest(path, init);
  if (!response.ok) throw new Error(response.status === 400 ? "Event settings are invalid." : "Event request could not be completed.");
  return response.json() as Promise<T>;
}

export function listEvents(organizationId: string): Promise<Event[]> { return eventRequest(`/organizations/${organizationId}/events`); }
export function getEvent(organizationId: string, eventId: string): Promise<Event> { return eventRequest(`/organizations/${organizationId}/events/${eventId}`); }
export function createEvent(organizationId: string, input: CreateEvent): Promise<Event> { return eventRequest(`/organizations/${organizationId}/events`, { method: "POST", body: JSON.stringify(input) }); }
export function updateEvent(organizationId: string, eventId: string, input: UpdateEvent): Promise<Event> { return eventRequest(`/organizations/${organizationId}/events/${eventId}`, { method: "PATCH", body: JSON.stringify(input) }); }
export async function archiveEvent(organizationId: string, eventId: string): Promise<void> { const response = await authRequest(`/organizations/${organizationId}/events/${eventId}`, { method: "DELETE" }); if (!response.ok) throw new Error("Event request could not be completed."); }
export function getEventStatus(organizationId: string, eventId: string): Promise<EventProcessingStatus> { return eventRequest(`/organizations/${organizationId}/events/${eventId}/status`); }
export async function getPublicEvent(publicToken: string): Promise<PublicEvent> { const response = await fetch(`${apiBaseUrl}/api/v1/public/events/${encodeURIComponent(publicToken)}`, { cache: "no-store" }); if (!response.ok) throw new Error("Public Event is unavailable."); return response.json() as Promise<PublicEvent>; }

export async function getApiHealth(): Promise<HealthResponse> {
  const response = await fetch(`${apiBaseUrl}/health/ready`, {
    cache: "no-store",
  });
  if (!response.ok) {
    throw new Error(`API health check failed with status ${response.status}`);
  }
  return response.json() as Promise<HealthResponse>;
}
