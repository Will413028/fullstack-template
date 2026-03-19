import { Suspense } from "react";
import { LoginForm } from "@/features/auth/components/login-form";

export default function LoginPage() {
  return (
    <div className="space-y-6">
      <div className="text-center">
        <h1 className="text-2xl font-medium text-white">Welcome back</h1>
        <p className="text-zinc-400 mt-2">Sign in to your account</p>
      </div>
      <div className="p-6 bg-zinc-900/50 border border-zinc-800 rounded-2xl">
        <Suspense fallback={<div className="h-64" />}>
          <LoginForm />
        </Suspense>
      </div>
    </div>
  );
}
