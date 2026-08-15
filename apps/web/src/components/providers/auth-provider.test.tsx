import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AuthProvider, useAuth } from "@/components/providers/auth-provider";
import * as api from "@/lib/api";

vi.mock("@/lib/api", () => ({ refresh: vi.fn(), register: vi.fn(), login: vi.fn(), logout: vi.fn() }));

function Probe() {
  const auth = useAuth();
  return <div>{auth.restoring ? "restoring" : auth.user?.email ?? "anonymous"}</div>;
}

describe("AuthProvider", () => {
  beforeEach(() => vi.clearAllMocks());

  it("restores the current user once from the refresh cookie", async () => {
    vi.mocked(api.refresh).mockResolvedValue({ accessToken: "memory-token", tokenType: "Bearer", expiresIn: 900, user: { id: "user-1", email: "person@example.com", status: "active", createdAt: "2026-08-15T00:00:00Z" } });
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
});
