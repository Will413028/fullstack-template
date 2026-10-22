"use client";

import { useTranslations } from "next-intl";
import { useState } from "react";
import { useRouter } from "@/i18n/navigation";
import { ApiError, problemMessage } from "@/lib/api/errors";
import { useDeleteAccount } from "../hooks/use-auth";

// Deletes the signed-in account and everything it owns after the user enters
// the password again. App stores require account deletion inside the app
// (Apple App Store Review Guideline 5.1.1(v), Google Play account deletion).
export function DeleteAccount({ account }: { account: string }) {
  const t = useTranslations("settings");
  const tp = useTranslations("problems");
  const router = useRouter();
  const remove = useDeleteAccount();
  const [password, setPassword] = useState("");

  let error: string | null = null;
  if (remove.error instanceof ApiError) {
    error = problemMessage(tp, remove.error.problem);
  } else if (remove.error) {
    error = tp("unknown");
  }

  return (
    <section className="p-6 border border-red-900/60 rounded-2xl space-y-4">
      <h2 className="text-lg font-medium text-white">{t("deleteTitle")}</h2>
      <p className="text-sm text-zinc-400">{t("deleteDescription")}</p>
      <form
        method="post"
        className="space-y-4"
        onSubmit={(event) => {
          event.preventDefault();
          remove.mutate(
            { account, password },
            { onSuccess: () => router.replace("/login") },
          );
        }}
      >
        <div>
          <label
            htmlFor="delete-password"
            className="block text-sm text-zinc-400 mb-1.5"
          >
            {t("deletePassword")}
          </label>
          <input
            id="delete-password"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            className="w-full px-4 py-2.5 bg-zinc-900 border border-zinc-700 rounded-lg text-white focus:outline-none focus:border-zinc-500 transition-colors"
          />
        </div>
        {error && (
          <p role="alert" className="text-sm text-red-400">
            {error}
          </p>
        )}
        <button
          type="submit"
          disabled={remove.isPending || password === ""}
          className="px-4 py-2.5 bg-red-600 hover:bg-red-500 text-white text-sm rounded-lg disabled:opacity-50 transition-colors"
        >
          {t("deleteConfirm")}
        </button>
      </form>
    </section>
  );
}
