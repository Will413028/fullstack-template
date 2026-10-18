import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: {
    default: "Fullstack Template",
    template: "%s | Fullstack Template",
  },
  description:
    "A production-ready fullstack template with Next.js, Go, and Docker Compose.",
  openGraph: {
    title: "Fullstack Template",
    description:
      "A production-ready fullstack template with Next.js, Go, and Docker Compose.",
    type: "website",
    locale: "en",
  },
  twitter: {
    card: "summary",
    title: "Fullstack Template",
    description:
      "A production-ready fullstack template with Next.js, Go, and Docker Compose.",
  },
  robots: {
    index: true,
    follow: true,
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return children;
}
