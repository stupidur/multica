import type { ReactNode } from "react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../locales/en/common.json";
import enSettings from "../../locales/en/settings.json";

const mockGetTenantSettings = vi.hoisted(() => vi.fn());
const mockGetTenantRole = vi.hoisted(() => vi.fn());
const mockPatchTenantSettings = vi.hoisted(() => vi.fn());

vi.mock("@multica/core/api", () => ({
  api: {
    getTenantSettings: mockGetTenantSettings,
    getTenantRole: mockGetTenantRole,
    patchTenantSettings: mockPatchTenantSettings,
  },
}));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

import { SystemTab } from "./system-tab";

const TEST_RESOURCES = {
  en: { common: enCommon, settings: enSettings },
};

function I18nWrapper({ children }: { children: ReactNode }) {
  return (
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      {children}
    </I18nProvider>
  );
}

describe("SystemTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetTenantSettings.mockResolvedValue({ title: "Tenant Title" });
    mockGetTenantRole.mockResolvedValue({ role: "owner" });
    mockPatchTenantSettings.mockResolvedValue({ title: "Tenant Title" });
  });

  it("checks tenant role from the Lark session and enables owner edits", async () => {
    render(<SystemTab />, { wrapper: I18nWrapper });

    expect(await screen.findByDisplayValue("Tenant Title")).toBeEnabled();
    expect(screen.getByRole("button", { name: /save/i })).toBeEnabled();
    expect(mockGetTenantRole).toHaveBeenCalledTimes(1);
  });

  it("falls back to read-only when tenant role cannot be loaded", async () => {
    mockGetTenantSettings.mockRejectedValue(new Error("not a lark session"));

    render(<SystemTab />, { wrapper: I18nWrapper });

    await waitFor(() => {
      expect(screen.getByLabelText(/page title/i)).toBeDisabled();
    });
    expect(screen.queryByRole("button", { name: /save/i })).not.toBeInTheDocument();
    expect(screen.getByText(/only admins can modify/i)).toBeInTheDocument();
  });
});
