import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import PublicEventPage from "./page";

const getPublicEvent = vi.fn();
vi.mock("@/lib/api", () => ({ getPublicEvent: (...args: unknown[]) => getPublicEvent(...args) }));
vi.mock("qrcode.react", () => ({ QRCodeSVG: ({ value, title }: { value: string; title: string }) => <svg aria-label={title} data-value={value} /> }));

describe("Public Event page", () => {
  it("uses one trusted canonical URL for the link and QR", async () => {
    getPublicEvent.mockResolvedValue({ name: "Wedding", expiresAt: null, downloadsEnabled: false });
    const page = await PublicEventPage({ params: Promise.resolve({ publicToken: "opaque-token" }) });
    render(page);
    expect(screen.getByRole("heading", { name: "Wedding" })).toBeInTheDocument();
    expect(screen.getByRole("link")).toHaveAttribute("href", "http://localhost:3000/e/opaque-token");
    expect(screen.getByLabelText("Mã QR sự kiện")).toHaveAttribute("data-value", "http://localhost:3000/e/opaque-token");
  });

  it("shows the same unavailable state for rejected public tokens", async () => {
    getPublicEvent.mockRejectedValue(new Error("private detail"));
    const page = await PublicEventPage({ params: Promise.resolve({ publicToken: "unknown" }) });
    render(page);
    expect(screen.getByRole("heading", { name: "Sự kiện không khả dụng" })).toBeInTheDocument();
    expect(screen.queryByText("private detail")).not.toBeInTheDocument();
  });
});
