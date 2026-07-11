"use client";

import { useRouter } from "next/navigation";
import { FormEvent, useState, startTransition } from "react";

import { Button, Card, CardBody, CardHeader, Input } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState("superadmin@erp.local");
  const [password, setPassword] = useState("DevPassword123!");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoading(true);
    setError(null);

    try {
      const session = await proxyClient<{ home_path: string }>("/auth/login", {
        method: "POST",
        body: JSON.stringify({ email, password })
      });

      startTransition(() => {
        router.push(session.home_path || "/dashboard");
        router.refresh();
      });
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Login failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="grid min-h-screen bg-background px-4 py-8 lg:grid-cols-[minmax(0,0.85fr)_minmax(0,0.65fr)] lg:px-8">
      <section className="hidden items-end rounded-lg border bg-gradient-to-br from-[hsl(173,80%,16%)] to-[hsl(173,60%,26%)] p-10 text-white shadow-sm lg:flex">
        <div className="space-y-4">
          <p className="text-xs font-medium uppercase tracking-widest text-white/60">Thin Client</p>
          <h1 className="max-w-lg text-4xl font-semibold tracking-tight">
            Pharmacy ERP
          </h1>
          <p className="max-w-lg text-sm leading-7 text-white/75">
            Next.js renders only. Every calculation, tax, stock movement, sequence,
            and audit trail is enforced by the Go API.
          </p>
        </div>
      </section>

      <section className="grid place-items-center">
        <Card className="w-full max-w-md">
          <CardHeader
            title="Sign In"
            description="Seeded dev accounts are ready. Try Super Admin, Branch Admin, or POS."
          />
          <CardBody>
            <form className="space-y-4" onSubmit={handleSubmit}>
              <label className="block space-y-2">
                <span className="text-sm font-medium">Email</span>
                <Input value={email} onChange={(event) => setEmail(event.target.value)} />
              </label>
              <label className="block space-y-2">
                <span className="text-sm font-medium">Password</span>
                <Input
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                />
              </label>
              {error ? <p className="text-sm text-destructive">{error}</p> : null}
              <Button className="w-full" disabled={loading} type="submit">
                {loading ? "Signing in..." : "Sign In"}
              </Button>
            </form>
            <div className="mt-6 rounded-md border bg-muted/50 p-4 text-sm text-muted-foreground">
              <p>`superadmin@erp.local` / `branchadmin@erp.local` / `pos@erp.local`</p>
              <p className="mt-1">Password: `DevPassword123!`</p>
            </div>
          </CardBody>
        </Card>
      </section>
    </main>
  );
}
