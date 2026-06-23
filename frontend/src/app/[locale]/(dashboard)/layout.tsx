import { DashboardShell } from "@/components/layout/dashboard-shell";
import { redirect } from "@/i18n/navigation";
import { getServerUser } from "@/lib/server-auth";

export default async function DashboardLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;

  // Real auth gate: verify the session server-side before rendering protected
  // pages. The edge middleware only does a cheap cookie-presence redirect.
  const user = await getServerUser();
  if (!user) {
    redirect({ href: "/login", locale });
  }

  return <DashboardShell>{children}</DashboardShell>;
}
