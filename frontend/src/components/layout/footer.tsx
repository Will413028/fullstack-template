"use client";

import { useTranslations } from "next-intl";
import { Link } from "@/i18n/navigation";

export default function Footer() {
  const t = useTranslations("footer");

  return (
    <footer className="px-6 py-16 border-t border-zinc-800">
      <div className="max-w-6xl mx-auto">
        <div className="flex flex-col md:flex-row items-center justify-between gap-4">
          <p className="text-sm text-zinc-500">
            © {new Date().getFullYear()}{" "}
            <span className="text-zinc-300">{t("brand")}</span>.{" "}
            {t("copyright")}
          </p>
          <div className="flex items-center gap-6">
            <Link
              href="/"
              className="text-sm text-zinc-400 hover:text-white transition-colors"
            >
              {t("home")}
            </Link>
          </div>
        </div>
      </div>
    </footer>
  );
}
