import { expect, test, type Page } from "@playwright/test";

const password = "DevPassword123!";

async function signIn(page: Page, email: string) {
  await page.goto("/login");
  await page.getByLabel("อีเมล").fill(email);
  await page.getByLabel("รหัสผ่าน").fill(password);
  await page.getByRole("button", { name: "เข้าสู่ระบบ" }).click();
  await page.waitForURL(/\/dashboard$/);
}

/** Today in Bangkok — the day the dashboard opens on, wherever the runner is. */
function bangkokToday() {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Bangkok" }).format(new Date());
}

test.describe("Dashboard", () => {
  test("เปิดมาที่วันนี้ และเดินวันถอยหลัง–เดินหน้าได้", async ({ page }) => {
    await signIn(page, "superadmin@erp.local");

    const todayLabel = new Intl.DateTimeFormat("th-TH", {
      timeZone: "Asia/Bangkok",
      day: "numeric",
      month: "long",
      year: "numeric"
    }).format(new Date());
    await expect(page.getByText(todayLabel, { exact: true })).toBeVisible();
    await expect(page.getByText("วันนี้ · อัปเดตสด")).toBeVisible();

    // Stepping back must move the window and say so, without the operator
    // having to type a date.
    await page.getByRole("button", { name: "วันก่อนหน้า" }).click();
    await page.waitForURL(/date_from=/);
    await expect(page.getByText("ย้อนหลัง")).toBeVisible();
    await expect(page.getByText(todayLabel, { exact: true })).toHaveCount(0);

    await page.getByRole("button", { name: "วันถัดไป" }).click();
    await page.waitForURL(new RegExp(`date_from=${bangkokToday()}`));
    await expect(page.getByText("วันนี้ · อัปเดตสด")).toBeVisible();
  });

  test("Superadmin เห็นยอดเข้าเงื่อนไขสต๊อกผี แต่ admin.central เห็นแค่ยอดจริง", async ({ browser }) => {
    const superContext = await browser.newContext();
    const superPage = await superContext.newPage();
    await signIn(superPage, "superadmin@erp.local");
    await expect(superPage.getByText("ยอดรวมจริง").first()).toBeVisible();
    await expect(superPage.getByText(/ยอดเข้าเงื่อนไข ต้นทุน/).first()).toBeVisible();
    await expect(superPage.getByText("มีในสต๊อกผี — ต้องหายไป").first()).toBeVisible();
    await superContext.close();

    // central_admin works from the adjusted books: one total, split by how the
    // money arrived. Ghost Stock does not exist on this screen or any other.
    const centralContext = await browser.newContext();
    const centralPage = await centralContext.newPage();
    await signIn(centralPage, "admin.central@erp.local");
    await expect(centralPage.getByText("ยอดรวม", { exact: true }).first()).toBeVisible();
    await expect(centralPage.getByText("ยอดโอน", { exact: true }).first()).toBeVisible();
    await expect(centralPage.getByText("ยอดเงินสด", { exact: true }).first()).toBeVisible();
    await expect(centralPage.getByText(/สต๊อกผี/)).toHaveCount(0);
    await expect(centralPage.getByText(/ยอดเข้าเงื่อนไข/)).toHaveCount(0);
    await expect(centralPage.getByText("ยอดรวมจริง")).toHaveCount(0);
    await expect(centralPage.getByRole("combobox", { name: "กรองตามสถานะการปิดรอบ" })).toHaveCount(0);
    await centralContext.close();
  });

  test("ตัวกรองตัดกล่องที่ไม่เกี่ยวออก", async ({ page }) => {
    await signIn(page, "superadmin@erp.local");
    // The selects are Radix comboboxes, so they open and are picked from
    // rather than set like a native <select>.
    await page.getByRole("combobox", { name: "กรองตามประเภทชำระเงิน" }).click();
    await page.getByRole("option", { name: "เงินโอน" }).click();
    await page.waitForURL(/payment_type=bank_transfer/);
    // Only the transfer groups survive, so the whole close panel goes with them.
    await expect(page.getByText(/ยอดเข้าเงื่อนไข/)).toHaveCount(0);
    await expect(page.getByText("ยอดโอน", { exact: true }).first()).toBeVisible();

    await page.getByRole("combobox", { name: "กรองตามสถานะการปิดรอบ" }).click();
    await page.getByRole("option", { name: /Hidden/ }).click();
    await page.waitForURL(/close_status=hidden/);
    await expect(page.getByText("ยอดรวมจริง")).toHaveCount(0);
  });

  test("ช่วงที่ปิดรอบแล้ว ยังเห็นยอดก่อนปรับ–หลังปรับ และส่วนต่าง", async ({ page }) => {
    await signIn(page, "superadmin@erp.local");
    await page.goto("/dashboard?date_from=2026-09-01&date_to=2026-09-06");
    await expect(page.getByText(/สรุปสิ้นเดือนแล้วในรอบ MER-/)).toBeVisible();
    // The bills the close removed are still counted on the "before" side, which
    // is the whole point of keeping the round's snapshot.
    await expect(page.getByText("หลังปรับ").first()).toBeVisible();
    await expect(page.getByText(/ส่วนต่าง/).first()).toBeVisible();
    await expect(page.getByText("มีในสต๊อกผี — ต้องหายไป").first()).toBeVisible();
  });

  test("สลับธีมมืดได้ และจำค่าไว้ข้ามหน้า", async ({ page }) => {
    await signIn(page, "superadmin@erp.local");
    const isDark = () => page.evaluate(() => document.documentElement.classList.contains("dark"));

    const startedDark = await isDark();
    await page.getByRole("button", { name: startedDark ? "เปลี่ยนเป็นธีมสว่าง" : "เปลี่ยนเป็นธีมมืด" }).click();
    expect(await isDark()).toBe(!startedDark);

    // The choice survives a reload, and is applied before paint rather than
    // flashing the other theme first.
    await page.reload();
    expect(await isDark()).toBe(!startedDark);
  });
});
