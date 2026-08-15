import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AuthProvider, useAuth } from "@/components/providers/auth-provider";
import * as api from "@/lib/api";

vi.mock("@/lib/api", () => ({ refresh: vi.fn(), register: vi.fn(), login: vi.fn(), logout: vi.fn(), listOrganizations: vi.fn() }));

function Probe() {
  const auth = useAuth();
  if (auth.restoring) return <div>restoring</div>;
  return <div>
    <span>{auth.user?.email ?? "anonymous"}</span>
    <span>{auth.organizationsLoading ? "organizations-loading" : auth.organizationsError ? "organizations-error" : auth.currentOrganization?.organizationName ?? "no-organization"}</span>
    {auth.organizations.map((organization) => <button key={organization.organizationId} onClick={() => auth.selectOrganization(organization.organizationId)}>{organization.organizationName}</button>)}
    <button onClick={() => auth.logout()}>logout</button>
  </div>;
}

describe("AuthProvider", () => {
  beforeEach(() => vi.clearAllMocks());

  it("restores the current user once from the refresh cookie", async () => {
    vi.mocked(api.refresh).mockResolvedValue({ accessToken: "memory-token", tokenType: "Bearer", expiresIn: 900, user: { id: "user-1", email: "person@example.com", status: "active", createdAt: "2026-08-15T00:00:00Z" } });
    vi.mocked(api.listOrganizations).mockResolvedValue([]);
    render(<AuthProvider><Probe /></AuthProvider>);
    expect(screen.getByText("restoring")).toBeInTheDocument();
    expect(await screen.findByText("person@example.com")).toBeInTheDocument();
    expect(api.refresh).toHaveBeenCalledTimes(1);
  });

  it("becomes anonymous when refresh is rejected", async () => {
    vi.mocked(api.refresh).mockRejectedValue(new Error("rejected"));
    render(<AuthProvider><Probe /></AuthProvider>);
    await waitFor(() => expect(screen.getByText("anonymous")).toBeInTheDocument());
  });

  it("loads and switches organizations in memory", async () => {
    vi.mocked(api.refresh).mockResolvedValue({ accessToken: "memory-token", tokenType: "Bearer", expiresIn: 900, user: { id: "user-1", email: "person@example.com", status: "active", createdAt: "2026-08-15T00:00:00Z" } });
    vi.mocked(api.listOrganizations).mockResolvedValue([
      { organizationId: "org-1", organizationName: "Organization One", role: "owner" },
      { organizationId: "org-2", organizationName: "Organization Two", role: "viewer" },
    ]);
    render(<AuthProvider><Probe /></AuthProvider>);
    expect(await screen.findByText("Organization One", { selector: "span" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Organization Two" }));
    expect(screen.getByText("Organization Two", { selector: "span" })).toBeInTheDocument();
  });

  it("clears organization context on logout", async () => {
    vi.mocked(api.refresh).mockResolvedValue({ accessToken: "memory-token", tokenType: "Bearer", expiresIn: 900, user: { id: "user-1", email: "person@example.com", status: "active", createdAt: "2026-08-15T00:00:00Z" } });
    vi.mocked(api.listOrganizations).mockResolvedValue([{ organizationId: "org-1", organizationName: "Organization One", role: "editor" }]);
    vi.mocked(api.logout).mockResolvedValue(undefined);
    render(<AuthProvider><Probe /></AuthProvider>);
    expect(await screen.findByText("Organization One", { selector: "span" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "logout" }));
    await waitFor(() => expect(screen.getByText("no-organization")).toBeInTheDocument());
    expect(screen.getByText("anonymous")).toBeInTheDocument();
  });
});
