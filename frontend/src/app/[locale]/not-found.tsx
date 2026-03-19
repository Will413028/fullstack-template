import { Link } from "@/i18n/navigation";

export default function NotFound() {
  return (
    <div className="min-h-screen bg-[#0a0a0b] flex items-center justify-center px-4">
      <div className="text-center">
        <h1 className="text-6xl font-medium text-white mb-4">404</h1>
        <p className="text-xl text-zinc-400 mb-8">Page not found</p>
        <Link
          href="/"
          className="px-6 py-3 bg-white text-black font-medium rounded-full hover:bg-zinc-100 transition-colors"
        >
          Go Home
        </Link>
      </div>
    </div>
  );
}
