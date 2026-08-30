export async function proxyClient<T>(path: string, init?: RequestInit): Promise<T> {
	const isFormData = typeof FormData !== "undefined" && init?.body instanceof FormData;
	const response = await fetch(`/api/backend${path}`, {
    ...init,
    cache: "no-store",
		headers: {
			...(isFormData ? {} : { "Content-Type": "application/json" }),
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
		return body.message || "ดำเนินการไม่สำเร็จ";
	} catch {
		return "ดำเนินการไม่สำเร็จ";
	}
}
