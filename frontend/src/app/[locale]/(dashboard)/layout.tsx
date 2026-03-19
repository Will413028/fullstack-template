"use client";

import {
  LayoutDashboard,
  Menu,
  Package,
  Settings,
  Users,
  X,
} from "lucide-react";
import { useState } from "react";
import { Link, usePathname } from "@/i18n/navigation";

const sidebarLinks = [
  { href: "/overview", label: "Overview", icon: LayoutDashboard },
  { href: "/products", label: "Products", icon: Package },
  { href: "/customers", label: "Customers", icon: Users },
  { href: "/settings", label: "Settings", icon: Settings },
];

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const pathname = usePathname();

  return (
    <div className="min-h-screen bg-[#0a0a0b] flex">
      {/* Sidebar */}
      <aside
        className={`fixed inset-y-0 left-0 z-50 w-64 bg-zinc-900/95 border-r border-zinc-800 transform transition-transform lg:relative lg:translate-x-0 ${
          sidebarOpen ? "translate-x-0" : "-translate-x-full"
        }`}
      >
        <div className="p-6">
          <Link href="/" className="text-lg font-semibold text-white">
            Dashboard
          </Link>
        </div>
        <nav className="px-3 space-y-1">
          {sidebarLinks.map((link) => {
            const isActive = pathname === link.href;
            return (
              <Link
                key={link.href}
                href={link.href}
                onClick={() => setSidebarOpen(false)}
                className={`flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm transition-colors ${
                  isActive
                    ? "bg-zinc-800 text-white"
                    : "text-zinc-400 hover:text-white hover:bg-zinc-800/50"
                }`}
              >
                <link.icon size={18} />
                {link.label}
              </Link>
            );
          })}
        </nav>
      </aside>

      {/* Overlay */}
      {sidebarOpen && (
        <button
          type="button"
          className="fixed inset-0 z-40 bg-black/50 lg:hidden"
          onClick={() => setSidebarOpen(false)}
          aria-label="Close sidebar overlay"
        />
      )}

      {/* Main content */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Top bar */}
        <header className="h-16 border-b border-zinc-800 flex items-center px-6 gap-4">
          <button
            type="button"
            className="lg:hidden p-2 text-zinc-400 hover:text-white"
            onClick={() => setSidebarOpen(!sidebarOpen)}
            aria-label={sidebarOpen ? "Close sidebar" : "Open sidebar"}
            aria-expanded={sidebarOpen}
          >
            {sidebarOpen ? (
              <X size={20} aria-hidden="true" />
            ) : (
              <Menu size={20} aria-hidden="true" />
            )}
          </button>
          <div className="flex-1" />
          <div className="w-8 h-8 rounded-full bg-zinc-700" />
        </header>

        {/* Page content */}
        <main className="flex-1 p-6">{children}</main>
      </div>
    </div>
  );
}
