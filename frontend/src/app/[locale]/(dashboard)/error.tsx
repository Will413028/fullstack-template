"use client";

export default function DashboardError({
  error: _error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <div className="flex flex-col items-center justify-center min-h-[400px]">
      <h2 className="text-xl font-medium mb-4">Something went wrong</h2>
      <button
        type="button"
        onClick={reset}
        className="px-4 py-2 bg-white text-black rounded-lg"
      >
        Try again
      </button>
    </div>
  );
}
