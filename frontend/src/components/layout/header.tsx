"use client";

import { Globe, Menu, X } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useState } from "react";
import { Link, usePathname, useRouter } from "@/i18n/navigation";
import { routing } from "@/i18n/routing";

const localeLabels: Record<string, string> = {
  en: "EN",
  "zh-TW": "中文",
};

export default function Navigation() {
  const t = useTranslations("nav");

  const navLinks = [
    { href: "/", label: t("home") },
    { href: "/dashboard", label: t("dashboard") },
  ];
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [localeMenuOpen, setLocaleMenuOpen] = useState(false);
  const pathname = usePathname();
  const router = useRouter();
  const locale = useLocale();

  function switchLocale(nextLocale: string) {
    router.replace(pathname, { locale: nextLocale });
    setLocaleMenuOpen(false);
  }

  return (
    <nav className="fixed top-0 left-0 right-0 z-50 px-6 py-4">
      <div className="max-w-6xl mx-auto flex items-center justify-between">
        <Link href="/" className="text-xl font-bold text-white">
          App
        </Link>

        {/* Desktop Menu */}
        <div className="hidden md:flex items-center gap-0.5 bg-zinc-900/90 backdrop-blur-xl rounded-full px-1.5 py-1 border border-zinc-800/80">
          {navLinks.map((link) => (
            <Link
              key={link.href}
              href={link.href}
              className={`px-4 py-1.5 text-sm rounded-full transition-all duration-200 ${
                pathname === link.href
                  ? "text-white bg-zinc-800"
                  : "text-zinc-400 hover:text-white hover:bg-zinc-800/50"
              }`}
            >
              {link.label}
            </Link>
          ))}
        </div>

        {/* Right side */}
        <div className="hidden md:flex items-center gap-2">
          <div className="relative">
            <button
              type="button"
              onClick={() => setLocaleMenuOpen(!localeMenuOpen)}
              aria-label="Switch language"
              aria-expanded={localeMenuOpen}
              aria-haspopup="listbox"
              className="flex items-center gap-1.5 px-3 py-2 rounded-lg bg-zinc-900/80 border border-zinc-800 text-zinc-400 hover:text-white hover:border-zinc-700 transition-all duration-200 text-sm"
            >
              <Globe size={14} aria-hidden="true" />
              {localeLabels[locale] || locale}
            </button>
            {localeMenuOpen && (
              <div className="absolute right-0 top-full mt-1 bg-zinc-900 border border-zinc-800 rounded-lg shadow-xl overflow-hidden min-w-[100px]">
                {routing.locales.map((loc) => (
                  <button
                    key={loc}
                    type="button"
                    onClick={() => switchLocale(loc)}
                    className={`w-full px-4 py-2 text-left text-sm transition-colors ${
                      loc === locale
                        ? "text-white bg-zinc-800"
                        : "text-zinc-400 hover:text-white hover:bg-zinc-800"
                    }`}
                  >
                    {localeLabels[loc] || loc}
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Mobile menu button */}
        <button
          type="button"
          className="md:hidden p-2 text-zinc-400 hover:text-white transition-colors"
          onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
          aria-label={mobileMenuOpen ? "Close menu" : "Open menu"}
          aria-expanded={mobileMenuOpen}
        >
          {mobileMenuOpen ? (
            <X size={24} aria-hidden="true" />
          ) : (
            <Menu size={24} aria-hidden="true" />
          )}
        </button>
      </div>

      {/* Mobile Menu */}
      {mobileMenuOpen && (
        <div className="md:hidden absolute top-full left-0 right-0 bg-zinc-900/98 backdrop-blur-xl border-t border-zinc-800 p-6 shadow-2xl">
          <div className="flex flex-col gap-1">
            {navLinks.map((link) => (
              <Link
                key={link.href}
                href={link.href}
                onClick={() => setMobileMenuOpen(false)}
                className={`px-4 py-3 rounded-lg transition-colors ${
                  pathname === link.href
                    ? "text-white bg-zinc-800"
                    : "text-zinc-400 hover:text-white hover:bg-zinc-800/50"
                }`}
              >
                {link.label}
              </Link>
            ))}
          </div>
        </div>
      )}
    </nav>
  );
}
