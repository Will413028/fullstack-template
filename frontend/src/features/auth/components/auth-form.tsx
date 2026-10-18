"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { useForm } from "react-hook-form";
import { Link, useRouter } from "@/i18n/navigation";
import {
  ApiError,
  fieldErrors,
  fieldMessage,
  problemMessage,
} from "@/lib/api/errors";
import { useLogin, useRegister } from "../hooks/use-auth";
import { type CredentialsFormData, credentialsSchema } from "../validations";

const fieldNames = ["account", "password"] as const;

// Sign in (mode "login") or create an account and sign in to it ("register").
export function AuthForm({ mode }: { mode: "login" | "register" }) {
  const t = useTranslations("auth");
  const tp = useTranslations("problems");
  const router = useRouter();
  const searchParams = useSearchParams();
  const callbackUrl = searchParams.get("callbackUrl");
  const login = useLogin();
  const signUp = useRegister();
  const submit = mode === "login" ? login : signUp;
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors },
  } = useForm<CredentialsFormData>({
    resolver: zodResolver(credentialsSchema),
  });

  const onSubmit = (data: CredentialsFormData) => {
    submit.mutate(data, {
      onSuccess: () => {
        // Only allow local paths; reject protocol-relative "//evil.com" redirects.
        const isSafe =
          !!callbackUrl &&
          callbackUrl.startsWith("/") &&
          !callbackUrl.startsWith("//");
        router.push(isSafe ? callbackUrl : "/overview");
      },
      onError: (error) => {
        if (!(error instanceof ApiError)) return;
        for (const [name, fieldError] of fieldErrors(error.problem)) {
          const field = fieldNames.find((f) => f === name);
          if (field) {
            setError(field, { type: "server", message: fieldError.code });
          }
        }
      },
    });
  };

  // A problem that names fields is shown next to them; anything else here.
  let formError: string | null = null;
  if (submit.error instanceof ApiError) {
    if (fieldErrors(submit.error.problem).size === 0) {
      formError = problemMessage(tp, submit.error.problem);
    }
  } else if (submit.error) {
    formError = tp("unknown");
  }

  const inputClass =
    "w-full px-4 py-2.5 bg-zinc-900 border border-zinc-700 rounded-lg text-white placeholder:text-zinc-500 focus:outline-none focus:border-zinc-500 transition-colors";

  return (
    // method="post": should the page ever submit without JavaScript, the
    // password must not end up in the URL (history, proxy logs).
    <form
      method="post"
      onSubmit={handleSubmit(onSubmit)}
      className="space-y-4"
      noValidate
    >
      <div>
        <label htmlFor="account" className="block text-sm text-zinc-400 mb-1.5">
          {t("account")}
        </label>
        <input
          {...register("account")}
          type="text"
          id="account"
          autoComplete="username"
          aria-invalid={!!errors.account}
          className={inputClass}
          placeholder="your_account"
        />
        {errors.account?.message && (
          <p className="mt-1 text-sm text-red-400">
            {fieldMessage(tp, errors.account.message)}
          </p>
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
          autoComplete={mode === "login" ? "current-password" : "new-password"}
          aria-invalid={!!errors.password}
          className={inputClass}
        />
        {errors.password?.message && (
          <p className="mt-1 text-sm text-red-400">
            {fieldMessage(tp, errors.password.message)}
          </p>
        )}
      </div>

      {formError && (
        <p role="alert" className="text-sm text-red-400">
          {formError}
        </p>
      )}

      <button
        type="submit"
        disabled={submit.isPending}
        className="w-full py-3 bg-white text-black font-medium rounded-lg hover:bg-zinc-100 transition-colors disabled:opacity-50"
      >
        {submit.isPending ? "..." : t(mode)}
      </button>

      <p className="text-center text-sm text-zinc-400">
        {mode === "login" ? t("noAccount") : t("hasAccount")}{" "}
        <Link
          href={mode === "login" ? "/register" : "/login"}
          className="text-white hover:underline"
        >
          {mode === "login" ? t("register") : t("login")}
        </Link>
      </p>
    </form>
  );
}
