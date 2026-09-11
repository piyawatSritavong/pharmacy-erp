import { expect, test, type Page } from "@playwright/test";

import { passwordFor } from "./credentials";

async function signIn(page: Page, email: string, expectedPath: string) {
  await page.goto("/login");
  await page.getByLabel("อีเมล").fill(email);
  await page.getByLabel("รหัสผ่าน").fill(passwordFor(email));
  await page.getByRole("button", { name: "เข้าสู่ระบบ" }).click();
  await page.waitForURL(new RegExp(`${expectedPath}$`));
}

function stockRows(page: Page) {
  return page.locator('[role="listbox"][aria-label="รายการสต๊อก"] [role="option"]');
}

async function expectUniqueStockRows(page: Page) {
  const values = await stockRows(page).evaluateAll((rows) =>
    rows.map((row) => (row.textContent || "").replace(/\s+/g, " ").trim())
  );
  expect(values.length).toBeGreaterThan(0);
  expect(new Set(values).size).toBe(values.length);
}

test.describe("refactored operational workflows", () => {
  test.describe.configure({ mode: "serial" });
  test.skip(!process.env.E2E_RUN, "กำหนด E2E_RUN=1 เมื่อเปิดบริการแล้ว");

  test("สต๊อกจริงเลือกสาขาได้ แต่สต๊อกผีเป็นหน้าดูข้อมูลแบบไม่มีตัวเลือกและไม่มีปุ่มแก้ไข", async ({ page }) => {
    await signIn(page, "superadmin@erp.local", "/dashboard");

    await page.goto("/real-inventory");
    await expectUniqueStockRows(page);
    const branch = page.getByLabel("กรองสาขาสต๊อก");
    await expect(branch).toBeVisible();
    const currentBranch = new URL(page.url()).searchParams.get("inventory_branch");
    await branch.click();
    const options = page.getByRole("option");
    expect(await options.count()).toBeGreaterThan(1);
    await options.nth(1).click();
    await page.waitForURL((url) =>
      url.pathname === "/real-inventory" &&
      Boolean(url.searchParams.get("inventory_branch")) &&
      url.searchParams.get("inventory_branch") !== currentBranch
    );
    await expectUniqueStockRows(page);

    await page.goto("/ghost-inventory?inventory_branch=ignored");
    await expectUniqueStockRows(page);
    await expect(page.getByLabel("กรองสาขาสต๊อก")).toHaveCount(0);
    await expect(page.getByText("สาขา", { exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "ดู Lot/วันหมดอายุ", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "ตั้งค่าเฉพาะสาขา", exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "ลบสินค้า", exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "เพิ่มรูปใหม่", exact: true })).toHaveCount(0);
    await expect(page.getByText("ราคาตั้งต้น", { exact: true })).toHaveCount(0);
    await expect(page.getByText("ราคาขาย", { exact: true })).toHaveCount(0);
  });

  test("ใบขายและ รพ.สต. เป็นฟีเจอร์ Pro และแสดงกล่องอัปเกรด", async ({ page }) => {
    await signIn(page, "superadmin@erp.local", "/dashboard");

    for (const route of ["/sales-management", "/government-sales"]) {
      await page.goto(route);
      await expect(page.getByText("PharmaPOS Pro").first()).toBeVisible();
      await expect(page.getByRole("button", { name: "สมัคร Pro รายเดือน" })).toBeVisible();
    }
  });

  test("POS ขอสินค้าได้เฉพาะสินค้าและจำนวน และไม่มีเช็คหรือผ่อนชำระ", async ({ page }) => {
    await signIn(page, "pos.mes@erp.local", "/sales");
    await page.goto("/requisitions");

    await expect(page.getByLabel("เลือกสินค้าที่ต้องการเบิก")).toHaveCount(0);
    await page.getByRole("button", { name: "สร้างใบเบิกสินค้า" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByLabel("เลือกสินค้าที่ต้องการเบิก")).toBeVisible();
    await expect(dialog.getByLabel("จำนวนที่ต้องการ")).toBeVisible();
    await expect(dialog.getByText(/ผู้ดูแลจะเลือกสาขาต้นทางและประเภทสต๊อกให้/)).toBeVisible();
    // A POS requisition names no branch and no stock bucket — that is the
    // admin's call — and never a cheque or instalment.
    await expect(dialog.getByText("สาขาที่ขอเบิก")).toHaveCount(0);
    await expect(dialog.getByLabel(/ประเภทสต๊อก/)).toHaveCount(0);
    await expect(page.getByText("เช็ค", { exact: true })).toHaveCount(0);
    await expect(page.getByText("ผ่อนชำระ", { exact: true })).toHaveCount(0);
  });

  test("รายงานภาษีและกำไรขาดทุนเรียงจากบนลงล่าง", async ({ page }) => {
    await signIn(page, "superadmin@erp.local", "/dashboard");
    await page.goto("/global-reports");

    const tax = page.getByRole("heading", { name: "รายงานภาษี", exact: true });
    const profit = page.getByRole("heading", { name: "กำไรและขาดทุน", exact: true });
    await expect(tax).toBeVisible();
    await expect(profit).toBeVisible();
    const taxBox = await tax.boundingBox();
    const profitBox = await profit.boundingBox();
    expect(taxBox).not.toBeNull();
    expect(profitBox).not.toBeNull();
    expect(profitBox!.y).toBeGreaterThan(taxBox!.y + taxBox!.height);
  });
});
