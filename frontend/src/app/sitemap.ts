import type { MetadataRoute } from "next";
import { routing } from "@/i18n/routing";

export default function sitemap(): MetadataRoute.Sitemap {
  const baseUrl = process.env.NEXT_PUBLIC_APP_URL || "https://example.com";
  const pages = ["/", "/login"];

  return pages.flatMap((page) =>
    routing.locales.map((locale) => ({
      url: `${baseUrl}/${locale}${page === "/" ? "" : page}`,
      lastModified: new Date(),
      alternates: {
        languages: Object.fromEntries(
          routing.locales.map((l) => [
            l,
            `${baseUrl}/${l}${page === "/" ? "" : page}`,
          ]),
        ),
      },
    })),
  );
}
