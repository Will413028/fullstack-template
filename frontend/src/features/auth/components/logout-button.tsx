"use client";

import { LogOut } from "lucide-react";
import { useTranslations } from "next-intl";
import { useRouter } from "@/i18n/navigation";
import { useLogout } from "../hooks/use-auth";

export function LogoutButton() {
  const t = useTranslations("auth");
  const router = useRouter();
  const logout = useLogout();

  return (
    <button
      type="button"
      disabled={logout.isPending}
      // Signing out succeeds without a valid session too, so the login page is
      // the right place to land whatever the outcome.
      onClick={() =>
        logout.mutate(undefined, { onSettled: () => router.replace("/login") })
      }
      className="flex items-center gap-2 px-3 py-1.5 text-sm text-zinc-400 hover:text-white transition-colors disabled:opacity-50"
    >
      <LogOut size={16} aria-hidden="true" />
      {t("logout")}
    </button>
  );
}
