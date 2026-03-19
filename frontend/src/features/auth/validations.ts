import { z } from "zod";

export const loginSchema = z.object({
  account: z.string().min(1, "Account is required"),
  password: z.string().min(8, "Password must be at least 8 characters"),
});

export type LoginFormData = z.infer<typeof loginSchema>;
