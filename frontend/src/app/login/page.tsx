"use client";

import { FormEvent, useState } from "react";

import { Cross } from "lucide-react";

import { Button, Card, CardBody, CardHeader, Input, Notice } from "@/components/ui/primitives";
import { ProxyError, proxyClient } from "@/services/api";

/** What PharmaPOS is, in one sentence — shown on every screen size. */
const ONE_LINER = "ระบบขายหน้าร้านและบริหารสต๊อกสำหรับเครือร้านขายยาหลายสาขา";
const FEATURES = "ขาย POS · สต๊อกทุกสาขา · โอนสินค้า · ใบสั่งซื้อ · ใบกำกับภาษี · ปิดรอบสิ้นเดือน ในระบบเดียว";

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
      // The server's own messages are Thai (e.g. a wrong password); anything
      // else is the network — the browser's "Failed to fetch" means nothing here.
      setError(caught instanceof ProxyError ? caught.message : "เชื่อมต่อระบบไม่ได้ กรุณาตรวจสอบอินเทอร์เน็ตแล้วลองอีกครั้ง");
      setLoading(false);
    }
  }

  return (
    <main className="grid min-h-screen gap-6 bg-background p-4 lg:grid-cols-[minmax(0,1.05fr)_minmax(420px,0.65fr)] lg:p-6">
      <section className="relative hidden overflow-hidden rounded-2xl bg-foreground p-12 text-background shadow-card lg:flex lg:items-end">
        <span className="absolute -right-24 -top-24 h-80 w-80 rounded-full bg-primary" />
        <span className="absolute right-40 top-24 h-20 w-20 rounded-full bg-secondary" />
        <div className="space-y-4">
          <p className="text-sm font-semibold text-warning">PharmaPOS</p>
          <h1 className="max-w-lg text-4xl font-bold leading-tight tracking-tight xl:text-5xl">{ONE_LINER}</h1>
          <p className="max-w-lg leading-8 text-background/70">{FEATURES}</p>
        </div>
      </section>

      <section className="flex flex-col items-center justify-center">
        {/* Below lg the hero above is hidden; tills run on tablets, so the
            product name and what it does must be said here too. */}
        <div className="mb-6 w-full max-w-md space-y-2 lg:hidden">
          <p className="flex items-center gap-2 text-sm font-semibold text-primary">
            <span className="grid h-8 w-8 place-items-center rounded-xl bg-primary text-primary-foreground"><Cross aria-hidden className="h-4 w-4" strokeWidth={3} /></span>
            PharmaPOS
          </p>
          <h1 className="text-xl font-semibold leading-snug tracking-tight">{ONE_LINER}</h1>
          <p className="text-sm text-muted-foreground">{FEATURES}</p>
        </div>
        <Card className="w-full max-w-md">
          <CardHeader
            title="เข้าสู่ระบบ"
            description="ใช้อีเมลและรหัสผ่านที่ได้รับจากผู้ดูแลระบบ ระบบจะพาไปยังหน้าจอตามสิทธิ์ของคุณ"
          />
          <CardBody>
            <form className="space-y-4" onSubmit={handleSubmit}>
              <label className="block space-y-2">
                <span className="text-sm font-medium">อีเมล</span>
                <Input
                  autoComplete="username"
                  inputMode="email"
                  name="email"
                  onChange={(event) => setEmail(event.target.value)}
                  required
                  type="email"
                  value={email}
                />
              </label>
              <label className="block space-y-2">
                <span className="text-sm font-medium">รหัสผ่าน</span>
                <Input
                  autoComplete="current-password"
                  name="password"
                  onChange={(event) => setPassword(event.target.value)}
                  required
                  type="password"
                  value={password}
                />
              </label>
              {error ? <Notice tone="error">{error}</Notice> : null}
              <Button className="w-full" loading={loading} loadingText="กำลังเข้าสู่ระบบ..." type="submit">
                เข้าสู่ระบบ
              </Button>
            </form>
          </CardBody>
        </Card>
      </section>
    </main>
  );
}
