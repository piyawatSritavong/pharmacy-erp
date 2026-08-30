"use client";

import { useCallback, useState } from "react";
import { z, type ZodError, type ZodType } from "zod";

/**
 * Centralized validation (A2). Every form pulls its field wording from
 * `messages` and builds its schema from `rules` instead of writing its own
 * copy per page — the same field error reads the same everywhere it's reused.
 */
export const messages = {
  required: (label: string) => `กรุณากรอก${label}`,
  select: (label: string) => `กรุณาเลือก${label}`,
  tooLong: (label: string, max: number) => `${label}ต้องไม่เกิน ${max.toLocaleString("th-TH")} ตัวอักษร`,
  tooShort: (label: string, min: number) => `${label}ต้องมีอย่างน้อย ${min.toLocaleString("th-TH")} ตัวอักษร`,
  duplicate: (label: string) => `${label}นี้มีอยู่แล้วในระบบ`,
  invalidNumber: (label: string) => `${label}ต้องเป็นตัวเลข`,
  mustBePositive: (label: string) => `${label}ต้องมากกว่า 0`,
  mustBeNonNegative: (label: string) => `${label}ต้องไม่ติดลบ`,
  invalidFormat: (label: string) => `รูปแบบ${label}ไม่ถูกต้อง`
} as const;

/** Reusable schema pieces — compose a form's zod object from these. */
export const rules = {
  text: (label: string, { max, min = 1 }: { max?: number; min?: number } = {}) => {
    let schema = z.string({ error: messages.required(label) }).trim().min(min, messages.required(label));
    if (max) schema = schema.max(max, messages.tooLong(label, max));
    return schema;
  },
  optionalText: (label: string, { max }: { max?: number } = {}) => {
    let schema = z.string().trim();
    if (max) schema = schema.max(max, messages.tooLong(label, max));
    return schema.optional().or(z.literal(""));
  },
  select: (label: string) => z.string({ error: messages.select(label) }).min(1, messages.select(label)),
  positiveNumber: (label: string) =>
    z.coerce.number({ error: messages.invalidNumber(label) }).positive(messages.mustBePositive(label)),
  nonNegativeNumber: (label: string) =>
    z.coerce.number({ error: messages.invalidNumber(label) }).nonnegative(messages.mustBeNonNegative(label)),
  email: (label = "อีเมล") =>
    z.string({ error: messages.required(label) }).trim().min(1, messages.required(label)).email(messages.invalidFormat(label)),
  phone: (label = "เบอร์โทรศัพท์") =>
    z
      .string()
      .trim()
      .regex(/^0[0-9]{8,9}$/, messages.invalidFormat(label))
      .optional()
      .or(z.literal(""))
};

/** Flattens a ZodError into { fieldName: message } for <Field error=...>. */
export function fieldErrors(error: ZodError<unknown>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const issue of error.issues) {
    const key = issue.path.join(".") || "_root";
    if (!out[key]) out[key] = issue.message;
  }
  return out;
}

export function validate<T>(schema: ZodType<T>, value: unknown) {
  const result = schema.safeParse(value);
  if (result.success) {
    return { success: true as const, data: result.data, errors: {} as Record<string, string> };
  }
  return { success: false as const, data: undefined, errors: fieldErrors(result.error) };
}

/**
 * Submit-time validation for the existing manual-useState form pattern.
 * Returns the current field-error map (feed straight into `<Field error>`)
 * and a `validate()` gate to call before submit.
 */
export function useFormErrors<T>(schema: ZodType<T>) {
  const [errors, setErrors] = useState<Record<string, string>>({});

  const runValidation = useCallback(
    (value: unknown) => {
      const result = validate(schema, value);
      setErrors(result.errors);
      return result;
    },
    [schema]
  );

  const clearError = useCallback((field: string) => {
    setErrors((current) => {
      if (!(field in current)) return current;
      const next = { ...current };
      delete next[field];
      return next;
    });
  }, []);

  return { errors, validate: runValidation, clearError, setErrors };
}
