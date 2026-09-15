import type { Browser, BrowserContext, Page } from "@playwright/test";
import { expect, test } from "@playwright/test";

import { passwordFor } from "./credentials";

type SessionHandle = {
  context: BrowserContext;
  page: Page;
};

async function signIn(page: Page, email: string, expectedPath: string) {
  await page.goto("/login");
  await page.getByLabel("อีเมล").fill(email);
  await page.getByLabel("รหัสผ่าน").fill(passwordFor(email));
  await page.getByRole("button", { name: "เข้าสู่ระบบ" }).click();
  await page.waitForURL(new RegExp(`${expectedPath}$`));
}

async function openSession(
  browser: Browser,
  email: string,
  expectedPath: string,
): Promise<SessionHandle> {
  const context = await browser.newContext();
  const page = await context.newPage();
  await signIn(page, email, expectedPath);
  return { context, page };
}

test.describe("สิทธิ์และการนำทางสองบทบาท", () => {
  test.describe.configure({ mode: "serial" });
  test.skip(!process.env.E2E_RUN, "กำหนด E2E_RUN=1 เมื่อเปิดบริการแล้ว");

  // This used to assert that the login page listed every account and its
  // password, which is the opposite of what a public page should do. It now
  // asserts they are absent: the page is reachable without a session, so
  // anything on it is published to anyone who visits.
  test("หน้าเข้าสู่ระบบต้องไม่เปิดเผยบัญชีหรือรหัสผ่าน", async ({ page }) => {
    await page.goto("/login");
    await expect(page.getByRole("button", { name: "เข้าสู่ระบบ" })).toBeVisible();

    for (const account of [
      "superadmin@erp.local",
      "admin.central@erp.local",
      "pos.mes@erp.local",
      "pos.phahol@erp.local",
      "pos.phasuk@erp.local",
      "pos.nakhonpathom@erp.local",
      "pos.knp@erp.local"
    ]) {
      await expect(page.getByText(account)).toHaveCount(0);
    }
    await expect(page.getByText(/รหัสผ่านทดสอบ/)).toHaveCount(0);
    await expect(page.getByText(/DevPassword/i)).toHaveCount(0);

    // And the form starts empty, so a visitor cannot submit somebody else's
    // account by pressing the button.
    await expect(page.getByLabel("อีเมล")).toHaveValue("");
    await expect(page.getByLabel("รหัสผ่าน")).toHaveValue("");
  });

  test("ผู้ใช้สองบทบาทเข้าสู่ระบบพร้อมกันและเห็นเมนูของตนเอง", async ({
    browser,
  }) => {
    const sessions = await Promise.all([
      openSession(browser, "superadmin@erp.local", "/dashboard"),
      openSession(browser, "pos.mes@erp.local", "/sales"),
    ]);
    const [admin, pos] = sessions.map((session) => session.page);

    // The sidebar is an accordion: one parent group is open at a time, so the
    // links on screen are the open group's children. Walk every group.
    const adminNav = admin.getByRole("navigation", { name: "เมนูหลัก", exact: true });
    // ขายหน้าร้าน is a standalone link in the nav (not a group), so it stays
    // visible alongside whichever group is open — count it in each total.
    await expect(adminNav.getByRole("link", { name: "ขายหน้าร้าน", exact: true })).toBeVisible();
    const adminGroups: Array<[string, string[]]> = [
      ["รายงาน", ["Dashboard", "สรุปสิ้นเดือน", "รายงานสรุปสิ้นเดือน"]],
      ["คลังสินค้า", ["รายการสินค้า", "สต๊อกจริง", "สต๊อกผี", "หมวดสินค้า", "โปรโมชั่น", "เบิกสินค้า", "โอนสินค้า"]],
      ["ใบเอกสาร", ["ใบสั่งซื้อเข้า", "บริษัทคู่ค้า", "เคลม/คืนสินค้า", "รพ.สต.", "ใบขาย", "อย."]],
      ["ระบบ", ["ตั้งค่า", "ประวัติระบบ", "ประวัติการขาย"]],
    ];
    for (const [group, children] of adminGroups) {
      // The group holding the current route is already open; clicking it again
      // would close it.
      const trigger = adminNav.getByRole("button", { name: group, exact: true });
      if ((await trigger.getAttribute("aria-expanded")) !== "true") {
        await trigger.click();
      }
      // +1 for the standalone ขายหน้าร้าน link that is always present.
      await expect(adminNav.getByRole("link")).toHaveCount(children.length + 1);
      for (const child of children) {
        await expect(adminNav.getByRole("link", { name: child, exact: true })).toBeVisible();
      }
    }

    const posNav = pos.getByRole("navigation", { name: "เมนูจุดขาย", exact: true });
    // Non-exact: a nav item may carry a red count badge (e.g. "พักบิล 1"), so
    // the accessible name is the label plus its count.
    const posLinks = [
      "ขายหน้าร้าน",
      "พักบิล",
      "ประวัติ",
      "เบิกสินค้า",
      "รับโอนสินค้า",
      "เคลม/คืนสินค้า",
      // A branch runs its own promotions, so the till has a way in.
      "โปรโมชั่น",
      "สรุปยอดขาย",
    ];
    await expect(posNav.getByRole("link")).toHaveCount(posLinks.length);
    for (const name of posLinks) {
      await expect(posNav.getByRole("link", { name }).first()).toBeVisible();
    }
    await expect(
      pos.getByRole("link", { name: "ตั้งค่า", exact: true }),
    ).toHaveCount(0);

    await Promise.all(sessions.map((session) => session.context.close()));
  });

  // Categories are created, renamed and deleted; business-flow.md removed the
  // archive/reactivate pair, so the console offers แก้ไข and ลบ only.
  test("สร้าง แก้ไข และลบหมวดสินค้า", async ({ browser }) => {
    const session = await openSession(
      browser,
      "superadmin@erp.local",
      "/dashboard",
    );
    const { page } = session;
    const suffix = Date.now();
    const originalName = `หมวด E2E ${suffix}`;
    const editedName = `${originalName} แก้ไข`;
    let categoryID = "";

    try {
      await page.goto("/product-categories");
      await expect(
        page.getByRole("heading", { name: "หมวดสินค้า", exact: true }),
      ).toBeVisible();
      await page.getByRole("button", { name: "เพิ่มหมวดสินค้า" }).click();
      const dialog = page.getByRole("dialog");
      await dialog.getByLabel("ชื่อหมวดสินค้า").fill(originalName);
      const createResponsePromise = page.waitForResponse(
        (response) =>
          response.url().includes("/api/backend/product-categories") &&
          response.request().method() === "POST",
      );
      await dialog.getByRole("button", { name: "บันทึก" }).click();
      const createResponse = await createResponsePromise;
      expect(createResponse.status()).toBe(201);
      categoryID = String((await createResponse.json()).id);
      await expect(page.getByText(originalName, { exact: true })).toBeVisible();

      await page.getByRole("button", { name: `แก้ไข ${originalName}` }).click();
      await page.getByRole("dialog").getByLabel("ชื่อหมวดสินค้า").fill(editedName);
      await page.getByRole("dialog").getByRole("button", { name: "บันทึก" }).click();
      await expect(page.getByText(editedName, { exact: true })).toBeVisible();

      page.once("dialog", (prompt) => prompt.accept());
      const deleteResponsePromise = page.waitForResponse(
        (response) =>
          response.url().includes(`/api/backend/product-categories/${categoryID}`) &&
          response.request().method() === "DELETE",
      );
      await page.getByRole("button", { name: `ลบ ${editedName}` }).click();
      expect((await deleteResponsePromise).status()).toBe(200);
      await expect(page.getByText(editedName, { exact: true })).toHaveCount(0);
      categoryID = "";
    } finally {
      if (categoryID) {
        await page.request.delete(`/api/backend/product-categories/${categoryID}`);
      }
      await session.context.close();
    }
  });

  // Scope, not counts: what each POS account sees must equal what an admin sees
  // when filtering the catalogue to that same branch. Hard-coded totals drift
  // with every reseed and hide the rule they were meant to protect.
  test("POS แต่ละบัญชีเห็นสินค้าเท่าที่สาขาตนมีเท่านั้น", async ({ browser }) => {
    const accounts = [
      ["pos.mes@erp.local", "MES"],
      ["pos.phahol@erp.local", "PHH"],
      ["pos.phasuk@erp.local", "PHS"],
      ["pos.nakhonpathom@erp.local", "NPT"],
    ] as const;

    const admin = await openSession(browser, "superadmin@erp.local", "/dashboard");
    const branches = await admin.page.evaluate(async () => {
      const result = await fetch("/api/backend/branches", { cache: "no-store" });
      return (await result.json()).items as Array<{ id: string; code: string }>;
    });

    for (const [email, branchCode] of accounts) {
      const branch = branches.find((item) => item.code === branchCode);
      expect(branch, `branch ${branchCode} must exist`).toBeTruthy();
      const scoped = await admin.page.evaluate(async (branchID) => {
        const result = await fetch(
          `/api/backend/products?search=OCH-&page=1&page_size=1&branch_id=${branchID}`,
          { cache: "no-store" },
        );
        return { status: result.status, body: await result.json() };
      }, branch!.id);
      expect(scoped.status).toBe(200);

      const session = await openSession(browser, email, "/sales");
      const response = await session.page.evaluate(async () => {
        const result = await fetch("/api/backend/products?search=OCH-&page=1&page_size=1", {
          cache: "no-store",
        });
        return { status: result.status, body: await result.json() };
      });
      expect(response.status).toBe(200);
      expect(response.body.pagination.total).toBe(scoped.body.pagination.total);
      await session.context.close();
    }
    await admin.context.close();
  });

  test("ผู้ดูแลเปิดทุกหน้าจาก sidebar ได้จริง", async ({ browser }) => {
    const session = await openSession(
      browser,
      "superadmin@erp.local",
      "/dashboard",
    );
    const pages = [
      ["สต๊อกจริง", "/real-inventory", "สต๊อกจริง"],
      ["สต๊อกผี", "/ghost-inventory", "สต๊อกผี"],
      ["รายการสินค้า", "/product-catalog", "รายการสินค้า"],
      ["หมวดสินค้า", "/product-categories", "หมวดสินค้า"],
      ["ใบสั่งซื้อเข้า", "/purchase-orders", "ใบสั่งซื้อเข้า"],
      ["บริษัทคู่ค้า", "/suppliers", "บริษัทคู่ค้า"],
      ["โอนสินค้า", "/transfers", "โอนสินค้า"],
      ["สรุปสิ้นเดือน", "/month-end", "สรุปสิ้นเดือน"],
      ["รายงานสรุปสิ้นเดือน", "/month-end-report", "รายงานสรุปสิ้นเดือน"],
      ["รายงาน", "/global-reports", "รายงาน"],
      ["ตั้งค่า", "/settings", "ตั้งค่า"],
      ["แดชบอร์ด", "/dashboard", "Dashboard"],
    ];

    for (const [, path, heading] of pages) {
      await session.page.goto(path);
      await session.page.waitForURL(new RegExp(`${path}$`));
      await expect(
        session.page.getByRole("heading", { level: 1, name: heading, exact: true }),
      ).toBeVisible();
    }
    // รพ.สต. and ใบขาย are Pro features now: the page renders the upgrade gate.
    for (const path of ["/government-sales", "/sales-management"]) {
      await session.page.goto(path);
      await session.page.waitForURL(new RegExp(`${path}$`));
      await expect(session.page.getByText("PharmaPOS Pro").first()).toBeVisible();
      await expect(session.page.getByRole("button", { name: "สมัคร Pro รายเดือน" })).toBeVisible();
    }
    await session.context.close();
  });

  test("Superadmin ตรวจช่วงวันที่ รายละเอียดจริง และคำเตือนที่ไม่ปิดปุ่มยืนยัน", async ({
    browser,
  }, testInfo) => {
    const session = await openSession(
      browser,
      "superadmin@erp.local",
      "/dashboard",
    );
    // The user's canonical example: 600 sold → hide CA002 (Ghost covers) →
    // CA003/CB001 recorded at cost × 1.05 = 75 → target 450.
    const planSummary = {
      original_revenue: 600,
      cash_no_tax_revenue: 300,
      hidden_revenue: 100,
      suppressed_revenue: 100,
      base_revenue: 500,
      repriced_original_revenue: 200,
      repriced_final_revenue: 150,
      adjustment_reduction: 50,
      unchanged_revenue: 300,
      final_revenue: 450,
      target_revenue: 450,
      invoice_count: 6,
      suppressed_invoice_count: 1,
      hidden_invoice_count: 1,
      repriced_invoice_count: 2,
      unchanged_invoice_count: 3,
      adjusted_item_count: 2,
      missing_cost_item_count: 0,
      adjustment_percent: 5,
      minimum_adjustment_percent: 5,
      maximum_adjustment_percent: 10,
    };
    await session.page.route("**/api/backend/accounting/month-end/reconciliation-overview", async (route) => {
      await route.fulfill({
        contentType: "application/json",
        json: {
          ...planSummary,
          invoice_groups: {
            cash_hidden_ghost: { invoice_count: 1, revenue: 100 },
            cash_repriced: { invoice_count: 2, revenue: 200 },
            cash_full_tax: { invoice_count: 0, revenue: 0 },
            bank_transfer: { invoice_count: 3, revenue: 300 },
            mixed: { invoice_count: 0, revenue: 0 },
            unclassified: { invoice_count: 0, revenue: 0 },
          },
        },
        status: 200,
      });
    });
    await session.page.route("**/api/backend/accounting/month-end/reconciliation-preview", async (route) => {
      await route.fulfill({
        contentType: "application/json",
        json: {
          ...planSummary,
          suppression_candidates: [{
            id: "invoice-ca002",
            branch_name: "คณาเภสัช",
            invoice_number: "CA002",
            customer_name: "ลูกค้าทดสอบ",
            created_at: "2026-06-11T03:00:00Z",
            total_amount: 100,
            final_total: 100,
            variance_amount: 0,
            classification: "hidden_ghost",
            items: [{ id: "item-ca002", product_id: "product-ghost", product_name: "สินค้าสาธิต A", sku: "DEMO-GHOST", quantity: 1, unit_price: 100, line_total: 100, cost_basis: 71.43, new_unit_price: 100, new_line_total: 100, variance_amount: 0, repriced: false, missing_cost: false, ghost_stock_available: 1 }],
          }],
          repriced_invoices: [{
            id: "invoice-ca003",
            branch_name: "คณาเภสัช",
            invoice_number: "CA003",
            customer_name: "ลูกค้าทดสอบ",
            created_at: "2026-06-12T03:00:00Z",
            total_amount: 100,
            final_total: 75,
            variance_amount: 25,
            classification: "repriced_cost_markup",
            items: [{ id: "item-ca003", product_id: "product-none", product_name: "สินค้าสาธิต B", sku: "DEMO-NONE", quantity: 1, unit_price: 100, line_total: 100, cost_basis: 71.43, new_unit_price: 75, new_line_total: 75, variance_amount: 25, repriced: true, missing_cost: false, ghost_stock_available: 0 }],
          }],
          stock_projection: {
            branch_real_returned: 1,
            warehouse_real_received: 1,
            warehouse_ghost_deducted: 1,
            ghost_deficit_created: 0,
            products: [{ product_id: "product-ghost", product_name: "สินค้าสาธิต A", quantity: 1, warehouse_ghost_before: 1, warehouse_ghost_after: 0, deficit_created: 0 }],
          },
        },
        status: 200,
      });
    });
    await session.page.goto("/month-end");
    await expect(
      session.page.getByRole("heading", { name: "สรุปสิ้นเดือน", exact: true }),
    ).toBeVisible();
    await expect(session.page.getByLabel("วันที่เริ่มต้น")).toBeVisible();
    await expect(session.page.getByLabel("วันที่สิ้นสุด")).toBeVisible();
    await expect(session.page.getByLabel("กำไรเหนือต้นทุน (%)")).toHaveValue("5");
    await expect(session.page.getByLabel("ยอดเป้าหมาย")).toBeDisabled();
    await session.page
      .getByRole("button", { name: "ตรวจสอบใบขายและคำนวณยอดเป้าหมาย" })
      .click();
    await expect(session.page.getByText("รายละเอียดกลุ่มบิลและสูตรจากข้อมูลจริง")).toBeVisible();
    await expect(session.page.getByLabel("ยอดเป้าหมาย")).toHaveValue("฿450.00");
    await expect(session.page.getByText("ยอดเป้าหมาย = ฿450.00")).toBeVisible();
    await expect(session.page.getByText("ใบขายที่จะซ่อน 1 ใบ")).toBeVisible();
    await expect(session.page.getByText("ใบขายที่จะบันทึกที่ต้นทุน + 5% · 2 ใบ")).toBeVisible();
    await expect(session.page.getByText("฿100.00 → ฿75.00", { exact: false })).toBeVisible();
    await expect(session.page.getByRole("button", { name: "ยืนยันและสรุปรอบ" })).toBeEnabled();
    await session.page.screenshot({
      path: testInfo.outputPath("month-end-preview.png"),
      fullPage: true,
    });
    await session.page.getByRole("button", { name: "ยืนยันและสรุปรอบ" }).click();
    await expect(session.page.getByRole("dialog").getByRole("heading", { name: "ยืนยันการสรุปรอบ" })).toBeVisible();
    await session.page.screenshot({
      path: testInfo.outputPath("month-end-confirmation.png"),
      fullPage: true,
    });
    await session.context.close();
  });

  test("พนักงานขายเปิดทุกหน้าจาก top nav ได้จริง", async ({ browser }) => {
    const session = await openSession(browser, "pos.mes@erp.local", "/sales");
    const pages = [
      ["ประวัติ", "/sales-history", "ประวัติ"],
      ["เบิกสินค้า", "/requisitions", "เบิกสินค้า"],
      ["รับโอนสินค้า", "/transfer-receipts", "รับโอนสินค้า"],
      ["สรุปยอดขาย", "/daily-sales", /สรุปยอดขาย/],
      ["ขายหน้าร้าน", "/sales", "ขายหน้าร้าน"],
    ] as const;

    for (const [linkName, path, heading] of pages) {
      // Non-exact: เบิกสินค้า / รับโอนสินค้า can carry a red count badge, so the
      // link's accessible name is the label plus its unseen count (e.g.
      // "เบิกสินค้า 2").
      await session.page
        .getByRole("link", { name: linkName })
        .first()
        .click();
      await session.page.waitForURL((url) => url.pathname === path);
      await expect(
        session.page.getByRole("heading", { level: 1, name: heading }),
      ).toBeVisible();
    }
    await session.context.close();
  });

  test("POS ใช้หน้าขายแบบราคาเดียวและซ่อนข้อมูลภาษีจนกว่าจะร้องขอ", async ({
    browser,
  }) => {
    const session = await openSession(browser, "pos.mes@erp.local", "/sales");
    await expect(session.page.getByLabel("ระดับราคา")).toHaveCount(0);
    await expect(
      session.page.getByText("โหมดราชการ", { exact: true }),
    ).toHaveCount(0);
    await expect(
      session.page.getByText("สินค้าทั้งหมด", { exact: true }),
    ).toHaveCount(0);
    await expect(session.page.getByLabel("ชื่อลูกค้า")).toHaveCount(0);
    await expect(session.page.getByLabel("เลขประจำตัวผู้เสียภาษี")).toHaveCount(
      0,
    );

    await session.page.getByLabel("ออกใบกำกับภาษีเต็มรูป").check();
    await expect(session.page.getByLabel("ชื่อลูกค้า")).toBeVisible();
    await expect(
      session.page.getByLabel("เลขประจำตัวผู้เสียภาษี"),
    ).toBeVisible();
    await session.page.getByLabel("ออกใบกำกับภาษีเต็มรูป").uncheck();

    await session.page.route("**/api/backend/pos/checkout", async (route) => {
      await route.fulfill({
        contentType: "application/json",
        json: {
          invoice_id: "invoice-ui-test",
          invoice_number: "BL-UI-TEST",
          total_amount: 100,
          cash_amount: 40,
          transfer_amount: 60,
          tendered_amount: 50,
          change_amount: 10,
        },
        status: 200,
      });
    });
    // The grid lists the whole catalogue and marks anything without stock as
    // "หมด", so pick a card that can actually be sold rather than whichever
    // product happens to sort first.
    const sellable = session.page
      .getByRole("button", { name: /^เพิ่ม .* ลงตะกร้า$/ })
      .filter({ hasText: "+ เพิ่ม" });
    await expect(sellable.first()).toBeVisible();
    await sellable.first().click();
    // Selling a tracked product picks a lot first: one cart line, one lot.
    const lotDialog = session.page.getByRole("dialog");
    if (await lotDialog.isVisible().catch(() => false)) {
      await lotDialog.getByRole("button").filter({ hasText: /^Lot / }).first().click();
    }
    await session.page.getByRole("button", { name: "รับชำระเงิน" }).click();
    const paymentDialog = session.page.getByRole("dialog");
    await expect(paymentDialog).toBeVisible();
    await paymentDialog.getByLabel("ช่องทางชำระเงิน").click();
    await session.page
      .getByRole("option", { name: "เงินสด + เงินโอน" })
      .click();
    const cashInput = paymentDialog.getByLabel("เงินสดที่ต้องชำระ");
    const transferInput = paymentDialog.getByLabel("ยอดเงินโอน");
    await expect(cashInput).toBeVisible();
    await expect(transferInput).toBeVisible();
    await expect(paymentDialog.getByLabel("เงินสดที่รับจากลูกค้า")).toHaveCount(
      0,
    );
    await expect(paymentDialog.getByLabel("เลขอ้างอิงการโอน")).toHaveCount(0);
    const cashBox = await cashInput.boundingBox();
    const transferBox = await transferInput.boundingBox();
    expect(cashBox).not.toBeNull();
    expect(transferBox).not.toBeNull();
    expect(cashBox!.x).toBeLessThan(transferBox!.x);
    const totalText = await paymentDialog
      .getByText(/^฿[\d,.]+$/)
      .first()
      .textContent();
    const total = Number((totalText || "0").replace(/[฿,]/g, ""));
    await cashInput.fill("100.00");
    await expect(transferInput).toHaveValue((total - 100).toFixed(2));
    await transferInput.fill("100.00");
    await expect(cashInput).toHaveValue((total - 100).toFixed(2));

    const targetCash = Number((total * 0.4).toFixed(2));
    const tenderedCash = targetCash + 100;
    await transferInput.fill((total - targetCash).toFixed(2));
    await expect(cashInput).toHaveValue(targetCash.toFixed(2));
    await cashInput.fill(tenderedCash.toFixed(2));
    await expect(transferInput).toHaveValue((total - targetCash).toFixed(2));
    await expect(
      paymentDialog
        .getByText("เงินสดสุทธิหลังหักเงินทอน", { exact: true })
        .locator("..")
        .getByText(`฿${targetCash.toFixed(2)}`, { exact: true }),
    ).toBeVisible();
    await expect(
      paymentDialog
        .getByText("เงินทอน", { exact: true })
        .locator("..")
        .getByText("฿100.00", { exact: true }),
    ).toBeVisible();
    await cashInput.fill("0");
    await expect(
      paymentDialog.getByText("ยอดเงินสดต้องมากกว่า 0", { exact: true }),
    ).toBeVisible();
    await expect(
      paymentDialog.getByRole("button", { name: "ยืนยันการชำระเงิน" }),
    ).toBeDisabled();
    await cashInput.fill("-1");
    await expect(cashInput).not.toHaveValue("-1");
    await cashInput.fill(tenderedCash.toFixed(2));
    await expect(
      paymentDialog.getByRole("button", { name: "ตรวจสอบยอดและคำนวณเงินทอน" }),
    ).toHaveCount(0);
    await paymentDialog
      .getByRole("button", { name: "ยืนยันการชำระเงิน" })
      .click();
    await expect(
      paymentDialog.getByRole("heading", { name: "ชำระเงินเสร็จสิ้น", exact: true }),
    ).toBeVisible();
    await expect(
      paymentDialog.getByText("BL-UI-TEST", { exact: true }),
    ).toBeVisible();
    await expect(
      paymentDialog.getByRole("link", { name: "พิมพ์ใบเสร็จ" }),
    ).toBeVisible();
    await paymentDialog
      .getByRole("button", { name: "เริ่มรายการใหม่" })
      .click();
    await expect(paymentDialog).toHaveCount(0);

    // เบิกสินค้า is its own menu now; a POS cashier raises a multi-line
    // requisition with no source/destination branch — the admin decides those.
    await session.page.goto("/requisitions");
    await expect(session.page.getByRole("heading", { name: "ใบเบิกสินค้าของสาขา" })).toBeVisible();
    await session.page.getByRole("button", { name: "สร้างใบเบิกสินค้า" }).click();
    const requisitionDialog = session.page.getByRole("dialog");
    await expect(requisitionDialog.getByRole("heading", { name: "สร้างใบเบิกสินค้า" })).toBeVisible();
    await expect(requisitionDialog.getByLabel("เลือกสินค้าที่ต้องการเบิก")).toBeVisible();
    await expect(requisitionDialog.getByLabel("จำนวนที่ต้องการ")).toBeVisible();
    // A POS cashier names neither source nor destination — the admin dest
    // selector (its own labelled control) is absent from this dialog.
    await expect(requisitionDialog.getByRole("combobox", { name: "สาขาที่ขอเบิก" })).toHaveCount(0);
    await session.context.close();
  });

  test("กันสิทธิ์ข้ามบทบาทและไม่มี route งานผู้ดูแลสาขาเดิม", async ({
    browser,
  }) => {
    const admin = await openSession(
      browser,
      "superadmin@erp.local",
      "/dashboard",
    );
    await admin.page.goto("/sales");
    await admin.page.waitForURL(/\/dashboard$/);
    await admin.page.goto("/branch-dashboard");
    await expect(admin.page.getByText("ไม่พบหน้าที่ต้องการ")).toBeVisible();
    await admin.context.close();

    const pos = await openSession(browser, "pos.mes@erp.local", "/sales");
    await pos.page.goto("/settings");
    await pos.page.waitForURL(/\/sales$/);
    await pos.context.close();
  });

  test("Central Admin, Branch Admin และ POS เข้า Month-End/Ghost API ไม่ได้", async ({ browser }) => {
    const accounts = [
      ["admin.central@erp.local", "/dashboard"],
      ["pos.mes@erp.local", "/sales"],
    ] as const;

    for (const [email, homePath] of accounts) {
      const session = await openSession(browser, email, homePath);
      const access = await session.page.evaluate(async () => {
        const [me, monthEnd, report, ghostLots] = await Promise.all([
          fetch("/api/backend/me", { cache: "no-store" }),
          fetch("/api/backend/accounting/month-end/reconciliations", { cache: "no-store" }),
          fetch("/api/backend/admin/month-end-report?period=2026-08", { cache: "no-store" }),
          fetch("/api/backend/inventory/lots?branch_id=00000000-0000-4000-8000-000000000001&product_id=00000000-0000-4000-8000-000000000002&stock_bucket=ghost", { cache: "no-store" }),
        ]);
        return { navigation: (await me.json()).navigation, statuses: [monthEnd.status, report.status, ghostLots.status] };
      });
      expect(JSON.stringify(access.navigation)).not.toContain('"month_end"');
      expect(JSON.stringify(access.navigation)).not.toContain('"month_end_report"');
      expect(access.statuses[0]).toBe(403);
      expect(access.statuses[1]).toBe(403);
      expect(access.statuses[2]).toBe(403);
      await session.page.goto("/month-end");
      await session.page.waitForURL(new RegExp(`${homePath}$`));
      await session.context.close();
    }
  });

  test("Ghost write API ตอบ 400 สำหรับ Superadmin และ 403 สำหรับ role อื่น", async ({ browser }) => {
    const accounts = [
      ["superadmin@erp.local", "/dashboard", 400],
      ["admin.central@erp.local", "/dashboard", 403],
      ["pos.mes@erp.local", "/sales", 403],
    ] as const;

    for (const [email, homePath, expectedStatus] of accounts) {
      const session = await openSession(browser, email, homePath);
      const statuses = await session.page.evaluate(async () => {
        const headers = { "Content-Type": "application/json" };
        const responses = await Promise.all([
          fetch("/api/backend/inventory/adjust", {
            method: "POST", headers,
            body: JSON.stringify({ product_id: "product", branch_id: "branch", stock_bucket: "ghost", quantity_delta: 1, reason: "policy-test" }),
          }),
          fetch("/api/backend/inventory/receive", {
            method: "POST", headers,
            body: JSON.stringify({ product_id: "product", branch_id: "branch", real_quantity: 0, ghost_quantity: 1 }),
          }),
          fetch("/api/backend/inventory/rebalance", {
            method: "POST", headers,
            body: JSON.stringify({ product_id: "product", branch_id: "branch", from_bucket: "real", to_bucket: "ghost", quantity: 1 }),
          }),
        ]);
        return responses.map((response) => response.status);
      });
      expect(statuses).toEqual([expectedStatus, expectedStatus, expectedStatus]);
      await session.context.close();
    }
  });

  test("ปุ่มออกจากระบบลบ session และกลับหน้าเข้าสู่ระบบ", async ({
    browser,
  }) => {
    const session = await openSession(browser, "pos.mes@erp.local", "/sales");
    await session.page.getByRole("button", { name: "ออกจากระบบ" }).click();
    await session.page.waitForURL(/\/login$/);
    await session.page.goto("/sales");
    await session.page.waitForURL(/\/login$/);
    await session.context.close();
  });

  /**
   * A logout that could not reach the server must not look like one that did.
   *
   * The navigation used to sit in a `finally`, so it ran whether or not the
   * request completed. With the API unreachable the browser went to /login
   * holding a cookie that was never cleared — the operator saw a login screen,
   * believed the till was locked, and the next navigation walked straight back
   * into the session. That is the dangerous direction to fail in on a shared
   * terminal, so this asserts the opposite: stay put, say so, and leave the
   * session visibly intact rather than pretending it ended.
   */
  test("ออกจากระบบไม่สำเร็จ ต้องไม่พาไปหน้าเข้าสู่ระบบ และ session ต้องยังใช้งานได้", async ({
    browser,
  }) => {
    const context = await browser.newContext();
    const page = await context.newPage();
    await signIn(page, "superadmin@erp.local", "/dashboard");

    const cookieValue = async () =>
      (await context.cookies()).find((c) => c.name === "pharmacy_erp_auth")?.value;
    const before = await cookieValue();
    expect(before, "a session cookie must exist before logging out").toBeTruthy();

    // The request never completes — the shape of an API that is down, or a
    // browser that has lost the network mid-click.
    await page.route("**/api/backend/auth/logout", (route) => route.abort("failed"));
    await page.getByRole("button", { name: "ออกจากระบบ" }).first().click();
    await page.waitForTimeout(3000);

    await expect(page).toHaveURL(/\/dashboard$/);
    await expect(page.locator("[data-sonner-toast]").first()).toBeVisible();
    expect(await cookieValue(), "a failed logout must not discard the session").toBe(before);

    // And the session is not merely bytes in a jar — it still opens pages.
    await page.unroute("**/api/backend/auth/logout");
    await page.goto("/real-inventory");
    await expect(page).not.toHaveURL(/\/login$/);

    await context.close();
  });

  /**
   * Logging in a second time, in a browser that has already been logged in
   * once, is the sequence that broke: login answered 200 and set the cookie,
   * and the page stayed on /login.
   *
   * The cause was client-side. router.push() consults the App Router's Router
   * Cache, which by then holds a /dashboard entry fetched while logged out —
   * middleware answers such a request with a redirect back to /login — and the
   * router.refresh() that followed it in the same transition could not clear
   * that entry before the push had already read it. A fresh browser context
   * has an empty cache, so the bug is invisible there; the second login in the
   * SAME context, without reloading /login, is what exposes it.
   *
   * The assertion is not "the url ended up right": a client push often does
   * arrive, which is why this was intermittent rather than broken. It is that
   * the login performed a real document navigation. A marker set on window
   * before submitting cannot survive one, and does survive a client push, so
   * reintroducing router.push here fails this test deterministically.
   */
  test("เข้าสู่ระบบซ้ำหลังออกจากระบบ ต้องพาไปหน้าแรกด้วยการโหลดหน้าใหม่", async ({
    browser,
  }) => {
    const context = await browser.newContext();
    const page = await context.newPage();

    await signIn(page, "superadmin@erp.local", "/dashboard");

    await page.getByRole("button", { name: "ออกจากระบบ" }).first().click();
    await page.waitForURL(/\/login$/);

    // No reload here on purpose — this is the state a real user is in after
    // logging out, and reloading would reset the very cache under test.
    await page.evaluate(() => {
      (window as unknown as { __beforeLogin?: boolean }).__beforeLogin = true;
    });

    await page.getByLabel("อีเมล").fill("superadmin@erp.local");
    await page.getByLabel("รหัสผ่าน").fill(passwordFor("superadmin@erp.local"));
    await page.getByRole("button", { name: "เข้าสู่ระบบ" }).click();

    await page.waitForURL(/\/dashboard$/);
    const markerSurvived = await page.evaluate(
      () => Boolean((window as unknown as { __beforeLogin?: boolean }).__beforeLogin)
    );
    expect(markerSurvived, "login must be a document navigation, not a client push").toBe(false);

    await context.close();
  });

  test("layout ผู้ดูแลและ POS ใช้งานได้บนหน้าจอมือถือโดยไม่ล้นแนวนอน", async ({
    browser,
  }) => {
    const adminContext = await browser.newContext({
      viewport: { width: 390, height: 844 },
    });
    const admin = await adminContext.newPage();
    await signIn(admin, "superadmin@erp.local", "/dashboard");
    await admin.getByRole("button", { name: "เปิดเมนูหลัก" }).click();
    await expect(
      admin.getByRole("navigation", { name: "เมนูหลักบนมือถือ" }),
    ).toBeVisible();
    await admin.getByRole("button", { name: "ปิดเมนู", exact: true }).click();
    await expect(admin.locator("aside")).toBeHidden();
    expect(
      await admin.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    await adminContext.close();

    const posContext = await browser.newContext({
      viewport: { width: 390, height: 844 },
    });
    const pos = await posContext.newPage();
    await signIn(pos, "pos.mes@erp.local", "/sales");
    await expect(
      pos.getByRole("navigation", { name: "เมนูจุดขายบนมือถือ", exact: true }),
    ).toBeVisible();
    await expect(
      pos.getByRole("heading", { name: "ขายหน้าร้าน" }),
    ).toBeVisible();
    await pos.getByRole("button", { name: "ตะกร้า", exact: true }).click();
    await expect(
      pos.getByRole("heading", { name: "รายการขายปัจจุบัน" }),
    ).toBeVisible();
    expect(
      await pos.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    await posContext.close();
  });
});
