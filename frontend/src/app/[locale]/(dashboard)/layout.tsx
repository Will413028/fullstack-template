import { DashboardShell } from "@/components/layout/dashboard-shell";
import { LogoutButton } from "@/features/auth";
import { getServerUser } from "@/features/auth/server";
import { redirect } from "@/i18n/navigation";

export default async function DashboardLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;

  // Real auth gate: verify the session server-side before rendering protected
  // pages. proxy.ts only does a cheap cookie-presence redirect.
  const user = await getServerUser();
  if (!user) {
    redirect({ href: "/login", locale });
  }

  return (
    <DashboardShell
      actions={
        <>
          <span className="text-sm text-zinc-400">{user?.account}</span>
          <LogoutButton />
        </>
      }
    >
      {children}
    </DashboardShell>
  );
}
