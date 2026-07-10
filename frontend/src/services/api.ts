export async function proxyClient<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/backend${path}`, {
    ...init,
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
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
