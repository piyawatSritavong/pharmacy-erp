/** A non-2xx answer from the proxy, carrying the status and the API's message. */
export class ProxyError extends Error {
  readonly status: number;
  readonly body: string;

  constructor(message: string, status: number, body: string) {
    super(message);
    this.name = "ProxyError";
    this.status = status;
    this.body = body;
  }
}

const GENERIC_FAILURE = "ดำเนินการไม่สำเร็จ";
const BODY_EXCERPT = 512;

async function failureFromResponse(response: Response): Promise<ProxyError> {
  const text = await response.text();
  let message = GENERIC_FAILURE;
  try {
    const parsed = JSON.parse(text) as { message?: unknown };
    if (typeof parsed?.message === "string" && parsed.message.trim()) {
      message = parsed.message;
    }
  } catch {
    // Not JSON; the excerpt is kept on the error, the message stays generic.
  }
  return new ProxyError(message, response.status, text.slice(0, BODY_EXCERPT));
}

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
    throw await failureFromResponse(response);
  }

  return (await response.json()) as T;
}
