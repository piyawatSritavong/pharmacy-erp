import { expect, test, type Locator, type Page, type TestInfo } from "@playwright/test";
import { passwordFor } from "./credentials";
import type { NavigationItem, Session } from "../src/types";

const phoneWidths = [320, 360, 375, 390, 414, 430];
const widths = [...phoneWidths, 639, 640, 768, 1024, 1440];

test.skip(!process.env.E2E_RUN, "Requires the local seeded backend (E2E_RUN=1)");

async function signIn(page: Page, email = "superadmin@erp.local"): Promise<Session> {
  const response = await page.request.post("/api/backend/auth/login", {
    data: { email, password: passwordFor(email) }
  });
  expect(response.ok()).toBeTruthy();
  const session = await page.request.get("/api/backend/me");
  expect(session.ok()).toBeTruthy();
  return session.json();
}

function leaves(items: NavigationItem[]): NavigationItem[] {
  return items.flatMap((item) => item.children?.length ? leaves(item.children) : [item]);
}

async function noPageOverflow(page: Page) {
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth), { message: `Page overflow at ${page.url()} / ${page.viewportSize()?.width}px` }).toBeLessThanOrEqual(1);
}

async function fitsViewport(locator: Locator, page: Page) {
  await expect(locator).toBeVisible();
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.y).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(page.viewportSize()!.width + 1);
  expect(box!.y + box!.height).toBeLessThanOrEqual(page.viewportSize()!.height + 1);
  expect(await locator.evaluate((el) => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1);
}

async function capture(page: Page, info: TestInfo, name: string) {
  const path = info.outputPath(`${name}.png`);
  await page.screenshot({ path });
  await info.attach(name, { path, contentType: "image/png" });
}

async function expandTree(nav: Locator, items: NavigationItem[]) {
  for (const item of items) {
    if (item.children?.length) {
      const toggle = nav.getByRole("button", { name: item.title, exact: true });
      if (await toggle.getAttribute("aria-expanded") !== "true") await toggle.click();
      await expandTree(nav, item.children);
    } else {
      await expect(nav.locator(`a[href="${item.href}"]`)).toBeVisible();
    }
  }
}

for (const [portal, email, start] of [
  ["Admin", "superadmin@erp.local", "/dashboard"],
  ["POS", "pos.mes@erp.local", "/sales"]
] as const) {
  test(`${portal}: complete navigation and responsive route sweep`, async ({ page }, info) => {
    const session = await signIn(page, email);
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.setViewportSize({ width: 320, height: 568 });
    await page.goto(start);
    const trigger = page.getByRole("button", { name: "เปิดเมนูหลัก", exact: true });
    await trigger.click();
    const drawer = page.getByRole("dialog", { name: "PharmaPOS" });
    await fitsViewport(drawer, page);
    const nav = drawer.getByRole("navigation");
    const before = page.url();
    await expandTree(nav, session.navigation);
    expect(page.url()).toBe(before); // Expanding a section must not navigate.
    await capture(page, info, `${portal}-navigation-320`);
    await page.keyboard.press("Escape");
    await expect(drawer).toHaveCount(0);
    await expect(trigger).toBeFocused();
    await trigger.click();
    await expandTree(drawer.getByRole("navigation"), session.navigation);
    const destination = leaves(session.navigation).find((item) => item.href !== start)!;
    await drawer.getByRole("link", { name: destination.title, exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`${destination.href}$`));
    await expect(drawer).toHaveCount(0);

    const routes = [...new Set([...leaves(session.navigation).map((item) => item.href), ...(portal === "Admin" ? ["/global-reports", "/sales-history", "/audit"] : [])])];
    for (const route of routes) {
      await page.goto(route);
      await expect(page.locator("main")).toBeVisible();
      for (const width of widths) {
        await page.setViewportSize({ width, height: 844 });
        await noPageOverflow(page);
        if (width < 640) await expect(page.getByRole("button", { name: "เปิดเมนูหลัก", exact: true })).toBeVisible();
        if (width === 1440 && portal === "Admin") await expect(page.getByRole("navigation", { name: "เมนูหลัก", exact: true })).toBeVisible();
      }
    }
    expect(errors).toEqual([]);
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(start);
    await capture(page, info, `${portal}-390`);
  });
}

test("Admin: filters, record cards, forms, bounded dropdowns and short dialogs", async ({ page }, info) => {
  await signIn(page);
  await page.goto("/dashboard");
  const filters = page.getByRole("button", { name: /ตัวกรองและช่วงวันที่/ });
  await expect(filters).toHaveAttribute("aria-expanded", "false");
  await filters.click();
  for (const width of phoneWidths) {
    await page.setViewportSize({ width, height: 844 });
    await noPageOverflow(page);
    await page.getByRole("combobox", { name: "กรองตามสถานะการปิดรอบ" }).click();
    await fitsViewport(page.getByRole("listbox"), page);
    await page.keyboard.press("Escape");
  }
  await page.getByRole("combobox", { name: "กรองตามประเภทชำระเงิน" }).click();
  await page.getByRole("option", { name: "เงินโอน", exact: true }).click();
  await expect(page).toHaveURL(/payment_type=bank_transfer/);
  await page.getByRole("button", { name: "วันก่อนหน้า" }).click();
  await expect(page).toHaveURL(/date_from=/);
  await noPageOverflow(page);

  for (const [route, action] of [
    ["/product-catalog", "เพิ่มสินค้าใหม่"],
    ["/product-categories", "เพิ่มหมวดสินค้า"],
    ["/suppliers", "เพิ่มบริษัทคู่ค้า"],
    ["/purchase-orders", "สร้างใบสั่งซื้อเข้า"]
  ]) {
    await page.goto(route);
    const card = page.locator(".mobile-card-table tbody tr").first();
    if (await card.count()) {
      await expect(card).toBeVisible();
      await expect(card.locator("[data-actions]")).toBeVisible();
    } else {
      await expect(page.getByText("ไม่มีรายการแสดง", { exact: true })).toBeVisible();
    }
    const opener = page.getByRole("button", { name: action, exact: true });
    await opener.click();
    const dialog = page.getByRole("dialog").last();
    for (const width of phoneWidths) {
      await page.setViewportSize({ width, height: 568 });
      await fitsViewport(dialog, page);
    }
    await page.setViewportSize({ width: 320, height: 480 });
    await fitsViewport(dialog, page);
    const lastAction = dialog.getByRole("button").last();
    await lastAction.scrollIntoViewIfNeeded();
    await expect(lastAction).toBeInViewport();
    const close = dialog.getByRole("button", { name: "ปิดหน้าต่าง", exact: true });
    await expect(close).toBeInViewport();
    await capture(page, info, `${route.slice(1)}-dialog-320x480`);
    await close.click();
    await expect(dialog).toHaveCount(0);
    await expect(opener).toBeFocused();
  }
});

test("POS: quantities, discounts, all payment types, receipt and cart persistence", async ({ page }, info) => {
  await signIn(page, "pos.mes@erp.local");
  // Exercise real inventory/lot selection and backend previews. Intercept only
  // the final sale so this UI regression never writes invoices or reduces stock.
  let checkoutBody: Record<string, unknown> | undefined;
  await page.route("**/api/backend/pos/checkout", async (route) => {
    checkoutBody = route.request().postDataJSON();
    await route.fulfill({ json: { invoice_id: "mobile-ui-test", invoice_number: "UI-TEST-001", total_amount: 100, cash_amount: 100, transfer_amount: 0, tendered_amount: 100, change_amount: 0 } });
  });
  await page.goto("/sales");
  await page.getByRole("button", { name: /เพิ่ม .* ลงตะกร้า/ }).first().click();
  const lot = page.getByRole("dialog").filter({ has: page.getByRole("heading", { name: /เลือก Lot/ }) });
  await lot.getByRole("button", { name: /^Lot / }).first().click();
  const cart = page.getByRole("dialog", { name: "รายการขายปัจจุบัน", exact: true });
  await fitsViewport(cart, page);
  await cart.getByRole("button", { name: /^เพิ่มจำนวน / }).click();
  await expect(cart.getByText("2", { exact: true })).toBeVisible();
  await cart.getByRole("button", { name: /^ลดจำนวน / }).click();
  await cart.getByRole("textbox", { name: /^ส่วนลด / }).fill("0");
  await cart.getByRole("textbox", { name: "ส่วนลดท้ายบิล", exact: true }).fill("0");
  await cart.getByRole("button", { name: "ปิดตะกร้า" }).click();
  await page.getByRole("button", { name: /^ตะกร้า/ }).click();
  await expect(cart.getByRole("button", { name: /^เพิ่มจำนวน / })).toBeVisible();
  await cart.getByRole("button", { name: "รับชำระเงิน", exact: true }).click();
  const payment = page.getByRole("dialog", { name: "รับชำระเงิน", exact: true });
  for (const method of ["เงินสด", "เงินโอน", "เงินสด + เงินโอน"]) {
    await payment.getByRole("combobox", { name: "ช่องทางชำระเงิน" }).click();
    await page.getByRole("option", { name: method, exact: true }).click();
    await expect(payment.getByRole("button", { name: "ยืนยันการชำระเงิน" })).toBeEnabled();
    for (const width of phoneWidths) {
      await page.setViewportSize({ width, height: 568 });
      await fitsViewport(payment, page);
    }
    await page.setViewportSize({ width: 320, height: 480 });
    const pay = payment.getByRole("button", { name: "ยืนยันการชำระเงิน" });
    await pay.scrollIntoViewIfNeeded();
    await expect(pay).toBeInViewport();
    await expect(payment.getByRole("button", { name: "ปิดหน้าชำระเงิน" })).toBeInViewport();
    await capture(page, info, `payment-${method}-320x480`);
  }
  await payment.getByRole("button", { name: "ยืนยันการชำระเงิน" }).click();
  await expect(page.getByRole("dialog", { name: "ชำระเงินเสร็จสิ้น" })).toBeVisible();
  expect(checkoutBody?.payment_type).toBe("mixed");
  expect((checkoutBody?.items as unknown[]).length).toBe(1);
  await page.getByRole("button", { name: "เริ่มรายการใหม่" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", { name: /^ตะกร้า/ }).click();
  await expect(cart.getByRole("button", { name: "รับชำระเงิน", exact: true })).toBeDisabled();
});

test("POS: mobile requisition product picker and quantities fit the dialog", async ({ page }) => {
  await signIn(page, "pos.mes@erp.local");
  await page.setViewportSize({ width: 320, height: 568 });
  await page.goto("/requisitions");
  await page.getByRole("button", { name: "สร้างใบเบิกสินค้า", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "สร้างใบเบิกสินค้า", exact: true });
  const input = dialog.getByRole("combobox", { name: "เลือกสินค้าที่ต้องการเบิก" });
  await input.fill("0.5คิว");
  const results = dialog.getByRole("listbox");
  const option = results.getByRole("option", { name: "0.5คิว", exact: true });
  await expect(option).toBeVisible();
  await option.scrollIntoViewIfNeeded();
  await expect(option).toBeInViewport();
  await option.click();
  await expect(input).toHaveValue(/0.5คิว/);
  await fitsViewport(dialog, page);
  await dialog.getByRole("spinbutton", { name: "จำนวนที่ต้องการ" }).fill("2");
  await expect(dialog.getByRole("button", { name: "ส่งใบเบิก", exact: true })).toBeEnabled();
  await dialog.getByRole("button", { name: "ปิดหน้าต่าง", exact: true }).click();
  await expect(dialog).toHaveCount(0);
});
