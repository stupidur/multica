import type { ReactNode } from "react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../locales/en/common.json";
import enSettings from "../../locales/en/settings.json";

const mockGetNotificationPreferences = vi.hoisted(() => vi.fn());
const mockMutate = vi.hoisted(() => vi.fn());

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-test",
}));

vi.mock("@multica/core/notification-preferences/queries", () => ({
  notificationPreferenceOptions: () => ({
    queryKey: ["notification-preferences", "ws-test"],
    queryFn: mockGetNotificationPreferences,
  }),
}));

vi.mock("@multica/core/notification-preferences/mutations", () => ({
  useUpdateNotificationPreferences: () => ({
    mutate: mockMutate,
  }),
}));

vi.mock("sonner", () => ({
  toast: { error: vi.fn() },
}));

import { NotificationsTab } from "./notifications-tab";

const TEST_RESOURCES = {
  en: { common: enCommon, settings: enSettings },
};

function Wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={new QueryClient()}>
      <I18nProvider locale="en" resources={TEST_RESOURCES}>
        {children}
      </I18nProvider>
    </QueryClientProvider>
  );
}

describe("NotificationsTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows the Feishu card section for Lark sessions", async () => {
    mockGetNotificationPreferences.mockResolvedValue({
      workspace_id: "ws-test",
      preferences: {},
      lark_card_notifications_available: true,
    });

    render(<NotificationsTab />, { wrapper: Wrapper });

    expect(await screen.findByText(/Feishu Interactive Cards/i)).toBeInTheDocument();
    expect(screen.getByText(/Enable Feishu interactive card notifications/i)).toBeInTheDocument();
  });

  it("hides the Feishu card section for non-Lark sessions", async () => {
    mockGetNotificationPreferences.mockResolvedValue({
      workspace_id: "ws-test",
      preferences: {},
      lark_card_notifications_available: false,
    });

    render(<NotificationsTab />, { wrapper: Wrapper });

    expect(await screen.findByRole("heading", { name: /Inbox Notifications/i })).toBeInTheDocument();
    expect(screen.queryByText(/Feishu Interactive Cards/i)).not.toBeInTheDocument();
  });
});
