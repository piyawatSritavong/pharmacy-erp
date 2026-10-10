export async function v2API<T>(path: string, input?: unknown, key?: string): Promise<T> {
  const response = await fetch(`/api/v2${path}`, {
    method: input === undefined ? "GET" : "POST",
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
      ...(input === undefined ? {} : { "Idempotency-Key": key || crypto.randomUUID() })
    },
    ...(input === undefined ? {} : { body: JSON.stringify(input) })
  });
  const result = await response.json();
  if (!response.ok) throw new Error(typeof result.message === "string" ? result.message : "ดำเนินการ V2 ไม่สำเร็จ");
  return result as T;
}
