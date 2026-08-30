import { expect, test } from "@playwright/test";

const password = "DevPassword123!";

test.describe("Supply Chain Management", () => {
  test.skip(!process.env.E2E_RUN, "กำหนด E2E_RUN=1 เมื่อเปิดบริการแล้ว");

  test("สร้างคู่ค้า รับสินค้าเข้า Lot และค้นผ่าน Generate Report", async ({
    page,
  }) => {
    const suffix = Date.now();
    const supplierName = `บริษัท SCM E2E ${suffix}`;
    const supplierCode = `SCM-${suffix}`;
    const productName = `สินค้าทดสอบ Lot ${suffix}`;
    const sku = `LOT-${suffix}`;
    let supplierID = "";
    let purchaseOrderID = "";

    await page.goto("/login");
    await page.getByLabel("อีเมล").fill("superadmin@erp.local");
    await page.getByLabel("รหัสผ่าน").fill(password);
    await page.getByRole("button", { name: "เข้าสู่ระบบ" }).click();
    await page.waitForURL(/\/dashboard$/);

    await page
      .getByRole("link", { name: "บริษัทคู่ค้า", exact: true })
      .first()
      .click();
    await expect(
      page.getByRole("heading", { name: "บริษัทคู่ค้า", exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "เพิ่มบริษัทคู่ค้า" }).click();
    const supplierDialog = page.getByRole("dialog");
    await supplierDialog.getByLabel("รหัสคู่ค้า").fill(supplierCode);
    await supplierDialog.getByLabel("ชื่อบริษัท *").fill(supplierName);
    await supplierDialog
      .getByLabel("เลขผู้เสียภาษี")
      .fill(String(suffix).padStart(13, "0").slice(-13));
    await supplierDialog.getByLabel("ที่อยู่").fill("99 ถนนทดสอบ");
    await supplierDialog.getByLabel("จังหวัด").fill("กรุงเทพมหานคร");
    const supplierResponsePromise = page.waitForResponse(
      (response) =>
        response.url().includes("/api/backend/suppliers") &&
        response.request().method() === "POST",
    );
    await supplierDialog
      .getByRole("button", { name: "บันทึก", exact: true })
      .click();
    const supplierResponse = await supplierResponsePromise;
    expect(supplierResponse.status()).toBe(201);
    supplierID = String((await supplierResponse.json()).id);
    await expect(page.getByText(supplierName, { exact: true })).toBeVisible();

    await page
      .getByRole("link", { name: "ใบสั่งซื้อเข้า", exact: true })
      .first()
      .click();
    await expect(
      page.getByRole("heading", { name: "ใบสั่งซื้อเข้า", exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "สร้างใบสั่งซื้อเข้า" }).click();
    const poDialog = page.getByRole("dialog");
    await poDialog.getByLabel("บริษัทคู่ค้า *").click();
    await page.getByRole("option", { name: supplierName, exact: true }).click();

    let productPage = 0;
    await page.route(
      "**/api/backend/purchase-orders/product-options?*",
      async (route) => {
        productPage += 1;
        const hasCursor = new URL(route.request().url()).searchParams.has(
          "cursor",
        );
        const items = hasCursor
          ? [
              {
                id: "99999999-9999-4999-8999-999999999999",
                sku: "MOCK-21",
                name: "Mock product 21",
                unit_name: "ชิ้น",
                cost_price: 1,
                sellable_quantity: 0,
              },
            ]
          : Array.from({ length: 20 }, (_, index) => ({
              id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, "0")}`,
              sku: `MOCK-${index + 1}`,
              name: `Mock product ${index + 1}`,
              unit_name: "ชิ้น",
              cost_price: 1,
              sellable_quantity: 0,
            }));
        await route.fulfill({
          contentType: "application/json",
          json: {
            items,
            next_cursor: hasCursor ? "" : "MjA",
            has_more: !hasCursor,
          },
        });
      },
    );
    await poDialog.getByLabel("ค้นหาสินค้าเข้า PO").fill("mock");
    const productList = poDialog.getByRole("listbox");
    await expect(
      productList.getByText("Mock product 20", { exact: true }),
    ).toBeVisible();
    await productList.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
      element.dispatchEvent(new Event("scroll"));
    });
    await expect(
      productList.getByText("Mock product 21", { exact: true }),
    ).toBeVisible();
    expect(productPage).toBeGreaterThanOrEqual(2);
    await page.unroute("**/api/backend/purchase-orders/product-options?*");

    await poDialog.getByRole("button", { name: "สินค้าใหม่" }).click();
    await poDialog.getByLabel("ชื่อสินค้า *").fill(productName);
    await poDialog.getByLabel("SKU").fill(sku);
    await poDialog.getByLabel("หน่วย", { exact: true }).fill("กล่อง");
    await poDialog.getByLabel("ราคาขาย", { exact: true }).fill("120");
    await poDialog.getByLabel("ราคาปลีก", { exact: true }).fill("135");
    await poDialog.getByLabel("ลดได้สูงสุด/หน่วย", { exact: true }).fill("15");
    await poDialog.getByLabel("เตือนสต๊อกจริง", { exact: true }).fill("3");
    await poDialog.getByLabel("ติดตามวันหมดอายุ").check();
    await poDialog.getByLabel("จำนวน", { exact: true }).fill("5");
    await poDialog.getByLabel("ราคาซื้อ/หน่วย", { exact: true }).fill("70");
    await poDialog.getByLabel("Lot/Batch", { exact: true }).fill(`BATCH-${suffix}`);
    await poDialog.getByLabel("วันหมดอายุ *").fill("2027-12-31");
    const poResponsePromise = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/backend/purchase-orders") &&
        response.request().method() === "POST",
    );
    await poDialog
      .getByRole("button", { name: "บันทึกและรับเข้าสต๊อก" })
      .click();
    const poResponse = await poResponsePromise;
    expect(poResponse.status()).toBe(201);
    purchaseOrderID = String((await poResponse.json()).id);
    const detailDialog = page.getByRole("dialog");
    await expect(
      detailDialog.getByText(productName, { exact: true }),
    ).toBeVisible();
    await expect(
      detailDialog.getByText(`BATCH-${suffix}`, { exact: true }),
    ).toBeVisible();
    await expect(detailDialog.getByText("5", { exact: true })).toHaveCount(2);
    await expect(
      detailDialog.getByRole("button", { name: "แก้รายการ" }),
    ).toBeVisible();
    await expect(
      detailDialog.getByRole("button", { name: "แก้ยอด/หมายเหตุ" }),
    ).toBeVisible();
    await page.keyboard.press("Escape");

    await page
      .getByRole("link", { name: "Generate Report", exact: true })
      .first()
      .click();
    await page.getByLabel("ชุดข้อมูลหลัก").selectOption("purchase_order_items");
    await page.getByText(/Advanced filters/).click();
    await page.getByRole("button", { name: "เงื่อนไข", exact: true }).click();
    await page.getByLabel("ฟิลด์ตัวกรอง").selectOption("supplier_name");
    await page.getByLabel("ค่าตัวกรอง").fill(supplierName);
    await expect(
      page
        .locator("#report-builder tbody")
        .getByText(supplierName, { exact: true })
        .first(),
    ).toBeVisible();
    await expect(
      page
        .locator("#report-builder tbody")
        .getByText(productName, { exact: true })
        .first(),
    ).toBeVisible();

    if (purchaseOrderID) {
      const cancel = await page.request.post(
        `/api/backend/purchase-orders/${purchaseOrderID}/cancel`,
        { data: { reason: "ล้างข้อมูลจาก SCM E2E" } },
      );
      expect(cancel.ok()).toBeTruthy();
    }
    if (supplierID) {
      const archive = await page.request.delete(
        `/api/backend/suppliers/${supplierID}`,
      );
      expect(archive.ok()).toBeTruthy();
    }
  });
});
