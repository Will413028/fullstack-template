import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: {
    default: "template-go-nextjs",
    template: "%s | template-go-nextjs",
  },
  description:
    "A production-ready fullstack template with Next.js, Go, and Docker Compose.",
  openGraph: {
    title: "template-go-nextjs",
    description:
      "A production-ready fullstack template with Next.js, Go, and Docker Compose.",
    type: "website",
    locale: "en",
  },
  twitter: {
    card: "summary",
    title: "template-go-nextjs",
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
