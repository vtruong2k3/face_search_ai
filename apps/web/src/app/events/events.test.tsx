import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import EventsPage from "./page";
import NewEventPage from "./new/page";

const push = vi.fn();
const listEvents = vi.fn();
const createEvent = vi.fn();
let role: "editor" | "viewer" = "editor";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));
vi.mock("@/components/providers/auth-provider", () => ({ useAuth: () => ({ user: { id: "user-1" }, restoring: false, organizationsLoading: false, currentOrganization: { organizationId: "org-1", organizationName: "Studio", role } }) }));
vi.mock("@/lib/api", async () => ({ listEvents: (...args: unknown[]) => listEvents(...args), createEvent: (...args: unknown[]) => createEvent(...args) }));

function renderPage(page: React.ReactNode) { const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } }); return render(<QueryClientProvider client={client}>{page}</QueryClientProvider>); }

describe("Event management", () => {
  beforeEach(() => { push.mockReset(); listEvents.mockReset(); createEvent.mockReset(); role = "editor"; });

  it("shows the tenant-scoped empty state", async () => {
    listEvents.mockResolvedValue([]);
    renderPage(<EventsPage />);
    expect(await screen.findByText("Chưa có sự kiện đang hoạt động.")).toBeInTheDocument();
    expect(listEvents).toHaveBeenCalledWith("org-1");
  });

  it("hides write controls from viewers", async () => {
    role = "viewer";
    listEvents.mockResolvedValue([]);
    renderPage(<EventsPage />);
    await screen.findByText("Chưa có sự kiện đang hoạt động.");
    expect(screen.queryByRole("link", { name: "Tạo sự kiện" })).not.toBeInTheDocument();
  });

  it("validates and creates an Event with server-generated navigation", async () => {
    createEvent.mockResolvedValue({ id: "event-1" });
    renderPage(<NewEventPage />);
    fireEvent.click(screen.getByRole("button", { name: "Tạo sự kiện" }));
    expect(await screen.findByText("Nhập tên sự kiện.")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Tên sự kiện"), { target: { value: "Wedding Day" } });
    fireEvent.click(screen.getByRole("button", { name: "Tạo sự kiện" }));
    await waitFor(() => expect(createEvent).toHaveBeenCalledWith("org-1", { name: "Wedding Day", visibility: "private", downloadsEnabled: false, expiresAt: null, matchThreshold: null }));
    expect(push).toHaveBeenCalledWith("/events/event-1");
  });
});
