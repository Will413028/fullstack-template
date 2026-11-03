import { z } from "zod";
import type { FieldViolationReasonName } from "@/lib/api/connect-errors";

// Only presence is checked here. Lengths and formats are the server's to
// check, in one place (the auth service, docs/adr/2026-11-03-api-contract-protobuf-connect.md D2):
// it answers with field violations, which the form shows. The messages are
// field violation reasons, translated at render time like the server's.
const required: FieldViolationReasonName = "REQUIRED";

export const credentialsSchema = z.object({
  account: z.string().min(1, required),
  password: z.string().min(1, required),
});

export type CredentialsFormData = z.infer<typeof credentialsSchema>;
