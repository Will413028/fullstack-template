import { useTranslations } from "next-intl";
import { Suspense } from "react";
import { AuthForm } from "@/features/auth";

export default function LoginPage() {
  const t = useTranslations("auth");

  return (
    <div className="space-y-6">
      <div className="text-center">
        <h1 className="text-2xl font-medium text-white">{t("loginTitle")}</h1>
        <p className="text-zinc-400 mt-2">{t("loginSubtitle")}</p>
      </div>
      <div className="p-6 bg-zinc-900/50 border border-zinc-800 rounded-2xl">
        <Suspense fallback={<div className="h-64" />}>
          <AuthForm mode="login" />
        </Suspense>
      </div>
    </div>
  );
}
