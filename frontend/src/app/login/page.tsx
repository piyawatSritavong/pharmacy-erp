"use client";

import { useRouter } from "next/navigation";
import { FormEvent, useState, startTransition } from "react";

import { Button, Card, CardBody, CardHeader, Input } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
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
      setError(caught instanceof Error ? caught.message : "เข้าสู่ระบบไม่สำเร็จ");
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="grid min-h-screen gap-6 bg-background p-4 lg:grid-cols-[minmax(0,1.05fr)_minmax(420px,0.65fr)] lg:p-6">
      <section className="relative hidden overflow-hidden rounded-[2.5rem] bg-foreground p-12 text-white shadow-card lg:flex lg:items-end">
        <span className="absolute -right-24 -top-24 h-80 w-80 rounded-full bg-primary" />
        <span className="absolute right-40 top-24 h-20 w-20 rounded-full bg-secondary" />
        <div className="space-y-4">
          <p className="text-sm font-semibold text-warning">PharmaPOS</p>
          <h1 className="max-w-lg text-5xl font-bold tracking-tight">จัดการร้านขายยาให้ง่ายขึ้นในทุกสาขา</h1>
          <p className="max-w-lg leading-8 text-white/70">ขายหน้าร้าน จัดการสต๊อก ออกเอกสาร และติดตามการเงินจากระบบเดียว</p>
        </div>
      </section>

      <section className="grid place-items-center">
        <Card className="w-full max-w-md">
          <CardHeader
            title="เข้าสู่ระบบ"
            description="เลือกใช้งานในฐานะผู้ดูแลระบบหรือพนักงานขายหน้าร้าน"
          />
          <CardBody>
            <form className="space-y-4" onSubmit={handleSubmit}>
              <label className="block space-y-2">
                <span className="text-sm font-medium">อีเมล</span>
                <Input value={email} onChange={(event) => setEmail(event.target.value)} />
              </label>
              <label className="block space-y-2">
                <span className="text-sm font-medium">รหัสผ่าน</span>
                <Input
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                />
              </label>
              {error ? <p className="text-sm text-destructive">{error}</p> : null}
              <Button className="w-full" disabled={loading} type="submit">
                {loading ? "กำลังเข้าสู่ระบบ..." : "เข้าสู่ระบบ"}
              </Button>
            </form>
          </CardBody>
        </Card>
      </section>
    </main>
  );
}
