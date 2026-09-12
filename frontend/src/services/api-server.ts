import { cookies } from "next/headers";

import { AUTH_COOKIE } from "@/lib/auth";
import { resolveBackendURL } from "@/lib/backend-url";


/**
 * A non-2xx answer from the API, carrying what the API actually said.
 *
 * The status is what a caller needs to tell "the token was refused" (401)
 * from "the API fell over" (5xx) from "the request was wrong" (4xx); the
 * message is the API's own; and the first part of the raw body is kept for
 * the case where the API did not answer in JSON at all — a proxy error page,
 * a gateway timeout — which the old code turned into "ดำเนินการไม่สำเร็จ" and
 * nothing else.
 */
export class ApiError extends Error {
  readonly status: number;
  readonly body: string;

  constructor(message: string, status: number, body: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
  }
}

const GENERIC_FAILURE = "ดำเนินการไม่สำเร็จ";
const BODY_EXCERPT = 512;

/**
 * Reads a failed response once, as text, and lifts the API's message out of
 * it if it is JSON. There is no catch to swallow anything: a body that is not
 * JSON is simply a body that is not JSON, and it is kept on the error.
 */
async function failureFromResponse(response: Response): Promise<ApiError> {
  const text = await response.text();
  let message = GENERIC_FAILURE;
  try {
    const parsed = JSON.parse(text) as { message?: unknown };
    if (typeof parsed?.message === "string" && parsed.message.trim()) {
      message = parsed.message;
    }
  } catch {
    // Not JSON. The excerpt below is the evidence; message stays generic.
  }
  return new ApiError(message, response.status, text.slice(0, BODY_EXCERPT));
}

export async function apiServer<T>(path: string, init?: RequestInit): Promise<T> {
  const backendURL = resolveBackendURL();
  const token = (await cookies()).get(AUTH_COOKIE)?.value;
  const response = await fetch(`${backendURL}/api/v1${path}`, {
    ...init,
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(init?.headers || {})
    }
  });

  if (!response.ok) {
    throw await failureFromResponse(response);
  }

  return (await response.json()) as T;
}
