import { getTranslations } from "next-intl/server";
import { DeleteAccount } from "@/features/auth";
import { getServerUser } from "@/features/auth/server";

export default async function SettingsPage() {
  const t = await getTranslations("settings");
  // the (dashboard) layout has already sent anyone without a session to /login
  const user = await getServerUser();

  return (
    <div className="max-w-xl space-y-6">
      <h1 className="text-2xl font-medium text-white">{t("title")}</h1>
      {user && <DeleteAccount account={user.account} />}
    </div>
  );
}
