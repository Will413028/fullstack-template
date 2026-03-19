export default function DashboardLoading() {
  return (
    <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
      {[1, 2, 3].map((i) => (
        <div
          key={i}
          className="h-24 bg-zinc-900/50 rounded-2xl animate-pulse"
        />
      ))}
    </div>
  );
}
