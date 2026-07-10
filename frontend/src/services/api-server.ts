import { cookies } from "next/headers";

import { AUTH_COOKIE } from "@/lib/auth";

const backendURL =
  process.env.BACKEND_INTERNAL_URL ||
  process.env.NEXT_PUBLIC_BACKEND_URL ||
  "http://localhost:8080";

export async function apiServer<T>(path: string, init?: RequestInit): Promise<T> {
  const token = cookies().get(AUTH_COOKIE)?.value;
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
    throw new Error(await extractMessage(response));
  }

  return (await response.json()) as T;
}

async function extractMessage(response: Response) {
  try {
    const body = (await response.json()) as { message?: string };
    return body.message || "Request failed";
  } catch {
    return "Request failed";
  }
}
