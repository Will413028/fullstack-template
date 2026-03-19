import { useTranslations } from "next-intl";

export default function Home() {
  const t = useTranslations("home");

  return (
    <main className="min-h-screen bg-[#0a0a0b]">
      <section className="flex flex-col items-center justify-center min-h-[80vh] px-6">
        <h1 className="text-4xl md:text-6xl font-bold text-white mb-4">
          {t("title")}
        </h1>
        <p className="text-lg text-zinc-400 max-w-2xl text-center">
          {t("description")}
        </p>
      </section>
    </main>
  );
}
