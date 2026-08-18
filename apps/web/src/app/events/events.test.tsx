import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import EventsPage from "./page";
import EventDetailPage from "./[eventId]/page";
import NewEventPage from "./new/page";

const push = vi.fn();
const listEvents = vi.fn();
const createEvent = vi.fn();
const getEvent = vi.fn();
const getEventStatus = vi.fn();
const listPhotos = vi.fn();
const reprocessPhoto = vi.fn();
let role: "editor" | "viewer" = "editor";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push }), useParams: () => ({ eventId: "event-1" }) }));
vi.mock("@/components/providers/auth-provider", () => ({ useAuth: () => ({ user: { id: "user-1" }, restoring: false, organizationsLoading: false, currentOrganization: { organizationId: "org-1", organizationName: "Studio", role } }) }));
vi.mock("@/lib/api", async () => ({ listEvents: (...args: unknown[]) => listEvents(...args), createEvent: (...args: unknown[]) => createEvent(...args), getEvent: (...args: unknown[]) => getEvent(...args), getEventStatus: (...args: unknown[]) => getEventStatus(...args), listPhotos: (...args: unknown[]) => listPhotos(...args), reprocessPhoto: (...args: unknown[]) => reprocessPhoto(...args) }));
vi.mock("./[eventId]/photo-uploader", () => ({ PhotoUploader: () => <div data-testid="photo-uploader" /> }));

const event = { id: "event-1", name: "Wedding Day", visibility: "private", downloadsEnabled: false, expiresAt: null };

const status = { eventId: "event-1", activeTotal: 2, pending: 0, uploading: 0, uploaded: 0, queued: 1, processing: 0, ready: 0, failed: 1, deleted: 0 };
const failedPhoto = { id: "photo-1", eventId: "event-1", originalFilename: "broken.jpg", contentType: "image/jpeg", byteSize: 10, status: "failed", createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z" };

function renderPage(page: React.ReactNode) { const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } }); return render(<QueryClientProvider client={client}>{page}</QueryClientProvider>); }

function mockDetail() { getEvent.mockResolvedValue(event); getEventStatus.mockResolvedValue(status); listPhotos.mockResolvedValue([failedPhoto]); }

describe("Event management", () => {
  beforeEach(() => { push.mockReset(); listEvents.mockReset(); createEvent.mockReset(); getEvent.mockReset(); getEventStatus.mockReset(); listPhotos.mockReset(); reprocessPhoto.mockReset(); role = "editor"; });

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

  it("renders trusted processing counters and retries failed photos", async () => {
    mockDetail();
    reprocessPhoto.mockResolvedValue({ ...failedPhoto, status: "queued" });
    renderPage(<EventDetailPage />);
    expect(await screen.findByText("Wedding Day")).toBeInTheDocument();
    expect(screen.getByText("Đang chờ")).toBeInTheDocument();
    expect(screen.getByText("Đang xử lý")).toBeInTheDocument();
    expect(screen.getByText("Sẵn sàng")).toBeInTheDocument();
    expect(screen.getByText("Lỗi")).toBeInTheDocument();
    expect(screen.getByText("broken.jpg")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
    await waitFor(() => expect(reprocessPhoto).toHaveBeenCalledWith("org-1", "event-1", "photo-1"));
  });

  it("does not expose retry controls to viewers", async () => {
    role = "viewer";
    mockDetail();
    renderPage(<EventDetailPage />);
    await screen.findByText("broken.jpg");
    expect(screen.queryByRole("button", { name: "Thử lại" })).not.toBeInTheDocument();
  });
});
