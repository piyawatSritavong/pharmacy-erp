import type { Browser, BrowserContext, Page } from "@playwright/test";
import { expect, test } from "@playwright/test";

const password = "DevPassword123!";

type SessionHandle = {
  context: BrowserContext;
  page: Page;
};

async function signIn(page: Page, email: string, expectedPath: string) {
  await page.goto("/login");
  await page.getByLabel("อีเมล").fill(email);
  await page.getByLabel("รหัสผ่าน").fill(password);
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

  test("หน้าเข้าสู่ระบบแสดงเฉพาะบัญชีผู้ดูแลและพนักงานขาย", async ({
    page,
  }) => {
    await page.goto("/login");
    await expect(page.getByText("superadmin@erp.local")).toBeVisible();
    await expect(page.getByText("pos.mes@erp.local")).toBeVisible();
    await expect(page.getByText("pos.phahol@erp.local")).toBeVisible();
    await expect(page.getByText("pos.phasuk@erp.local")).toBeVisible();
    await expect(page.getByText("pos.nakhonpathom@erp.local")).toBeVisible();
    await expect(page.getByText(/Branch Admin/i)).toHaveCount(0);
    await expect(page.getByText(/ผู้ดูแลสาขา/)).toHaveCount(0);
  });

  test("ผู้ใช้สองบทบาทเข้าสู่ระบบพร้อมกันและเห็นเมนูของตนเอง", async ({
    browser,
  }) => {
    const sessions = await Promise.all([
      openSession(browser, "superadmin@erp.local", "/dashboard"),
      openSession(browser, "pos.mes@erp.local", "/sales"),
    ]);
    const [admin, pos] = sessions.map((session) => session.page);

    await expect(
      admin.getByRole("navigation", { name: "เมนูหลัก", exact: true }).getByRole("link"),
    ).toHaveCount(14);
    await expect(
      pos.getByRole("navigation", { name: "เมนูจุดขาย", exact: true }).getByRole("link"),
    ).toHaveCount(5);

    for (const name of [
      "แดชบอร์ด",
      "สต๊อกจริง",
      "สต๊อกผี",
      "หมวดสินค้า",
      "ใบสั่งซื้อเข้า",
      "บริษัทคู่ค้า",
      "โอนสินค้า",
      "สรุปสิ้นเดือน",
      "รายงานสรุปสิ้นเดือน",
      "รพ.สต.",
      "การขายและเอกสาร",
      "รายงาน",
      "Generate Report",
      "ตั้งค่า",
    ]) {
      await expect(
        admin.getByRole("link", { name, exact: true }).first(),
      ).toBeVisible();
    }
    await expect(
      admin.getByRole("link", { name: "ขายหน้าร้าน", exact: true }),
    ).toHaveCount(0);

    for (const name of [
      "ขายหน้าร้าน",
      "ประวัติ",
      "เช็กสต๊อก",
      "รับโอนสินค้า",
      "สรุปยอดขาย",
    ]) {
      await expect(
        pos.getByRole("link", { name, exact: true }).first(),
      ).toBeVisible();
    }
    await expect(
      pos.getByRole("link", { name: "ตั้งค่า", exact: true }),
    ).toHaveCount(0);

    await Promise.all(sessions.map((session) => session.context.close()));
  });

  test("สร้าง แก้ไข archive และ reactivate หมวดสินค้า", async ({ browser }) => {
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
      await page.getByRole("button", { name: `เก็บ ${editedName}` }).click();
      await expect(page.getByText("เก็บแล้ว", { exact: true })).toBeVisible();

      page.once("dialog", (prompt) => prompt.accept());
      await page.getByRole("button", { name: `นำ ${editedName} กลับมาใช้` }).click();
      await expect(page.getByText("เปิดใช้งาน", { exact: true }).last()).toBeVisible();
    } finally {
      if (categoryID) {
        await page.request.delete(`/api/backend/product-categories/${categoryID}`);
      }
      await session.context.close();
    }
  });

  test("POS ทั้ง 4 บัญชีเห็นเฉพาะสมาชิกสินค้าของสาขาตน", async ({ browser }) => {
    const accounts = [
      ["pos.mes@erp.local", 202],
      ["pos.phahol@erp.local", 414],
      ["pos.phasuk@erp.local", 424],
      ["pos.nakhonpathom@erp.local", 297],
    ] as const;

    for (const [email, expectedTotal] of accounts) {
      const session = await openSession(browser, email, "/sales");
      const response = await session.page.evaluate(async () => {
        const result = await fetch("/api/backend/products?search=OCH-&page=1&page_size=1", {
          cache: "no-store",
        });
        return { status: result.status, body: await result.json() };
      });
      expect(response.status).toBe(200);
      expect(response.body.pagination.total).toBe(expectedTotal);
      await session.context.close();
    }
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
      ["รพ.สต.", "/government-sales", "รพ.สต."],
      ["การขายและเอกสาร", "/sales-management", "ใบขาย"],
      ["รายงาน", "/global-reports", "รายงาน"],
      ["Generate Report", "/generate-report", "Generate Report"],
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
    await session.page.route("**/api/backend/accounting/month-end/reconciliation-overview", async (route) => {
      await route.fulfill({
        contentType: "application/json",
        json: {
          original_revenue: 367897.91,
          suppressed_revenue: 244298.87,
          base_revenue: 123599.04,
          invoice_count: 68,
          suppressed_invoice_count: 37,
          invoice_groups: {
            cash_suppressed: { invoice_count: 37, revenue: 244298.87 },
            cash_full_tax: { invoice_count: 0, revenue: 0 },
            bank_transfer: { invoice_count: 29, revenue: 119599.04 },
            mixed: { invoice_count: 2, revenue: 4000 },
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
          original_revenue: 367897.91,
          suppressed_revenue: 244298.87,
          final_revenue: 123599.04,
          suppressed_invoice_count: 1,
          suppression_candidates: [{
            id: "invoice-e2e",
            branch_name: "MES",
            invoice_number: "MES-BL2026080100001",
            customer_name: "ลูกค้าทดสอบ",
            created_at: "2026-08-01T03:00:00Z",
            total_amount: 100,
            items: [{ id: "item-e2e", product_id: "product-e2e", product_name: "สินค้าทดสอบ", sku: "E2E", quantity: 1 }],
          }],
          stock_projection: {
            branch_real_returned: 1,
            warehouse_real_received: 1,
            warehouse_ghost_deducted: 1,
            ghost_deficit_created: 1,
            products: [{ product_id: "product-e2e", product_name: "สินค้าทดสอบ", quantity: 1, warehouse_ghost_before: 0, warehouse_ghost_after: -1, deficit_created: 1 }],
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
    await expect(session.page.getByLabel("ยอดขายเป้าหมาย")).toBeDisabled();
    await expect(session.page.getByLabel("เปอร์เซ็นต์ปรับราคา")).toBeDisabled();
    await session.page
      .getByRole("button", { name: "ตรวจสอบใบขายและสต๊อก" })
      .click();
    await expect(session.page.getByText("รายละเอียดกลุ่มบิลและสูตรจากข้อมูลจริง")).toBeVisible();
    await expect(session.page.getByText("Ghost deficit ที่คาดว่าจะเกิด")).toBeVisible();
    await expect(session.page.getByText("ปุ่มยืนยันใช้งานได้แม้ Ghost ไม่พอ")).toBeVisible();
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
      ["เช็กสต๊อก", "/inventory-check", "เช็กสต๊อก"],
      ["รับโอนสินค้า", "/transfer-receipts", "รับโอนสินค้า"],
      ["สรุปยอดขาย", "/daily-sales", /สรุปยอดขาย/],
      ["ขายหน้าร้าน", "/sales", "ขายหน้าร้าน"],
    ] as const;

    for (const [linkName, path, heading] of pages) {
      await session.page
        .getByRole("link", { name: linkName, exact: true })
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
    await session.page
      .getByRole("button", { name: /^เพิ่ม .* ลงตะกร้า$/ })
      .first()
      .click();
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
      paymentDialog.getByText("ชำระเงินเสร็จสิ้น", { exact: true }),
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

    await session.page.goto("/inventory-check");
    const requestStatus = session.page.getByRole("heading", {
      name: "สถานะคำขอสินค้า",
    });
    const stockTable = session.page
      .getByRole("heading", { name: "เช็กสต๊อก", exact: true })
      .last();
    await expect(requestStatus).toBeVisible();
    await expect(stockTable).toBeVisible();
    expect(
      await requestStatus.evaluate((status) => {
        const stock = Array.from(document.querySelectorAll("h2")).find(
          (heading) => heading.textContent?.trim() === "เช็กสต๊อก",
        );
        return Boolean(
          stock &&
            status.compareDocumentPosition(stock) &
              Node.DOCUMENT_POSITION_FOLLOWING,
        );
      }),
    ).toBe(true);
    await session.page
      .getByRole("button", { name: "สร้างใบเบิกสินค้า" })
      .click();
    await expect(
      session.page
        .getByRole("dialog")
        .getByRole("heading", { name: "สร้างใบเบิกสินค้า" }),
    ).toBeVisible();
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

  test("layout ผู้ดูแลและ POS ใช้งานได้บนหน้าจอมือถือโดยไม่ล้นแนวนอน", async ({
    browser,
  }) => {
    const adminContext = await browser.newContext({
      viewport: { width: 390, height: 844 },
    });
    const admin = await adminContext.newPage();
    await signIn(admin, "superadmin@erp.local", "/dashboard");
    await expect(
      admin.getByRole("navigation", { name: "เมนูหลักบนมือถือ" }),
    ).toBeVisible();
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
      pos.getByRole("navigation", { name: "เมนูจุดขาย" }),
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
