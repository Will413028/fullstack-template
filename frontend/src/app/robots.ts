import type { MetadataRoute } from "next";
import { appUrl } from "@/lib/env";

export default function robots(): MetadataRoute.Robots {
  const baseUrl = appUrl();

  return {
    rules: {
      userAgent: "*",
      allow: "/",
      disallow: ["/api/", "/overview/"],
    },
    sitemap: `${baseUrl}/sitemap.xml`,
  };
}
