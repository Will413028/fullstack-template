export default function OverviewPage() {
  return (
    <div>
      <h1 className="text-2xl font-medium text-white mb-6">Overview</h1>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        {[
          { label: "Total Revenue", value: "$0.00" },
          { label: "Orders", value: "0" },
          { label: "Customers", value: "0" },
        ].map((stat) => (
          <div
            key={stat.label}
            className="p-6 bg-zinc-900/50 border border-zinc-800 rounded-2xl"
          >
            <p className="text-sm text-zinc-500 mb-1">{stat.label}</p>
            <p className="text-2xl font-medium text-white">{stat.value}</p>
          </div>
        ))}
      </div>
    </div>
  );
}
