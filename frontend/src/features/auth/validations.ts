import { z } from "zod";

// Only presence is checked here. Lengths and formats belong to the API
// contract (plan I1): the backend enforces them and answers with field errors,
// which the form shows. The messages are field error codes, translated at
// render time like the backend's.
export const credentialsSchema = z.object({
  account: z.string().min(1, "required"),
  password: z.string().min(1, "required"),
});

export type CredentialsFormData = z.infer<typeof credentialsSchema>;
