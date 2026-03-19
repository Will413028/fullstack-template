interface PageTitleProps {
  label?: string;
  title: string;
  highlight?: string;
  description?: string;
}

export function PageTitle({
  label,
  title,
  highlight,
  description,
}: PageTitleProps) {
  return (
    <section className="px-6 pt-32 pb-16 max-w-6xl mx-auto">
      {label && (
        <p className="text-xs uppercase tracking-[0.2em] text-zinc-500 mb-4">
          {label}
        </p>
      )}
      <h1 className="text-4xl md:text-6xl font-medium leading-tight mb-6">
        {title}
        {highlight && (
          <>
            {" "}
            <span className="gradient-text-pink font-serif italic">
              {highlight}
            </span>
          </>
        )}
      </h1>
      {description && (
        <p className="text-xl text-zinc-400 max-w-2xl">{description}</p>
      )}
    </section>
  );
}
