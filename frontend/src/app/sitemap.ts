import type { MetadataRoute } from "next";
import { routing } from "@/i18n/routing";
import { appUrl } from "@/lib/env";

export default function sitemap(): MetadataRoute.Sitemap {
  const baseUrl = appUrl();
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
