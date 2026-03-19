"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { useForm } from "react-hook-form";
import { useRouter } from "@/i18n/navigation";
import { useLogin } from "../hooks/use-auth";
import { type LoginFormData, loginSchema } from "../validations";

export function LoginForm() {
  const t = useTranslations("auth");
  const router = useRouter();
  const searchParams = useSearchParams();
  const callbackUrl = searchParams.get("callbackUrl");
  const login = useLogin();
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
  });

  const onSubmit = (data: LoginFormData) => {
    login.mutate(data, {
      onSuccess: () => {
        const target = callbackUrl?.startsWith("/") ? callbackUrl : "/overview";
        router.push(target);
      },
    });
  };

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
      <div>
        <label htmlFor="account" className="block text-sm text-zinc-400 mb-1.5">
          {t("email")}
        </label>
        <input
          {...register("account")}
          type="text"
          id="account"
          autoComplete="username"
          className="w-full px-4 py-2.5 bg-zinc-900 border border-zinc-700 rounded-lg text-white placeholder:text-zinc-500 focus:outline-none focus:border-zinc-500 transition-colors"
          placeholder="your_account"
        />
        {errors.account && (
          <p className="mt-1 text-sm text-red-400">{errors.account.message}</p>
        )}
      </div>

      <div>
        <label
          htmlFor="password"
          className="block text-sm text-zinc-400 mb-1.5"
        >
          {t("password")}
        </label>
        <input
          {...register("password")}
          type="password"
          id="password"
          autoComplete="current-password"
          className="w-full px-4 py-2.5 bg-zinc-900 border border-zinc-700 rounded-lg text-white placeholder:text-zinc-500 focus:outline-none focus:border-zinc-500 transition-colors"
          placeholder="Enter your password"
        />
        {errors.password && (
          <p className="mt-1 text-sm text-red-400">{errors.password.message}</p>
        )}
      </div>

      {login.error && (
        <p className="text-sm text-red-400">
          {login.error instanceof Error ? login.error.message : "Login failed"}
        </p>
      )}

      <button
        type="submit"
        disabled={login.isPending}
        className="w-full py-3 bg-white text-black font-medium rounded-lg hover:bg-zinc-100 transition-colors disabled:opacity-50"
      >
        {login.isPending ? "..." : t("login")}
      </button>
    </form>
  );
}
