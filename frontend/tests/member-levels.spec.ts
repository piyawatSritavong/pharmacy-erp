import { expect, test, type Browser, type Page } from "@playwright/test";
import { addProductToCart } from "./pos-helpers";
import { passwordFor } from "./credentials";

async function signIn(page: Page, email: string, landing: RegExp) {
  await page.goto("/login");
  await page.getByLabel("อีเมล").fill(email);
  await page.getByLabel("รหัสผ่าน").fill(passwordFor(email));
  await page.getByRole("button", { name: "เข้าสู่ระบบ" }).click();
  await page.waitForURL(landing);
}

async function open(browser: Browser, email: string, landing: RegExp) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();
  await signIn(page, email, landing);
  return { context, page };
}

/**
 * Member levels buy at price levels: a walk-in pays L1, a level-2 member pays
 * L3 by default. Head office sets the levels and the prices; the till sees the
 * member's level and the price it brought.
 */
test.describe("ระดับสมาชิกและระดับราคา", () => {
  test.describe.configure({ mode: "serial" });
  test.skip(!process.env.E2E_RUN, "กำหนด E2E_RUN=1 เมื่อเปิดบริการแล้ว");

  const stamp = String(Date.now()).slice(-8);
  const phone = `09${stamp}`;
  let productName = "";

  test("หน้าร้านเห็นลูกค้าระดับ 2 ได้ราคา L3 ของสินค้า", async ({ browser }) => {
    // The till names the product it can sell, so the price lands on a shelf
    // that has stock.
    const pos = await open(browser, "pos.mes@erp.local", /\/sales$/);
    const me = await (await pos.page.request.get("/api/backend/me")).json();
    const branchId = String(me.user?.branch_id || me.branch_id || "");
    const listed = await (await pos.page.request.get(`/api/backend/products?active=true&page_size=1&in_stock_first=1&branch_id=${branchId}`)).json();
    const product = listed.items[0];
    productName = String(product.name);

    const hq = await open(browser, "admin.central@erp.local", /\/dashboard$/);
    const shelf = Number(product.effective_price || product.base_selling_price);
    const l3 = Math.max(1, Math.round(shelf * 0.8 * 100) / 100);
    expect((await hq.page.request.put(`/api/backend/products/${product.id}/level-prices`, {
      data: { prices: [{ unit_id: "", price_level: 2, unit_price: Math.max(1, Math.round(shelf * 0.9 * 100) / 100) }, { unit_id: "", price_level: 3, unit_price: l3 }] }
    })).ok()).toBe(true);
    expect((await hq.page.request.post("/api/backend/customers", {
      data: { name: `ระดับสอง ${stamp}`, phone, member_level: 2 }
    })).ok()).toBe(true);

    // The product dialog shows what was saved.
    await hq.page.goto("/product-catalog");
    await hq.page.getByPlaceholder("ชื่อ, SKU, บาร์โค้ด").fill(productName);
    await hq.page.getByPlaceholder("ชื่อ, SKU, บาร์โค้ด").press("Enter");
    await hq.page.waitForURL(/search=/);
    await hq.page.getByRole("row").filter({ hasText: productName }).first().getByRole("button", { name: "หน่วย/ราคาส่ง" }).click();
    const dialog = hq.page.getByRole("dialog", { name: /^หน่วยขาย/ });
    await expect(dialog.getByText("ราคาตามระดับสมาชิก")).toBeVisible();
    await expect(dialog.getByLabel(/^ราคา L3 ของ/).first()).toHaveValue(String(l3));
    await hq.context.close();

    await pos.page.goto("/sales");
    await pos.page.getByLabel("ค้นหาสินค้า").fill(productName);
    await addProductToCart(pos.page, pos.page.getByRole("button", { name: `เพิ่ม ${productName} ลงตะกร้า`, exact: true }));
    await pos.page.getByRole("button", { name: /แตะเพื่อเลือกสมาชิก/ }).click();
    const picker = pos.page.getByRole("dialog", { name: "เลือกสมาชิก" });
    await pos.page.keyboard.type(phone);
    await picker.getByRole("button").filter({ hasText: `ระดับสอง ${stamp}` }).click();
    await expect(pos.page.getByText(/Lv\.2 .* · ได้ราคา L3/)).toBeVisible();
    await expect(pos.page.getByText(/^ราคาสมาชิก L3 · /)).toBeVisible();
    await pos.context.close();
  });

  test("หน้าร้านเปลี่ยนระดับสมาชิกเองไม่ได้ สำนักงานใหญ่ตั้งค่าระดับได้", async ({ browser }) => {
    const pos = await open(browser, "pos.mes@erp.local", /\/sales$/);
    await pos.page.goto("/customers");
    await pos.page.getByLabel("ค้นหาลูกค้า").fill(phone);
    await pos.page.getByRole("button", { name: "แก้ไข" }).first().click();
    await expect(pos.page.getByRole("combobox", { name: "ระดับสมาชิก" })).toBeDisabled();
    await expect(pos.page.getByRole("button", { name: "ตั้งค่าระดับ" })).toHaveCount(0);
    await pos.context.close();

    const hq = await open(browser, "admin.central@erp.local", /\/dashboard$/);
    await hq.page.goto("/customers");
    await hq.page.getByRole("button", { name: "ตั้งค่าระดับ" }).click();
    const settings = hq.page.getByRole("dialog", { name: "ตั้งค่าระดับสมาชิกและระดับราคา" });
    await expect(settings.getByLabel(/^ชื่อระดับสมาชิก/)).toHaveCount(5);
    await expect(settings.getByLabel(/^ชื่อราคาระดับ/)).toHaveCount(5);
    await settings.getByRole("button", { name: "บันทึก", exact: true }).click();
    await expect(hq.page.getByText("บันทึกระดับสมาชิกและระดับราคาแล้ว")).toBeVisible();
    await hq.context.close();
  });
});
