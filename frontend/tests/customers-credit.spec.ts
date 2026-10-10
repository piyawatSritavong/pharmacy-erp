import { expect, test, type Browser, type Page } from "@playwright/test";

import { passwordFor } from "./credentials";

/**
 * The customer features end to end, the way the demo walks them: a member is
 * registered at the till and collects points, spends them on the next bill, a
 * wholesale account buys on credit, and the debt is collected at the counter.
 * Everything the suite needs it creates itself.
 */

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

/** Puts the first sellable product on the till into the cart. */
async function addFirstProduct(page: Page) {
  await page.getByRole("button", { name: /^เพิ่ม .* ลงตะกร้า$/ }).first().click();
  const lotDialog = page.getByRole("dialog", { name: /^เลือก Lot/ });
  await expect(lotDialog).toBeVisible();
  await lotDialog.getByRole("button", { name: /^Lot / }).first().click();
  await expect(lotDialog).toHaveCount(0);
}

async function pay(page: Page, method?: string | RegExp) {
  await page.getByRole("button", { name: "รับชำระเงิน", exact: true }).click();
  const payment = page.getByRole("dialog", { name: "รับชำระเงิน" });
  await expect(payment).toBeVisible();
  if (method) {
    await payment.getByRole("combobox", { name: "ช่องทางชำระเงิน" }).click();
    await page.getByRole("option", { name: method, exact: typeof method === "string" }).click();
  }
  const confirm = payment.getByRole("button", { name: /^ยืนยัน/ });
  await expect(confirm).toBeEnabled();
  await confirm.click();
}

test.describe("สมาชิก แต้มสะสม และขายเชื่อ", () => {
  test.describe.configure({ mode: "serial" });
  test.skip(!process.env.E2E_RUN, "กำหนด E2E_RUN=1 เมื่อเปิดบริการแล้ว");

  const stamp = String(Date.now()).slice(-8);
  const memberPhone = `08${stamp}`;
  const memberName = `สมาชิก E2E ${stamp}`;
  const accountName = `ร้านขายส่ง E2E ${stamp}`;

  test("สำนักงานใหญ่เปิดบัญชีขายส่งพร้อมวงเงินเครดิต", async ({ browser }) => {
    const { context, page } = await open(browser, "admin.central@erp.local", /\/dashboard$/);
    await page.goto("/customers");
    await expect(page.getByRole("heading", { level: 1, name: "ลูกค้าและสมาชิก" })).toBeVisible();
    await expect(page.getByText(/ซื้อทุก .* บาท ได้ 1 แต้ม/)).toBeVisible();
    await page.getByRole("button", { name: "สมัครสมาชิก" }).first().click();
    const dialog = page.getByRole("dialog", { name: "สมัครสมาชิก" });
    await dialog.getByLabel("ชื่อลูกค้า *").fill(accountName);
    await dialog.getByLabel("ประเภทลูกค้า").click();
    await page.getByRole("option", { name: "ร้านค้า/นิติบุคคล" }).click();
    await dialog.getByLabel("ระดับราคา").click();
    await page.getByRole("option", { name: /^ขายส่ง/ }).click();
    await dialog.getByLabel("วงเงินเครดิต (บาท)").fill("50000");
    await dialog.getByLabel("เครดิต (วัน)").fill("30");
    await dialog.getByRole("button", { name: "บันทึก" }).click();
    await expect(page.getByText(/สมัครสมาชิกแล้ว · รหัส C\d{6}/)).toBeVisible();
    await page.getByLabel("ค้นหาลูกค้า").fill(accountName);
    const row = page.getByRole("row", { name: new RegExp(accountName) });
    await expect(row).toBeVisible();
    await expect(row.getByText("ขายส่ง", { exact: true })).toBeVisible();
    await expect(row.getByText(/ใช้ได้อีก/)).toBeVisible();
    await context.close();
  });

  test("หน้าร้านสมัครสมาชิกจากตะกร้า แล้วบิลแรกได้แต้ม", async ({ browser }) => {
    const { context, page } = await open(browser, "pos.mes@erp.local", /\/sales$/);
    await page.getByRole("button", { name: /ลูกค้าทั่วไป/ }).click();
    const picker = page.getByRole("dialog", { name: "เลือกสมาชิก" });
    await picker.getByLabel("ค้นหาสมาชิก").fill(memberPhone);
    await picker.getByRole("button", { name: "สมัครสมาชิกใหม่" }).click();
    const register = page.getByRole("dialog", { name: "สมัครสมาชิกใหม่" });
    await expect(register.getByLabel("เบอร์โทร *")).toHaveValue(memberPhone);
    await register.getByLabel("ชื่อ *").fill(memberName);
    await register.getByRole("button", { name: "สมัครและเลือก" }).click();
    await expect(page.getByText(memberName)).toBeVisible();

    await addFirstProduct(page);
    await expect(page.getByText(/บิลนี้ได้ \d+ แต้ม/)).toBeVisible();
    await pay(page);
    const done = page.getByRole("dialog", { name: "ชำระเงินเสร็จสิ้น" });
    await expect(done.getByText(memberName)).toBeVisible();
    await expect(done.getByText(/ได้รับ \d+ แต้ม/)).toBeVisible();
    await done.getByRole("button", { name: "เริ่มรายการใหม่" }).click();
    await context.close();
  });

  test("สมาชิกใช้แต้มเป็นส่วนลด และบัญชีขายส่งซื้อเชื่อ", async ({ browser }) => {
    const central = await open(browser, "admin.central@erp.local", /\/dashboard$/);
    // Top the member up so a redemption is possible on a fresh database.
    const members = await central.page.request.get(`/api/backend/customers/lookup?q=${memberPhone}`);
    const member = (await members.json()).items[0];
    const adjusted = await central.page.request.post(`/api/backend/customers/${member.id}/points`, {
      data: { points: 100, note: "E2E" }
    });
    expect(adjusted.status()).toBe(200);
    await central.context.close();

    const { context, page } = await open(browser, "pos.mes@erp.local", /\/sales$/);
    await page.getByRole("button", { name: /ลูกค้าทั่วไป/ }).click();
    const picker = page.getByRole("dialog", { name: "เลือกสมาชิก" });
    await picker.getByLabel("ค้นหาสมาชิก").fill(memberPhone);
    await picker.getByRole("button", { name: new RegExp(memberName) }).click();
    await addFirstProduct(page);
    await page.getByLabel("ใช้แต้มเป็นส่วนลด").fill("40");
    await expect(page.getByText(/ใช้ 40 แต้ม/)).toBeVisible();
    await pay(page, "เงินโอน");
    const done = page.getByRole("dialog", { name: "ชำระเงินเสร็จสิ้น" });
    await expect(done.getByText(/ใช้ 40 แต้ม/)).toBeVisible();
    await done.getByRole("button", { name: "เริ่มรายการใหม่" }).click();

    // Credit: the wholesale account pays nothing now and gets a due date.
    await page.getByRole("button", { name: /ลูกค้าทั่วไป/ }).click();
    await page.getByRole("dialog", { name: "เลือกสมาชิก" }).getByLabel("ค้นหาสมาชิก").fill(accountName);
    await page.getByRole("dialog", { name: "เลือกสมาชิก" }).getByRole("button", { name: new RegExp(accountName) }).click();
    await expect(page.getByText(/เครดิตคงเหลือ/)).toBeVisible();
    await addFirstProduct(page);
    await pay(page, /^ขายเชื่อ/);
    const credit = page.getByRole("dialog", { name: "ชำระเงินเสร็จสิ้น" });
    await expect(credit.getByText("บันทึกขายเชื่อแล้ว")).toBeVisible();
    await expect(credit.getByText(/ครบกำหนดชำระ/)).toBeVisible();
    await credit.getByRole("button", { name: "เริ่มรายการใหม่" }).click();
    await context.close();
  });

  test("หน้าร้านรับชำระหนี้บิลเครดิต และยอดค้างลดลง", async ({ browser }) => {
    const { context, page } = await open(browser, "pos.mes@erp.local", /\/sales$/);
    await page.goto("/receivables");
    await expect(page.getByRole("heading", { level: 1, name: "ลูกหนี้ค้างชำระ" })).toBeVisible();
    await page.getByLabel("ค้นหาลูกหนี้").fill(accountName);
    const row = page.getByRole("row", { name: new RegExp(accountName) });
    await expect(row).toBeVisible();
    await row.getByRole("button", { name: "รับชำระ" }).click();
    const dialog = page.getByRole("dialog", { name: new RegExp(`รับชำระหนี้ · ${accountName}`) });
    await expect(dialog.getByLabel("ยอดรับชำระ (บาท)")).not.toHaveValue("");
    await dialog.getByRole("button", { name: /^รับชำระ/ }).click();
    await expect(page.getByText(new RegExp(`รับชำระจาก ${accountName}`))).toBeVisible();
    await expect(page.getByText("ไม่มีบิลขายเชื่อค้างชำระตามเงื่อนไขนี้").first()).toBeVisible();
    await context.close();
  });

  test("ตั้งหน่วยขายแบบแพ็คและราคาส่งได้จากรายการสินค้า", async ({ browser }) => {
    const { context, page } = await open(browser, "superadmin@erp.local", /\/dashboard$/);
    await page.goto("/product-catalog");
    await page.getByRole("button", { name: "หน่วย/ราคาส่ง" }).first().click();
    const dialog = page.getByRole("dialog", { name: /^หน่วยขายและราคาส่ง/ });
    await expect(dialog.getByText("หน่วยขาย (แตกขายปลีก–ส่ง)")).toBeVisible();
    await dialog.getByRole("button", { name: "เพิ่มหน่วย" }).click();
    await dialog.getByLabel("ชื่อหน่วย").last().fill(`แพ็ค ${stamp}`);
    await dialog.getByLabel("จำนวนต่อหน่วย").last().fill("12");
    await dialog.getByRole("button", { name: "บันทึกหน่วยขาย" }).click();
    await expect(dialog.getByText("บันทึกหน่วยขายแล้ว")).toBeVisible();
    await dialog.getByRole("button", { name: "เพิ่มราคา" }).click();
    await dialog.getByLabel("ราคาส่งต่อหน่วย").last().fill("1");
    await dialog.getByRole("button", { name: "บันทึกราคาส่ง" }).click();
    await expect(dialog.getByText(/บันทึกราคาส่งแล้ว/)).toBeVisible();
    await context.close();
  });

  test("หน้าลูกค้าและลูกหนี้ไม่ล้นจอมือถือ", async ({ browser }) => {
    const context = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true });
    const page = await context.newPage();
    await signIn(page, "pos.mes@erp.local", /\/sales$/);
    for (const path of ["/customers", "/receivables"]) {
      await page.goto(path);
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    }
    await context.close();
  });
});
