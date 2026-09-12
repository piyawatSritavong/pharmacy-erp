"use client";

import { FormEvent, useState } from "react";

import { Button, Card, CardBody, CardHeader, Input } from "@/components/ui/primitives";
import { proxyClient } from "@/services/api";

export default function LoginPage() {
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

      // A full document load, not a client navigation.
      //
      // The cookie has just changed, and every entry in the App Router's
      // client-side Router Cache predates it — including any /dashboard entry
      // fetched while logged out, which middleware answered with a redirect
      // back to /login. router.push() reads that cache before router.refresh()
      // can clear it, so the navigation resolves into a stale payload and the
      // page appears never to leave /login. A document load cannot consult the
      // cache at all: it re-runs middleware and sends the new cookie on a fresh
      // request. A login happens once per session, so the cost of a reload is
      // not worth the class of bug the client route avoids.
      //
      // replace() rather than assign() so /login does not stay in the history
      // stack. It is what a login should do regardless — nobody wants Back to
      // return to the form they just submitted — and it is load-bearing here:
      // /login is statically prerendered and bfcache-eligible, and middleware
      // treats it as public, so with assign() a Back from the landing page
      // restored this document with its React state intact. `loading` is left
      // true on this path (the document is going away, and clearing it would
      // flash the idle label mid-navigation), which in a restored document
      // meant a permanently disabled button reading "กำลังเข้าสู่ระบบ..." with
      // no request in flight. Removing the entry removes the way back to it.
      window.location.replace(session.home_path || "/dashboard");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "เข้าสู่ระบบไม่สำเร็จ");
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
