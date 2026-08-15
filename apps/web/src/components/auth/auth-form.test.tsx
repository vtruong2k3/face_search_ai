import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AuthForm } from "@/components/auth/auth-form";

const push = vi.fn();
const login = vi.fn();
const registerUser = vi.fn();

vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));
vi.mock("@/components/providers/auth-provider", () => ({
  useAuth: () => ({ login, register: registerUser }),
}));

describe("AuthForm", () => {
  beforeEach(() => {
    push.mockReset();
    login.mockReset();
    registerUser.mockReset();
  });

  it("validates email and password before registration", async () => {
    render(<AuthForm mode="register" />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "invalid" } });
    fireEvent.change(screen.getByLabelText("Mật khẩu"), { target: { value: "short" } });
    fireEvent.click(screen.getByRole("button", { name: "Đăng ký" }));
    expect(await screen.findByText("Nhập địa chỉ email hợp lệ.")).toBeInTheDocument();
    expect(screen.getByText("Mật khẩu phải có ít nhất 12 ký tự.")).toBeInTheDocument();
    expect(registerUser).not.toHaveBeenCalled();
  });

  it("registers valid credentials and navigates to account", async () => {
    registerUser.mockResolvedValue(undefined);
    render(<AuthForm mode="register" />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "person@example.com" } });
    fireEvent.change(screen.getByLabelText("Mật khẩu"), { target: { value: "correct-horse-battery" } });
    fireEvent.click(screen.getByRole("button", { name: "Đăng ký" }));
    await waitFor(() => expect(registerUser).toHaveBeenCalledWith({ email: "person@example.com", password: "correct-horse-battery" }));
    expect(push).toHaveBeenCalledWith("/account");
  });

  it("shows a generic login error", async () => {
    login.mockRejectedValue(new Error("internal detail"));
    render(<AuthForm mode="login" />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "person@example.com" } });
    fireEvent.change(screen.getByLabelText("Mật khẩu"), { target: { value: "incorrect-password" } });
    fireEvent.click(screen.getByRole("button", { name: "Đăng nhập" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Yêu cầu xác thực bị từ chối.");
    expect(screen.queryByText("internal detail")).not.toBeInTheDocument();
  });
});
