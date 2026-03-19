"use client";

import { useEffect } from "react";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error("[GlobalError]", error);
  }, [error]);

  return (
    <div className="min-h-screen bg-[#0a0a0b] flex items-center justify-center px-6">
      <div className="text-center max-w-md">
        <h1 className="text-2xl font-bold text-white mb-4">
          Something went wrong
        </h1>
        <p className="text-zinc-400 mb-6">
          An unexpected error occurred. Please try again.
        </p>
        <button
          type="button"
          onClick={reset}
          className="px-6 py-3 bg-white text-black font-medium rounded-lg hover:bg-zinc-100 transition-colors"
        >
          Try again
        </button>
      </div>
    </div>
  );
}
