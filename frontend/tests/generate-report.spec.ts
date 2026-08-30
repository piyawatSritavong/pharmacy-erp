import { expect, test } from "@playwright/test";

const password = "DevPassword123!";

async function dismissToasts(page: import("@playwright/test").Page) {
  await page.mouse.move(20, 500);
  await expect(page.locator("[data-sonner-toast]")).toHaveCount(0, { timeout: 10_000 });
}

test.describe("Generate Report", () => {
  test.skip(!process.env.E2E_RUN, "กำหนด E2E_RUN=1 เมื่อเปิดบริการแล้ว");

  test("สร้าง อัปเดตอัตโนมัติ เปิด dialog คอลัมน์ บันทึก และปักหมุด report ส่วนตัว", async ({ page }) => {
    const reportName = `Generate Report E2E ${Date.now()}`;
    const unmatchedProduct = `ไม่พบสินค้า E2E ${Date.now()}`;
    await page.goto("/login");
    await page.getByLabel("อีเมล").fill("superadmin@erp.local");
    await page.getByLabel("รหัสผ่าน").fill(password);
    await page.getByRole("button", { name: "เข้าสู่ระบบ" }).click();
    await page.waitForURL(/\/dashboard$/);

    await page.getByRole("link", { name: "Generate Report", exact: true }).first().click();
    await page.waitForURL(/\/generate-report$/);
    await expect(page.getByRole("heading", { name: "Generate Report", exact: true })).toBeVisible();
    await expect(page.getByText("All Fields", { exact: true })).toBeVisible();
    await expect(page.getByText(/Safe semantic query · Read-only/)).toBeVisible();
    await expect(page.getByText("Selected columns:", { exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "รันรายงาน", exact: true })).toHaveCount(0);
    await expect(page.getByText(/read-only/).last()).toBeVisible();

    const fieldSidebar = page.locator("#report-builder aside").filter({ hasText: "All Fields" });
    await expect(fieldSidebar.locator("details[open]")).toHaveCount(0);
    await fieldSidebar.locator("summary").filter({ hasText: "สาขา" }).first().click();

    const headers = page.locator("#report-builder thead th");
    await expect(headers).toHaveCount(14);
    const branchIDField = fieldSidebar.getByRole("button").filter({ hasText: "รหัสอ้างอิงสาขา" });
    await expect(branchIDField).toBeEnabled();
    await branchIDField.dragTo(headers.nth(1), { targetPosition: { x: 2, y: 12 } });
    await expect(headers.nth(1)).toContainText("รหัสอ้างอิงสาขา");
    await expect(headers).toHaveCount(15);
    await expect(branchIDField).toBeDisabled();

    await headers.nth(1).dragTo(headers.nth(2), { targetPosition: { x: 130, y: 12 } });
    await expect(headers.nth(1)).toContainText("ชื่อสาขา");
    await expect(headers.nth(2)).toContainText("รหัสอ้างอิงสาขา");
    await expect(page.locator("#report-builder tbody tr").first()).toBeVisible();

    await page.getByLabel("ชื่อรายงาน").fill(reportName);
    await expect(page.locator("#report-builder").getByRole("img", { name: /กราฟเปรียบเทียบ .* กับ จำนวนรายการ/ })).toBeVisible();
    await expect(page.getByText("แกน Y: จำนวนรายการ", { exact: true }).last()).toBeVisible();
    await expect(page.getByText(/แกน X: กิจกรรมล่าสุด/).last()).toBeVisible();
    await dismissToasts(page);

    await page.getByText(/Advanced filters/).click();
    await page.getByRole("button", { name: "เงื่อนไข", exact: true }).click();
    await page.getByLabel("ฟิลด์ตัวกรอง").selectOption("product_name");
    await page.getByLabel("ค่าตัวกรอง").fill(unmatchedProduct);
    await expect(page.getByText("ไม่พบข้อมูลตามเงื่อนไข", { exact: true })).toBeVisible();
    await dismissToasts(page);

    const sidebarResize = page.getByRole("separator", { name: "ปรับความกว้าง All Fields" });
    const originalWidth = Number(await sidebarResize.getAttribute("aria-valuenow"));
    const resizeBox = await sidebarResize.boundingBox();
    expect(resizeBox).not.toBeNull();
    await page.mouse.move(resizeBox!.x + resizeBox!.width / 2, resizeBox!.y + 100);
    await page.mouse.down();
    await page.mouse.move(resizeBox!.x + resizeBox!.width / 2 + 64, resizeBox!.y + 100, { steps: 5 });
    await page.mouse.up();
    const resizedWidth = Number(await sidebarResize.getAttribute("aria-valuenow"));
    expect(resizedWidth).toBeGreaterThan(originalWidth);

    const firstHeader = page.locator("#report-builder thead th").nth(1);
    await firstHeader.click({ button: "right" });
    await expect(page.getByRole("dialog")).toBeVisible();
    await page.keyboard.press("Escape");
    await firstHeader.focus();
    await page.keyboard.press("Shift+F10");
    await expect(page.getByRole("dialog")).toBeVisible();
    await page.getByRole("button", { name: "DESC", exact: true }).click();

    await page.getByRole("button", { name: "บันทึก", exact: true }).click();
    await expect(page.getByText("สร้างรายงานแล้ว", { exact: true })).toBeVisible();
    await dismissToasts(page);
    await page.getByRole("button", { name: "ปักหมุด", exact: true }).click();
    await expect(page.getByText("ปักหมุดรายงานแล้ว", { exact: true })).toBeVisible();

    await page.reload();
    const pinnedCard = page
      .getByRole("heading", { name: reportName, exact: true })
      .locator("xpath=ancestor::section[1]");
    await expect(pinnedCard).toBeVisible();
    await pinnedCard.getByRole("button", { name: "เปิดแก้ไข", exact: true }).click();
    await expect(page.getByRole("separator", { name: "ปรับความกว้าง All Fields" })).toHaveAttribute("aria-valuenow", String(resizedWidth));
    await dismissToasts(page);

    await page.getByRole("button", { name: "ใหม่", exact: true }).click();
    await expect(page.getByRole("separator", { name: "ปรับความกว้าง All Fields" })).toHaveAttribute("aria-valuenow", "288");
    await page.getByLabel("รายงานที่บันทึก").selectOption({ label: `📌 ${reportName}` });
    await page.getByRole("button", { name: `ถอนหมุด ${reportName}`, exact: true }).click();
    await expect(page.getByText("ถอนหมุดรายงานแล้ว", { exact: true })).toBeVisible();
    await dismissToasts(page);

    page.once("dialog", (dialog) => dialog.accept());
    await page.getByRole("button", { name: "ลบรายงาน", exact: true }).click();
    await expect(page.getByText("ลบรายงานแล้ว", { exact: true })).toBeVisible();
  });

  test("All Fields และเมนูคอลัมน์ยังใช้งานได้บนมือถือ", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/login");
    await page.getByLabel("อีเมล").fill("superadmin@erp.local");
    await page.getByLabel("รหัสผ่าน").fill(password);
    await page.getByRole("button", { name: "เข้าสู่ระบบ" }).click();
    await page.waitForURL(/\/dashboard$/);
    await page.goto("/generate-report");

    const fieldSidebar = page.locator("#report-builder aside").filter({ hasText: "All Fields" });
    await expect(page.getByRole("separator", { name: "ปรับความกว้าง All Fields" })).toBeHidden();
    await expect(fieldSidebar.locator("details[open]")).toHaveCount(0);
    await fieldSidebar.locator("summary").filter({ hasText: "สาขา" }).first().click();
    await fieldSidebar.getByRole("button").filter({ hasText: "รหัสอ้างอิงสาขา" }).click();
    await expect(page.locator("#report-builder thead th")).toHaveCount(15);

    const menuButton = page.getByRole("button", { name: /เมนูคอลัมน์/ }).first();
    await menuButton.click();
    await expect(page.getByRole("dialog")).toBeVisible();
  });
});
